// The fetch+SSE implementation of the workbench Transport, talking to
// the cmd/satpulsewb HTTP API. The per-run access token rides a query
// parameter on every request. Writer POSTs (the mutating session
// operations) also carry the current write seat; reader POSTs and the SSE
// streams carry only the token.

import type {
    ConnState,
    CorrectionSource,
    DecodeOptions,
    LLH,
    PortInfo,
    Transport,
} from '@satpulse/workbench/src/transport';

// HttpError carries the HTTP status so callers can distinguish an
// auth failure (401) and a lost seat (410) from operation errors.
export class HttpError extends Error {
    status: number;
    constructor(status: number, message: string) {
        super(message);
        this.status = status;
    }
}

export async function newHTTPTransport(token: string): Promise<Transport> {
    const authParams = new URLSearchParams();
    if (token) authParams.set('t', token);
    const authQuery = authParams.size ? '?' + authParams : '';
    async function claim(): Promise<{seat: string; grant: string}> {
        const resp = await fetch('/api/seat' + authQuery, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: '{}',
        });
        if (!resp.ok) throw await httpError(resp);
        const c = await resp.json();
        if (typeof c.seat !== 'string' || !c.seat || typeof c.grant !== 'string' || !c.grant) {
            throw new Error('Invalid seat claim response');
        }
        return {seat: c.seat, grant: c.grant};
    }
    const initial = await claim();
    let seat = initial.seat;
    const events = new EventStreams(authQuery, initial.grant);
    const seatQuery = () => {
        const p = new URLSearchParams(authParams);
        p.set('seat', seat);
        return '?' + p;
    };
    async function call(method: string, path: string, body: unknown, writer: boolean): Promise<any> {
        const init: RequestInit = {method};
        if (body !== undefined) {
            init.headers = {'Content-Type': 'application/json'};
            init.body = JSON.stringify(body);
        }
        const sent = seat;
        const query = method === 'POST' && writer ? seatQuery() : authQuery;
        const resp = await fetch('/api/' + path + query, init);
        if (!resp.ok) {
            // Flip read-only only if the seat this request carried is still
            // current: a reclaim while the POST was in flight makes the 410
            // stale, and no further writer broadcast would flip us back.
            if (resp.status === 410 && sent === seat) events.readOnly();
            throw await httpError(resp);
        }
        return resp.json();
    }
    const get = (path: string) => call('GET', path, undefined, false);
    const readerPost = (path: string, body?: unknown) => call('POST', path, body ?? {}, false);
    const writerPost = (path: string, body?: unknown) => call('POST', path, body ?? {}, true);
    return {
        getConnState: () => get('state') as Promise<ConnState>,
        getReceiverState: () => get('receiver'),
        getCorrectionsState: () => get('corrections'),
        getAllSignals: (gnss: string[]) => readerPost('signals', {gnss}),
        readConfig: () => writerPost('config/read'),
        applyConfig: async target => { await writerPost('config/apply', target); },
        startCorrections: async (src: CorrectionSource) => { await writerPost('corrections/start', src); },
        stopCorrections: async () => { await writerPost('corrections/stop'); },
        decodePacket: (data: string, opts: DecodeOptions) =>
            readerPost('decode-packet', {data, hex: opts.hex, out: opts.out}),
        ecefToLLH: (x, y, z) => readerPost('geo/ecef-to-llh', {x, y, z}) as Promise<LLH>,
        llhToECEF: (lat, lon, height) => readerPost('geo/llh-to-ecef', {lat, lon, height}),
        checkOnEarth: (x, y, z) => readerPost('geo/check-on-earth', {x, y, z}),
        velNEDtoECEF: (n, e, d) => readerPost('geo/vel-ned-to-ecef', {n, e, d}),
        velECEFtoNED: (x, y, z) => readerPost('geo/vel-ecef-to-ned', {x, y, z}),
        eventsOn: (name, cb) => events.on(name, cb),
        openURL: url => { window.open(url, '_blank'); },
        reclaim: async () => {
            const c = await claim();
            seat = c.seat;
            events.setGrant(c.grant);
        },
        connection: {
            connect: async (device: string, speed: number) => {
                await writerPost('connect', {device, speed});
            },
            disconnect: async () => { await writerPost('disconnect'); },
            listPorts: () => get('ports') as Promise<PortInfo[]>,
        },
        // No msgFile capability yet: message files arrive with the
        // library/upload endpoints; the Messages tab stays hidden.
    };
}

async function httpError(resp: Response): Promise<HttpError> {
    let msg = '';
    try {
        msg = (await resp.json()).error;
    } catch {
        // fall through to the status text
    }
    return new HttpError(resp.status, msg || resp.statusText);
}

// EventStreams multiplexes transport event subscriptions onto the /sse
// endpoints: one long-lived EventSource for the regular events, and a
// second one carrying only the high-rate gps:packet stream, open only
// while someone is subscribed to it -- the server streams packets only
// while such a client is connected.
//
// It also owns the window's writer state. The server broadcasts a sticky
// `writer` event carrying the current grant; this window is the writer iff
// that grant equals the grant from its own claim. It exposes this to the app
// as the synthetic wb:readonly event (boolean payload) and surfaces a stale
// token, seen as a terminal EventSource close, as wb:authlost.
class EventStreams {
    private query: string;
    private listeners = new Map<string, Set<(data: any) => void>>();
    private main?: EventSource;
    private packets?: EventSource;
    private grant: string;
    private broadcast = ''; // latest broadcast grant; '' until the first writer event
    private ro = false;
    private authLost = false;

    constructor(query: string, grant: string) {
        this.query = query;
        this.grant = grant;
    }

    on(name: string, cb: (data: any) => void): () => void {
        let set = this.listeners.get(name);
        const first = !set;
        if (!set) {
            set = new Set();
            this.listeners.set(name, set);
        }
        set.add(cb);
        // Synthetic events ride no stream: register the callback and deliver
        // the current state so a late subscriber is consistent.
        if (name === 'wb:readonly') {
            const ro = this.ro;
            queueMicrotask(() => { if (set!.has(cb)) cb(ro); });
            return () => { set!.delete(cb); };
        }
        if (name === 'wb:authlost') {
            if (this.authLost) queueMicrotask(() => { if (set!.has(cb)) cb({}); });
            return () => { set!.delete(cb); };
        }
        if (name === 'gps:packet') {
            if (!this.packets) {
                this.packets = this.openStream((this.query ? this.query + '&' : '?') + 'stream=packets');
                this.addDispatch(this.packets, name);
            }
        } else {
            if (!this.main) {
                this.main = this.openStream(this.query);
                this.main.addEventListener('writer', e => this.onWriter(e as MessageEvent));
            }
            if (first) {
                this.addDispatch(this.main, name);
            }
        }
        return () => {
            set!.delete(cb);
            if (name === 'gps:packet' && set!.size === 0 && this.packets) {
                this.packets.close();
                this.packets = undefined;
            }
        };
    }

    // readOnly forces the window into the read-only state, used as a belt-and-
    // braces path when a writer POST returns 410 (a missed writer broadcast).
    readOnly() {
        this.setReadOnly(true);
    }

    // setGrant adopts a fresh claim's grant (a re-claim). The broadcast is the
    // sole authority for the writable flag: compare against the latest one
    // rather than forcing writable, so a stale claim response (another window
    // claimed while ours was in flight, and its broadcast already arrived)
    // cannot mark this window writable. If our own broadcast has not arrived
    // yet this shows read-only for an instant until it does -- the safe
    // direction. Before any broadcast the window is the writer (boot).
    setGrant(grant: string) {
        this.grant = grant;
        this.setReadOnly(this.broadcast !== '' && this.broadcast !== grant);
    }

    private onWriter(e: MessageEvent) {
        let grant = '';
        try {
            grant = JSON.parse(e.data).grant;
        } catch {
            return;
        }
        this.broadcast = grant;
        this.setReadOnly(grant !== this.grant);
    }

    private setReadOnly(ro: boolean) {
        if (ro === this.ro) return;
        this.ro = ro;
        for (const cb of this.listeners.get('wb:readonly') ?? []) {
            cb(ro);
        }
    }

    // openStream opens an SSE EventSource for the token. A terminal close
    // (readyState CLOSED) is unambiguous now that SSE is seat-free: it can only
    // mean the token went stale after a server restart, so surface wb:authlost.
    // A transient drop leaves readyState CONNECTING and is ignored so the
    // browser reconnects and re-primes.
    private openStream(query: string): EventSource {
        const es = new EventSource('/sse' + query);
        es.addEventListener('error', () => {
            if (es.readyState === EventSource.CLOSED) this.fireAuthLost();
        });
        return es;
    }

    private fireAuthLost() {
        if (this.authLost) return;
        this.authLost = true;
        this.main?.close();
        this.packets?.close();
        this.main = undefined;
        this.packets = undefined;
        for (const cb of this.listeners.get('wb:authlost') ?? []) {
            cb({});
        }
    }

    private addDispatch(es: EventSource, name: string) {
        es.addEventListener(name, e => {
            const set = this.listeners.get(name);
            if (!set) return;
            const data = JSON.parse((e as MessageEvent).data);
            for (const cb of set) {
                cb(data);
            }
        });
    }
}

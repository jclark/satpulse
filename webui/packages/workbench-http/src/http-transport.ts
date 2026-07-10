// The fetch+SSE implementation of the workbench Transport, talking to
// the cmd/satpulsewb HTTP API. The per-run access token rides a query
// parameter on every request. POSTs and SSE streams also carry the
// current workbench seat (EventSource cannot set headers).

import type {
    ConnState,
    CorrectionSource,
    DecodeOptions,
    LLH,
    PortInfo,
    Transport,
} from '@satpulse/workbench/src/transport';

// HttpError carries the HTTP status so callers can distinguish an
// auth failure (401) and seat loss (410) from operation errors.
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
    const claimResp = await fetch('/api/seat' + authQuery, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: '{}',
    });
    if (!claimResp.ok) throw await httpError(claimResp);
    const claim = await claimResp.json();
    if (typeof claim.seat !== 'string' || !claim.seat) {
        throw new Error('Invalid seat claim response');
    }
    const seatParams = new URLSearchParams(authParams);
    seatParams.set('seat', claim.seat);
    const seatQuery = '?' + seatParams;
    const events = new EventStreams(seatQuery);
    async function call(method: string, path: string, body?: unknown): Promise<any> {
        const init: RequestInit = {method};
        if (body !== undefined) {
            init.headers = {'Content-Type': 'application/json'};
            init.body = JSON.stringify(body);
        }
        const resp = await fetch('/api/' + path + (method === 'POST' ? seatQuery : authQuery), init);
        if (!resp.ok) {
            if (resp.status === 410) events.seatLost();
            throw await httpError(resp);
        }
        return resp.json();
    }
    const get = (path: string) => call('GET', path);
    const post = (path: string, body?: unknown) => call('POST', path, body ?? {});
    return {
        getConnState: () => get('state') as Promise<ConnState>,
        getReceiverState: () => get('receiver'),
        getCorrectionsState: () => get('corrections'),
        getAllSignals: (gnss: string[]) => post('signals', {gnss}),
        readConfig: () => post('config/read'),
        applyConfig: async target => { await post('config/apply', target); },
        startCorrections: async (src: CorrectionSource) => { await post('corrections/start', src); },
        stopCorrections: async () => { await post('corrections/stop'); },
        decodePacket: (data: string, opts: DecodeOptions) =>
            post('decode-packet', {data, hex: opts.hex, out: opts.out}),
        ecefToLLH: (x, y, z) => post('geo/ecef-to-llh', {x, y, z}) as Promise<LLH>,
        llhToECEF: (lat, lon, height) => post('geo/llh-to-ecef', {lat, lon, height}),
        checkOnEarth: (x, y, z) => post('geo/check-on-earth', {x, y, z}),
        velNEDtoECEF: (n, e, d) => post('geo/vel-ned-to-ecef', {n, e, d}),
        velECEFtoNED: (x, y, z) => post('geo/vel-ecef-to-ned', {x, y, z}),
        eventsOn: (name, cb) => events.on(name, cb),
        openURL: url => { window.open(url, '_blank'); },
        connection: {
            connect: async (device: string, speed: number) => {
                await post('connect', {device, speed});
            },
            disconnect: async () => { await post('disconnect'); },
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
class EventStreams {
    private query: string;
    private listeners = new Map<string, Set<(data: any) => void>>();
    private main?: EventSource;
    private packets?: EventSource;
    private lost = false;

    constructor(query: string) {
        this.query = query;
    }

    on(name: string, cb: (data: any) => void): () => void {
        let set = this.listeners.get(name);
        const first = !set;
        if (!set) {
            set = new Set();
            this.listeners.set(name, set);
        }
        set.add(cb);
        if (this.lost) {
            if (name === 'wb:seatlost') queueMicrotask(() => cb({}));
            return () => { set.delete(cb); };
        }
        if (name === 'gps:packet') {
            if (!this.packets) {
                this.packets = this.openStream((this.query ? this.query + '&' : '?') + 'stream=packets');
                this.addDispatch(this.packets, name);
            }
        } else {
            if (!this.main) {
                this.main = this.openStream(this.query);
            }
            if (first && name !== 'wb:seatlost') {
                this.addDispatch(this.main, name);
            }
        }
        return () => {
            set.delete(cb);
            if (name === 'gps:packet' && set.size === 0 && this.packets) {
                this.packets.close();
                this.packets = undefined;
            }
        };
    }

    seatLost() {
        if (this.lost) return;
        this.lost = true;
        this.main?.close();
        this.packets?.close();
        this.main = undefined;
        this.packets = undefined;
        for (const cb of this.listeners.get('wb:seatlost') ?? []) {
            cb({});
        }
    }

    // openStream opens an SSE EventSource for the seat. A takeover event means
    // the seat was lost. A terminal close (readyState CLOSED) is ambiguous: it
    // covers both a stale-seat 204 and a stale-token 401 after a restart, and
    // EventSource hides the status, so terminalClose probes to tell them apart.
    // Transient drops leave readyState CONNECTING and are ignored so the browser
    // can reconnect and re-prime.
    private openStream(query: string): EventSource {
        const es = new EventSource('/sse' + query);
        es.addEventListener('takeover', () => this.seatLost());
        es.addEventListener('error', () => {
            if (es.readyState === EventSource.CLOSED) this.terminalClose();
        });
        return es;
    }

    // terminalClose reacts to an EventSource that closed without retrying. The
    // server answered a terminal status that EventSource does not expose, so
    // probe with a GET (token-checked but seat-free) to tell a stale seat from
    // a stale token: 200 means the token still works, so the seat was
    // superseded; 401 means a restart minted a fresh token, so reload to reach
    // the notice pointing at the newly printed URL.
    private async terminalClose() {
        if (this.lost) return;
        let resp;
        try {
            resp = await fetch('/api/state' + this.query);
        } catch {
            return; // server unreachable; not a terminal condition
        }
        if (resp.status === 401) {
            window.location.reload();
            return;
        }
        this.seatLost();
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

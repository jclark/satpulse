// The fetch+SSE implementation of the workbench Transport, talking to
// the cmd/satpulsewb HTTP API. The per-run access token rides a query
// parameter on every request, including the SSE streams (EventSource
// cannot set headers).

import type {
    ConnState,
    CorrectionSource,
    DecodeOptions,
    LLH,
    PortInfo,
    Transport,
} from '@satpulse/workbench/src/transport';

export function newHTTPTransport(token: string): Transport {
    const query = token ? '?t=' + encodeURIComponent(token) : '';
    async function call(method: string, path: string, body?: unknown): Promise<any> {
        const init: RequestInit = {method};
        if (body !== undefined) {
            init.headers = {'Content-Type': 'application/json'};
            init.body = JSON.stringify(body);
        }
        const resp = await fetch('/api/' + path + query, init);
        if (!resp.ok) {
            let msg = '';
            try {
                msg = (await resp.json()).error;
            } catch {
                // fall through to the status text
            }
            throw new Error(msg || resp.statusText);
        }
        return resp.json();
    }
    const get = (path: string) => call('GET', path);
    const post = (path: string, body?: unknown) => call('POST', path, body ?? {});
    const events = new EventStreams(query);
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
            connect: async (device: string, speed: number, vendor: string) => {
                await post('connect', {device, speed, vendor});
            },
            disconnect: async () => { await post('disconnect'); },
            listPorts: () => get('ports') as Promise<PortInfo[]>,
            listVendors: () => get('vendors') as Promise<string[]>,
        },
        // No msgFile capability yet: message files arrive with the
        // library/upload endpoints; the Messages tab stays hidden.
    };
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
        if (name === 'gps:packet') {
            if (!this.packets) {
                this.packets = new EventSource('/sse' + (this.query ? this.query + '&' : '?') + 'stream=packets');
                this.addDispatch(this.packets, name);
            }
        } else {
            if (!this.main) {
                this.main = new EventSource('/sse' + this.query);
            }
            if (first) {
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

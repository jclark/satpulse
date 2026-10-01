import type {Transport} from '@satpulse/workbench/src/transport';
import {WebSerial} from './web-serial';

interface WasmAPI {
    init(serial: WebSerial, vendor: string): string | null;
    on(name: string, cb: ((json: string) => void) | null): void;
    call(name: string, json: string): Promise<string>;
}

interface GoRuntime {
    importObject: WebAssembly.Imports;
    run(instance: WebAssembly.Instance): Promise<void>;
}

declare global {
    var Go: new () => GoRuntime;
    var satpulseWorkbench: WasmAPI;
    var satpulseWorkbenchReady: () => void;
}

export async function newWasmTransport(onFatal: (error: Error) => void): Promise<Transport> {
    const go = new Go();
    let ready!: () => void;
    const registered = new Promise<void>(resolve => { ready = resolve; });
    globalThis.satpulseWorkbenchReady = ready;
    const response = await fetch(new URL('./workbench.wasm', document.baseURI));
    if (!response.ok) throw new Error(`Could not load Workbench (${response.status}).`);
    const {instance} = await WebAssembly.instantiateStreaming(response, go.importObject);
    const serial = new WebSerial();
    let failure: Error | undefined;
    let started = false;
    let rejectFailure!: (error: Error) => void;
    const failed = new Promise<never>((_, reject) => { rejectFailure = reject; });
    const fail = (reason: unknown) => {
        if (failure) return;
        failure = reason instanceof Error ? reason : new Error(String(reason));
        rejectFailure(failure);
        void serial.close();
        if (started) onFatal(failure);
    };
    go.run(instance).then(() => fail(new Error('Workbench stopped. Reload the page to restart.')), fail);
    await Promise.race([registered, failed]);
    const api = globalThis.satpulseWorkbench;
    const error = api.init(serial, new URLSearchParams(window.location.search).get('vendor') ?? '');
    if (error) throw new Error(error);
    const call = async (name: string, data: unknown = {}) => {
        if (failure) throw failure;
        return JSON.parse(await Promise.race([api.call(name, JSON.stringify(data)), failed]));
    };
    const listeners = new Map<string, Set<(data: any) => void>>();
    started = true;
    return {
        getConnection: () => call('connection'),
        getReceiverState: () => call('receiver'),
        getAllSignals: gnss => call('signals', {gnss}),
        readConfig: () => call('config/read'),
        applyConfig: target => call('config/apply', target),
        decodePacket: (data, opts) => call('decode-packet', {data, ...opts}),
        ecefToLLH: (x, y, z) => call('geo/ecef-to-llh', [x, y, z]),
        llhToECEF: (lat, lon, height) => call('geo/llh-to-ecef', [lat, lon, height]),
        checkOnEarth: (x, y, z) => call('geo/check-on-earth', [x, y, z]),
        velNEDtoECEF: (n, e, d) => call('geo/vel-ned-to-ecef', [n, e, d]),
        velECEFtoNED: (x, y, z) => call('geo/vel-ecef-to-ned', [x, y, z]),
        eventsOn: (name, cb) => {
            if (failure) return () => {};
            let set = listeners.get(name);
            if (!set) {
                set = new Set();
                listeners.set(name, set);
                api.on(name, json => {
                    // Session events can originate while Go is holding locks.
                    queueMicrotask(() => {
                        if (failure) return;
                        const data = JSON.parse(json);
                        for (const f of listeners.get(name) ?? []) f(data);
                    });
                });
            }
            set.add(cb);
            return () => {
                set!.delete(cb);
                if (!set!.size) {
                    listeners.delete(name);
                    if (!failure) api.on(name, null);
                }
            };
        },
        openURL: url => { window.open(url, '_blank', 'noopener'); },
        connection: {
            listPorts: () => serial.listPorts(),
            choosePort: () => serial.choosePort(),
            connect: (device, speed) => call('connect', {device, speed}),
            disconnect: () => call('disconnect'),
        },
        msgFile: {
            listMsgFiles: () => call('msgfile/catalog'),
            selectMsgFile: (vendor, file) => call('msgfile/select', {vendor, file}),
            sendMsgFile: (tag, port, save) => call('msgfile/send', {tag, port, save}),
            cancelMsgSend: () => call('msgfile/cancel'),
        },
    };
}

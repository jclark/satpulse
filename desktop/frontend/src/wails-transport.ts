// The Wails implementation of the workbench Transport: method calls go
// to the generated Go bindings, events ride the Wails event bus.

import type {
    ConnState,
    CorrectionSource,
    DecodeOptions,
    LLH,
    MsgFileInfo,
    PortInfo,
    Transport,
} from '@satpulse/workbench/src/transport';
import * as App from '../wailsjs/go/main/App';
import {BrowserOpenURL, EventsOn} from '../wailsjs/runtime/runtime';

// wrap converts a Wails rejection (a plain string) into the Error the
// transport contract promises.
async function wrap<T>(p: Promise<T>): Promise<T> {
    try {
        return await p;
    } catch (e) {
        throw e instanceof Error ? e : new Error(String(e));
    }
}

// act adapts an action binding's Result return to resolve/reject.
async function act(p: Promise<{ok: boolean; error?: string}>): Promise<void> {
    const r = await wrap(p);
    if (!r.ok) throw new Error(r.error || 'operation failed');
}

export function newWailsTransport(): Transport {
    return {
        getConnState: () => wrap(App.GetConnState()) as Promise<ConnState>,
        getReceiverState: () => wrap(App.GetReceiverState()),
        getCorrectionsState: () => wrap(App.GetCorrectionsState()),
        getAllSignals: (gnss: string[]) => wrap(App.GetAllSignals(gnss as any)),
        readConfig: () => wrap(App.ReadConfig()) as Promise<Record<string, any>>,
        applyConfig: target => act(App.ApplyConfig(target as any)),
        startCorrections: (src: CorrectionSource) => act(App.StartCorrections(src as any)),
        stopCorrections: () => act(App.StopCorrections()),
        decodePacket: (data: string, opts: DecodeOptions) =>
            wrap(App.DecodePacket(data, opts as any)),
        ecefToLLH: (x, y, z) => wrap(App.ECEFtoLLH(x, y, z)) as Promise<LLH>,
        llhToECEF: (lat, lon, height) =>
            wrap(App.LLHtoECEF(lat, lon, height)) as Promise<[number, number, number]>,
        checkOnEarth: (x, y, z) => wrap(App.CheckOnEarth(x, y, z)),
        velNEDtoECEF: (n, e, d) =>
            wrap(App.VelNEDtoECEF(n, e, d)) as Promise<[number, number, number] | null>,
        velECEFtoNED: (x, y, z) =>
            wrap(App.VelECEFtoNED(x, y, z)) as Promise<[number, number, number] | null>,
        eventsOn: (name, cb) => EventsOn(name, cb),
        openURL: url => BrowserOpenURL(url),
        connection: {
            connect: (device, speed) => act(App.Connect(device, speed)),
            disconnect: () => act(App.Disconnect()),
            listPorts: async () => ((await wrap(App.ListPorts())) ?? []) as PortInfo[],
        },
        msgFile: {
            loadMsgFile: async () =>
                ((await wrap(App.LoadMsgFile())) ?? null) as MsgFileInfo | null,
            sendMsgFile: (tag, port, save) => act(App.SendMsgFile(tag, port, save)),
            cancelMsgSend: () => act(App.CancelMsgSend()),
        },
    };
}

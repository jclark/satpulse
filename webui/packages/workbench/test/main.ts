import {h, render} from 'preact';
import {App} from '../src/app';
import {setTransport} from '../src/transport';
import type {Transport} from '../src/transport';
import '../src/style.css';

// This entry point is served by the existing Vite dev server only for tests.
// It mounts the real app against a mock transport, without a backend or WASM.
const listeners = new Map<string, Set<(data: unknown) => void>>();
const calls: {method: string; args: unknown[]}[] = [];
declare global {
    interface Window {
        workbenchTest: {
            calls: typeof calls;
            subscriptions(): string[];
        };
    }
}
window.workbenchTest = {calls, subscriptions: () => [...listeners.keys()]};

function record(method: string, ...args: unknown[]) {
    calls.push({method, args});
}
function emit(name: string, data: unknown) {
    for (const cb of listeners.get(name) ?? []) cb(data);
}

const transport: Transport = {
    getConnection: async () => ({state: 'connected', device: 'test'}),
    getReceiverState: async () => ({ok: false}),
    getAllSignals: async () => null,
    readConfig: async () => ({}),
    applyConfig: async () => {},
    decodePacket: async () => null,
    ecefToLLH: async () => ({lat: 0, lon: 0, height: 0}),
    llhToECEF: async () => [0, 0, 0],
    checkOnEarth: async () => true,
    velNEDtoECEF: async () => null,
    velECEFtoNED: async () => null,
    eventsOn: (name, cb) => {
        let set = listeners.get(name);
        if (!set) listeners.set(name, set = new Set());
        set.add(cb);
        return () => {
            set.delete(cb);
            if (!set.size) listeners.delete(name);
        };
    },
    openURL: () => {},
};

if (new URLSearchParams(location.search).has('corrections')) {
    transport.corrections = {
        getCorrectionsState: async () => {
            record('getCorrectionsState');
            return {state: 'stopped'};
        },
        startCorrections: async source => {
            record('startCorrections', source);
            emit('gps:corrections', {state: 'connected'});
        },
        stopCorrections: async () => {
            record('stopCorrections');
            emit('gps:corrections', {state: 'stopped'});
        },
    };
}

setTransport(transport);
render(h(App, {}), document.getElementById('app')!);

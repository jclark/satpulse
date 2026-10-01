import {h, render} from 'preact';
import {App} from '../src/app';
import {setTransport} from '../src/transport';
import type {PortInfo, Transport} from '../src/transport';
import '../src/style.css';

// This entry point is served by the existing Vite dev server only for tests.
// It mounts the real app against a mock transport, without a backend or WASM.
const listeners = new Map<string, Set<(data: unknown) => void>>();
const calls: {method: string; args: unknown[]}[] = [];
const params = new URLSearchParams(location.search);
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

if (params.has('corrections')) {
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

const picker = params.get('picker');
if (picker !== null) {
    let ports: PortInfo[] = [];
    transport.getConnection = async () => ({state: 'disconnected', device: params.get('device') ?? '', speed: 38400});
    transport.connection = {
        listPorts: async () => {
            record('listPorts');
            return ports;
        },
        connect: async (device, speed) => { record('connect', device, speed); },
        disconnect: async () => {},
    };
    if (picker !== 'none') {
        transport.connection.choosePort = async () => {
            record('choosePort');
            if (picker === 'cancel') return null;
            if (picker === 'error') throw new Error('Permission denied');
            const port = {device: 'serial:1', display: 'Receiver'};
            ports = [port];
            return port;
        };
    }
}

setTransport(transport);
render(h(App, {}), document.getElementById('app')!);

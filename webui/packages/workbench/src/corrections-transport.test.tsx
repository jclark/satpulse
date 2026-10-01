// @vitest-environment jsdom
import {h, render} from 'preact';
import {act} from 'preact/test-utils';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {App} from './app';
import {CorrectionsPanel} from './corrections-panel';
import {setTransport} from './transport';
import type {CorrectionsTransport, Transport} from './transport';

let root: HTMLDivElement;
let listeners: Map<string, Set<(data: any) => void>>;

function mockTransport(corrections?: CorrectionsTransport): Transport {
    return {
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
            return () => { set.delete(cb); };
        },
        openURL: () => {},
        corrections,
    };
}

function emit(name: string, data: unknown) {
    for (const cb of listeners.get(name) ?? []) cb(data);
}

function button(name: string): HTMLButtonElement | undefined {
    return [...root.querySelectorAll('button')].find(b => b.textContent?.trim() === name);
}

beforeEach(() => {
    root = document.createElement('div');
    document.body.append(root);
    listeners = new Map();
    localStorage.clear();
    vi.stubGlobal('ResizeObserver', class {
        observe() {}
        disconnect() {}
    });
});

afterEach(() => {
    act(() => render(null, root));
    root.remove();
    vi.unstubAllGlobals();
});

describe('optional corrections transport', () => {
    it('omits the corrections tab and panel without the capability', async () => {
        setTransport(mockTransport());
        await act(async () => render(<App/>, root));
        expect(button('Corrections')).toBeUndefined();
        expect(root.querySelector('input[placeholder="e.g. 10.0.0.1"]')).toBeNull();
        expect(listeners.get('gps:corrpacket')).toBeUndefined();
    });

    it('mounts the corrections tab and synchronizes its panel with the capability', async () => {
        const getCorrectionsState = vi.fn(async () => ({state: 'stopped'}));
        setTransport(mockTransport({
            getCorrectionsState,
            startCorrections: async () => {},
            stopCorrections: async () => {},
        }));
        await act(async () => render(<App/>, root));
        expect(button('Corrections')).toBeDefined();
        expect(root.querySelector('input[placeholder="e.g. 10.0.0.1"]')).not.toBeNull();
        await vi.waitFor(() => expect(getCorrectionsState).toHaveBeenCalledOnce());
        expect(listeners.get('gps:corrpacket')?.size).toBe(1);
    });

    it('starts and stops through the supplied capability', async () => {
        const corrections = {
            getCorrectionsState: vi.fn(async () => ({state: 'stopped'})),
            startCorrections: vi.fn(async () => emit('gps:corrections', {state: 'connected'})),
            stopCorrections: vi.fn(async () => emit('gps:corrections', {state: 'stopped'})),
        };
        setTransport(mockTransport());
        localStorage.setItem('corr-host', 'caster.test');
        localStorage.setItem('corr-mountpoint', 'TEST');
        await act(async () => render(<CorrectionsPanel corrections={corrections} connState="connected" readOnly={false}/>, root));
        expect(corrections.getCorrectionsState).toHaveBeenCalledOnce();
        await vi.waitFor(() => expect(button('Connect')?.disabled).toBe(false));
        await act(async () => button('Connect')!.click());
        expect(corrections.startCorrections).toHaveBeenCalledExactlyOnceWith({
            mode: 'ntrip', host: 'caster.test', port: 2101, mountpoint: 'TEST',
            username: '', password: '', nmeaSend: false,
        });
        await act(async () => button('Disconnect')!.click());
        expect(corrections.stopCorrections).toHaveBeenCalledOnce();
        await vi.waitFor(() => expect(button('Connect')?.disabled).toBe(false));
    });
});

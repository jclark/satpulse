import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {newWasmTransport} from './wasm-transport';
import {WebSerial} from './web-serial';

let resolveRun: () => void;
let rejectRun: (error: Error) => void;
let register: boolean;
const call = vi.fn();
const on = vi.fn();

beforeEach(() => {
    register = true;
    call.mockReset().mockResolvedValue('{}');
    on.mockReset();
    vi.stubGlobal('document', {baseURI: 'http://localhost/'});
    vi.stubGlobal('window', {location: {search: ''}});
    vi.stubGlobal('fetch', vi.fn(async () => ({ok: true})));
    vi.spyOn(WebAssembly, 'instantiateStreaming').mockResolvedValue({instance: {}} as WebAssembly.WebAssemblyInstantiatedSource);
    vi.spyOn(WebSerial.prototype, 'close').mockResolvedValue();
    vi.stubGlobal('satpulseWorkbench', {init: () => null, call, on});
    vi.stubGlobal('Go', class {
        importObject = {};
        run() {
            const running = new Promise<void>((resolve, reject) => { resolveRun = resolve; rejectRun = reject; });
            if (register) globalThis.satpulseWorkbenchReady();
            return running;
        }
    });
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

it('rejects startup when Go exits before registering the API', async () => {
    register = false;
    const fatal = vi.fn();
    const startup = newWasmTransport(fatal);
    const rejected = expect(startup).rejects.toThrow('startup failed');
    await vi.waitFor(() => expect(rejectRun).toBeDefined());
    rejectRun(new Error('startup failed'));
    await rejected;
    expect(fatal).not.toHaveBeenCalled();
    expect(WebSerial.prototype.close).toHaveBeenCalledOnce();
});

it.each(['resolve', 'reject'])('reports runtime %s after boot and rejects pending and future calls', async ending => {
    const fatal = vi.fn();
    const transport = await newWasmTransport(fatal);
    const off = transport.eventsOn('gps:state', () => {});
    call.mockImplementation(() => new Promise(() => {}));
    const pending = transport.getConnection();
    const rejected = expect(pending).rejects.toThrow(ending === 'resolve' ? 'Workbench stopped' : 'runtime failed');
    if (ending === 'resolve') resolveRun();
    else rejectRun(new Error('runtime failed'));
    await rejected;
    expect(fatal).toHaveBeenCalledOnce();
    expect(WebSerial.prototype.close).toHaveBeenCalledOnce();
    await expect(transport.getConnection()).rejects.toThrow();
    expect(call).toHaveBeenCalledOnce();
    off();
    expect(on).toHaveBeenCalledOnce();
});

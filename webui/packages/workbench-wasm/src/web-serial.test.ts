import {beforeEach, afterEach, expect, it, vi} from 'vitest';
import {WebSerial} from './web-serial';

function deferred() {
    let resolve!: () => void;
    const promise = new Promise<void>(r => { resolve = r; });
    return {promise, resolve};
}

class Port {
    private stream: ReadableStream<Uint8Array> | null = null;
    private replace = false;
    controller!: ReadableStreamDefaultController<Uint8Array>;
    writable: WritableStream<Uint8Array> | null = null;
    state = 'closed';
    opens: number[] = [];
    closes = 0;
    readers = 0;
    writes: number[][] = [];
    barrier?: Promise<void>;
    failOpen = false;

    get readable() {
        if (this.replace && !this.stream?.locked) {
            this.replace = false;
            this.newStream();
        }
        return this.stream;
    }

    getInfo() { return {}; }

    async open({baudRate}: {baudRate: number}) {
        if (this.state !== 'closed') throw new DOMException('Already opening or open', 'InvalidStateError');
        this.state = 'opening';
        this.opens.push(baudRate);
        await this.barrier;
        if (this.failOpen) {
            this.state = 'closed';
            throw new DOMException('Port busy', 'NetworkError');
        }
        this.state = 'open';
        this.newStream();
        this.writable = new WritableStream({write: b => { this.writes.push([...b]); }});
    }

    async close() {
        expect(this.stream?.locked ?? false).toBe(false);
        expect(this.writable?.locked).toBe(false);
        this.stream = null;
        this.writable = null;
        this.state = 'closed';
        this.closes++;
    }

    fault(name: string, temporary = true) {
        this.replace = temporary;
        this.controller.error(new DOMException('Serial fault', name));
        if (!temporary) this.stream = null;
    }

    private newStream() {
        this.stream = new ReadableStream({start: c => { this.controller = c; }});
        this.readers++;
    }
}

let port: Port;
let serial: WebSerial;

beforeEach(async () => {
    port = new Port();
    vi.stubGlobal('navigator', {serial: {getPorts: async () => [port]}});
    serial = new WebSerial();
    await serial.listPorts();
});

afterEach(async () => {
    await serial.close();
    vi.unstubAllGlobals();
});

it.each(['FramingError', 'ParityError', 'BreakError', 'BufferOverrunError'])('recovers %s and preserves byte/error ordering and the writer', async name => {
    const c = await serial.open('serial:1', 38400);
    port.controller.enqueue(new Uint8Array([1, 2]));
    await Promise.resolve();
    port.fault(name);
    await vi.waitFor(() => expect(port.readers).toBe(2));
    port.controller.enqueue(new Uint8Array([3]));
    expect([...(await c.read())!]).toEqual([1, 2]);
    await expect(c.read()).rejects.toMatchObject({name});
    expect([...(await c.read())!]).toEqual([3]);
    await c.write(new Uint8Array([4]));
    expect(port.writes).toEqual([[4]]);
    await c.close();
    expect(port.closes).toBe(1);
});

it('delivers buffered bytes before a terminal error', async () => {
    const c = await serial.open('serial:1', 38400);
    port.controller.enqueue(new Uint8Array([1, 2]));
    await Promise.resolve();
    port.fault('NetworkError', false);
    expect([...(await c.read())!]).toEqual([1, 2]);
    await expect(c.read()).rejects.toMatchObject({name: 'NetworkError'});
    await expect(c.read()).rejects.toMatchObject({name: 'NetworkError'});
});

it('holds ownership until an aborted delayed open is closed', async () => {
    const gate = deferred();
    port.barrier = gate.promise;
    const abort = new AbortController();
    const first = serial.open('serial:1', 38400, abort.signal);
    await vi.waitFor(() => expect(port.state).toBe('opening'));
    abort.abort();
    await expect(first).rejects.toMatchObject({name: 'AbortError'});
    const second = serial.open('serial:1', 115200);
    expect(port.opens).toEqual([38400]);
    gate.resolve();
    const c = await second;
    expect(port.opens).toEqual([38400, 115200]);
    expect(port.closes).toBe(1);
    await c.close();
    expect(port.closes).toBe(2);
});

it('an aborted queued attempt does not let a newer open bypass the owner', async () => {
    const first = await serial.open('serial:1', 38400);
    const abort = new AbortController();
    const second = serial.open('serial:1', 57600, abort.signal);
    abort.abort();
    await expect(second).rejects.toMatchObject({name: 'AbortError'});
    const third = serial.open('serial:1', 115200);
    expect(port.opens).toEqual([38400]);
    await first.close();
    await (await third).close();
    expect(port.opens).toEqual([38400, 115200]);
});

it('releases ownership after an open failure', async () => {
    port.failOpen = true;
    await expect(serial.open('serial:1', 38400)).rejects.toThrow('Port busy');
    port.failOpen = false;
    await (await serial.open('serial:1', 38400)).close();
    expect(port.closes).toBe(1);
});

it('close waits for a pending baud reopen before another connection opens', async () => {
    const first = await serial.open('serial:1', 38400);
    const gate = deferred();
    port.barrier = gate.promise;
    const changed = first.changeSpeed(57600);
    const rejected = expect(changed).rejects.toThrow('Serial port is closed.');
    await vi.waitFor(() => expect(port.state).toBe('opening'));
    const closed = first.close();
    const next = serial.open('serial:1', 115200);
    gate.resolve();
    await rejected;
    await closed;
    await (await next).close();
    expect(port.opens).toEqual([38400, 57600, 115200]);
    expect(port.closes).toBe(3);
});

it('adapter shutdown rejects queued opens and closes the active port', async () => {
    await serial.open('serial:1', 38400);
    const next = serial.open('serial:1', 115200);
    const rejected = expect(next).rejects.toThrow('Serial port is closed.');
    await serial.close();
    await rejected;
    expect(port.opens).toEqual([38400]);
    expect(port.closes).toBe(1);
    await expect(serial.open('serial:1', 38400)).rejects.toThrow('Serial port is closed.');
});

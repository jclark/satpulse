import type {PortInfo} from '@satpulse/workbench/src/transport';

// Only the part of Web Serial this shell uses. Stream objects and locks remain
// in JavaScript; bytes cross the wasm boundary once per received chunk.
interface BrowserPort {
    readable: ReadableStream<Uint8Array> | null;
    writable: WritableStream<Uint8Array> | null;
    open(options: {baudRate: number; bufferSize: number}): Promise<void>;
    close(): Promise<void>;
    getInfo(): {usbVendorId?: number; usbProductId?: number};
}

interface BrowserSerial {
    getPorts(): Promise<BrowserPort[]>;
    requestPort(): Promise<BrowserPort>;
}

function serialAPI(): BrowserSerial {
    const api = (navigator as Navigator & {serial?: BrowserSerial}).serial;
    if (!api) throw new Error('Web Serial requires a supported browser, such as Chrome or Edge, on HTTPS or localhost.');
    return api;
}

// Cancelling the wait cannot cancel Web Serial's underlying open operation.
// Its eventual completion must still be followed by close before reusing it.
async function waitFor<T>(p: Promise<T>, signal?: AbortSignal): Promise<T> {
    if (!signal) return p;
    signal.throwIfAborted();
    let abort!: () => void;
    const cancelled = new Promise<never>((_, reject) => {
        abort = () => reject(signal.reason);
        signal.addEventListener('abort', abort, {once: true});
    });
    try {
        return await Promise.race([p, cancelled]);
    } finally {
        signal.removeEventListener('abort', abort);
    }
}

export class WebSerial {
    private ports = new Map<string, BrowserPort>();
    private available = new Map<BrowserPort, Promise<void>>();
    private connections = new Set<SerialConnection>();
    private closed = false;

    async listPorts(): Promise<PortInfo[]> {
        return (await serialAPI().getPorts()).map(p => this.remember(p));
    }

    async choosePort(): Promise<PortInfo | null> {
        try {
            // requestPort must run before any await to preserve the gesture.
            return this.remember(await serialAPI().requestPort());
        } catch (e) {
            if (e instanceof DOMException && e.name === 'NotFoundError') return null;
            throw e;
        }
    }

    async open(device: string, speed: number, signal?: AbortSignal): Promise<SerialConnection> {
        if (this.closed) throw new Error('Serial port is closed.');
        const port = this.ports.get(device);
        if (!port) throw new Error('Choose a serial port first.');
        const previous = this.available.get(port) ?? Promise.resolve();
        let release!: () => void;
        const held = new Promise<void>(resolve => { release = resolve; });
        const available = previous.then(() => held);
        this.available.set(port, available);
        available.then(() => {
            if (this.available.get(port) === available) this.available.delete(port);
        });
        try {
            await waitFor(previous, signal);
            signal?.throwIfAborted();
            if (this.closed) throw new Error('Serial port is closed.');
        } catch (e) {
            release();
            throw e;
        }
        const c = new SerialConnection(port, speed, () => {
            this.connections.delete(c);
            release();
        });
        this.connections.add(c);
        await c.open(signal);
        return c;
    }

    async close(): Promise<void> {
        this.closed = true;
        await Promise.allSettled([...this.connections].map(c => c.close()));
    }

    private remember(port: BrowserPort): PortInfo {
        let device = [...this.ports].find(([, p]) => p === port)?.[0];
        if (!device) {
            device = `serial:${this.ports.size + 1}`;
            this.ports.set(device, port);
        }
        const info = port.getInfo();
        const usb = info.usbVendorId === undefined ? ''
            : ` (USB ${info.usbVendorId.toString(16).padStart(4, '0')}:${(info.usbProductId ?? 0).toString(16).padStart(4, '0')})`;
        return {device, display: `Port ${device.split(':')[1]}${usb}`};
    }
}

const temporaryErrors = new Set(['FramingError', 'ParityError', 'BreakError', 'BufferOverrunError']);

export class SerialConnection {
    private reader?: ReadableStreamDefaultReader<Uint8Array>;
    private writer?: WritableStreamDefaultWriter<Uint8Array>;
    private pumping?: Promise<void>;
    private opening?: Promise<void>;
    private changingSpeed?: Promise<void>;
    private queue: (Uint8Array | Error)[] = [];
    private queued = 0;
    private wake?: () => void;
    private stopped = false;
    private ended = false;
    private changing = false;
    private error?: Error;
    private transmitUntil = 0;
    private closing?: Promise<void>;
    private opened = false;

    constructor(private port: BrowserPort, private speed: number, private released: () => void) {}

    async open(signal?: AbortSignal): Promise<void> {
        this.opening = this.port.open({baudRate: this.speed, bufferSize: 65536}).then(() => {
            this.opened = true;
            if (!this.stopped) this.startStreams();
        });
        try {
            await waitFor(this.opening, signal);
        } catch (e) {
            // Keep ownership until a late open has completed and been closed.
            void this.close().catch(() => {});
            throw e;
        }
    }

    async read(): Promise<Uint8Array | null> {
        if (!this.queue.length && !this.stopped && !this.ended && !this.error) {
            await new Promise<void>(resolve => {
                const timer = setTimeout(() => { this.wake = undefined; resolve(); }, 100);
                this.wake = () => { clearTimeout(timer); this.wake = undefined; resolve(); };
            });
        }
        if (this.stopped) return new Uint8Array();
        const next = this.queue.shift();
        if (next instanceof Error) throw next;
        if (next) {
            this.queued -= next.length;
            return next;
        }
        if (this.error) throw this.error;
        return this.ended ? new Uint8Array() : null;
    }

    async write(b: Uint8Array): Promise<void> {
        if (this.stopped || !this.writer) throw new Error('Serial port is closed.');
        await this.writer.write(b);
        // Web Serial has no tcdrain. Keep a conservative 8N1 transmit estimate.
        this.transmitUntil = Math.max(performance.now(), this.transmitUntil) + b.length * 10000 / this.speed;
    }

    async drain(): Promise<void> {
        const delay = this.transmitUntil - performance.now();
        if (delay > 0) await new Promise(resolve => setTimeout(resolve, delay));
    }

    changeSpeed(speed: number): Promise<void> {
        return this.changingSpeed = this.reopen(speed);
    }

    private async reopen(speed: number): Promise<void> {
        this.changing = true;
        try {
            await this.drain();
            await this.closeStreams();
            if (this.stopped) throw new Error('Serial port is closed.');
            this.queue = [];
            this.queued = 0;
            await this.port.open({baudRate: speed, bufferSize: 65536});
            this.opened = true;
            if (this.stopped) {
                await this.closeStreams();
                throw new Error('Serial port is closed.');
            }
            this.speed = speed;
            this.transmitUntil = 0;
            this.changing = false;
            this.startStreams();
        } catch (e) {
            this.error = e instanceof Error ? e : new Error(String(e));
            throw this.error;
        } finally {
            this.changing = false;
            this.wake?.();
        }
    }

    stop(): void {
        this.stopped = true;
        this.wake?.();
    }

    close(): Promise<void> {
        this.stop();
        return this.closing ??= (async () => {
            try {
                await this.opening?.catch(() => {});
                await this.changingSpeed?.catch(() => {});
                await this.closeStreams();
            } finally {
                this.released();
            }
        })();
    }

    private startStreams(): void {
        if (!this.port.readable || !this.port.writable) throw new Error('Serial port has no readable or writable stream.');
        this.ended = false;
        this.error = undefined;
        this.writer = this.port.writable.getWriter();
        this.pumping = this.pump();
    }

    private async pump(): Promise<void> {
        while (!this.stopped && !this.changing) {
            const stream = this.port.readable;
            if (!stream) { this.ended = true; break; }
            let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
            let retry = false;
            let readError: Error | undefined;
            try {
                reader = stream.getReader();
                this.reader = reader;
                while (true) {
                    const {value, done} = await reader.read();
                    if (done) { this.ended = !this.changing; break; }
                    if (value?.length) {
                        if (this.queued + value.length > 1024 * 1024 || this.queue.length >= 4096) throw new Error('Serial input buffer overflow.');
                        this.queue.push(value);
                        this.queued += value.length;
                        this.wake?.();
                    }
                }
            } catch (e) {
                if (!this.stopped && !this.changing) {
                    const error = e instanceof Error ? e : new Error(String(e));
                    readError = error;
                    retry = temporaryErrors.has(error.name);
                    if (this.queue.length >= 4096) {
                        this.error = new Error('Serial input buffer overflow.');
                        retry = false;
                    } else {
                        this.queue.push(error);
                        if (!retry) this.error = error;
                    }
                }
            } finally {
                reader?.releaseLock();
                this.reader = undefined;
                this.wake?.();
            }
            if (retry && (!this.port.readable || this.port.readable === stream)) {
                this.error = new Error(readError!.message);
                retry = false;
            }
            if (!retry) break;
        }
    }

    private async closeStreams(): Promise<void> {
        if (!this.opened) return;
        const reader = this.reader;
        const writer = this.writer;
        try {
            await reader?.cancel().catch(() => {});
            await this.pumping;
            await writer?.close().catch(() => {});
        } finally {
            writer?.releaseLock();
            this.reader = undefined;
            this.writer = undefined;
            try {
                await this.port.close();
            } finally {
                this.opened = false;
            }
        }
    }
}

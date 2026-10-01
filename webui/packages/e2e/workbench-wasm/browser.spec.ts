import {test, expect} from '@playwright/test';
import {spawn, ChildProcessWithoutNullStreams} from 'node:child_process';
import * as fs from 'node:fs';
import * as http from 'node:http';
import * as path from 'node:path';

const repo = path.resolve(__dirname, '../../../..');
const assets = path.join(repo, 'out/workbench-wasm');
let server: http.Server;
let baseURL: string;

test.beforeAll(async () => {
    const files = new Map(fs.readdirSync(assets).filter(name => !/\.(br|gz|json)$/.test(name) && fs.statSync(path.join(assets, name)).isFile()).map(name => {
        const b = fs.readFileSync(path.join(assets, name));
        return [name, b];
    }));
    server = http.createServer((req, res) => {
        const name = (req.url ?? '').split('?')[0].replace(/^\/browser\//, '') || 'index.html';
        if (!/^[\w.-]+$/.test(name)) { res.writeHead(404).end(); return; }
        try {
            const file = files.get(name);
            if (!file) { res.writeHead(404).end(); return; }
            const b = file;
            const types: Record<string, string> = {'.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.wasm': 'application/wasm'};
            res.setHeader('Content-Type', types[path.extname(name)] ?? 'application/octet-stream');
            res.setHeader('Cache-Control', 'no-store');
            res.end(b);
        } catch { res.writeHead(404).end(); }
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    const address = server.address() as {port: number};
    baseURL = `http://127.0.0.1:${address.port}/browser/`;
});

test.afterAll(async () => {
    await new Promise<void>(resolve => server.close(() => resolve()));
});

test('browser wasm runs the session, configuration, messages and serial lifecycle', async ({page}) => {
    const errors: string[] = [];
    let sim: ChildProcessWithoutNullStreams | undefined;
    let pending: number[][] = [];
    let forwarding = false;
    let stderr = '';
    page.on('pageerror', e => errors.push(e.message));
    await page.exposeFunction('simOpen', () => {
        if (!sim) {
            sim = spawn(path.join(repo, 'out/wasm-test/wasmsim'), [
                path.join(repo, 'gps/app/ubxsim/testdata/f9p/f9p-personality.ubx'),
                path.join(repo, 'gps/testdata/config/u-blox/ZED-F9P/sim.jsonl'),
            ]);
            sim.stderr.on('data', b => { stderr += b.toString(); });
            sim.stdout.on('data', b => {
                const bytes = [...b];
                if (forwarding) {
                    page.evaluate(b => (window as any).serialInput(b), bytes).catch(() => {});
                } else pending.push(bytes);
            });
        }
        forwarding = true;
        for (const b of pending) page.evaluate(b => (window as any).serialInput(b), b).catch(() => {});
        pending = [];
    });
    await page.exposeFunction('simWrite', (b: number[]) => new Promise<void>((resolve, reject) => {
        sim!.stdin.write(Buffer.from(b), err => err ? reject(err) : resolve());
    }));
    await page.exposeFunction('simClose', () => { forwarding = false; });
    await page.addInitScript(() => {
        const w = window as any;
        const original = WebAssembly.instantiateStreaming;
        WebAssembly.instantiateStreaming = (async (...args: any[]) => {
            const result = await (original as any)(...args);
            w.wasmMemory = result.instance.exports.mem;
            return result;
        }) as typeof WebAssembly.instantiateStreaming;
        let controller: ReadableStreamDefaultController<Uint8Array>;
        w.serialInput = (b: number[]) => controller?.enqueue(new Uint8Array(b));
        const port = {
            readable: null as ReadableStream<Uint8Array> | null,
            writable: null as WritableStream<Uint8Array> | null,
            opens: [] as number[],
            closes: 0,
            pickerGestures: [] as boolean[],
            getInfo: () => ({usbVendorId: 0x1546, usbProductId: 0x01a9}),
            async open(options: {baudRate: number}) {
                if (this.readable || this.writable) throw new Error('Port already open');
                this.opens.push(options.baudRate);
                this.readable = new ReadableStream({start(c) { controller = c; }});
                this.writable = new WritableStream({write(b) { return w.simWrite([...b]); }});
                await w.simOpen();
            },
            async close() {
                if (this.readable?.locked || this.writable?.locked) throw new Error('Port streams still locked');
                this.readable = null;
                this.writable = null;
                this.closes++;
                await w.simClose();
            },
        };
        w.serialFault = (name: string) => {
            controller.error(new DOMException('Injected serial fault', name));
            port.readable = new ReadableStream({start(c) { controller = c; }});
        };
        let granted = false;
        Object.defineProperty(navigator, 'serial', {value: {
            getPorts: async () => granted ? [port] : [],
            requestPort: async () => {
                port.pickerGestures.push(navigator.userActivation.isActive);
                granted = true;
                return port;
            },
        }});
        w.testPort = port;
    });

    try {
        const started = Date.now();
        await page.goto(baseURL);
        await expect(page.getByRole('banner')).toBeVisible();
        const startup = Date.now() - started;
        console.log(`wasm browser startup: ${startup} ms`);
        await expect(page.getByRole('button', {name: 'Corrections', exact: true})).toHaveCount(0);
        const api = async (name: string, data: unknown = {}) => page.evaluate(async ({name, data}) =>
            JSON.parse(await (window as any).satpulseWorkbench.call(name, JSON.stringify(data))), {name, data});
        expect((await api('connection')).state).toBe('disconnected');
        expect(await api('geo/llh-to-ecef', [0, 0, 0])).toEqual([6378137, 0, 0]);
        await expect(page.getByRole('button', {name: 'Connect', exact: true})).toBeDisabled();
        await expect(page.getByRole('button', {name: 'Choose port...'})).toHaveCount(0);
        await expect(page.getByRole('button', {name: 'Add a device...'})).toHaveCount(0);
        await page.getByRole('button', {name: 'Select port'}).click();
        await expect(page.getByText('No devices added', {exact: true})).toBeVisible();
        await page.screenshot({path: path.join(repo, 'out/wasm-test/device-picker.png')});
        await page.getByRole('button', {name: 'Add a device...'}).click();
        await expect(page.getByRole('button', {name: 'Add a device...'})).toHaveCount(0);
        await expect(page.getByRole('button', {name: 'Connect', exact: true})).toBeEnabled();
        await page.getByRole('button', {name: 'Select port'}).click();
        await page.getByText('Port 1 (USB 1546:01a9)', {exact: true}).click();
        await expect(page.getByRole('button', {name: 'Add a device...'})).toHaveCount(0);
        expect(await page.evaluate(() => (window as any).testPort.pickerGestures)).toEqual([true]);
        await page.getByRole('banner').getByRole('combobox').selectOption('38400');
        await page.getByRole('button', {name: 'Connect', exact: true}).click();
        await expect(page.getByText('u-blox ZED-F9P (FW HPG 1.51 PROTVER 27.50)', {exact: true})).toBeVisible();
        await expect(page.getByText('Connected', {exact: true})).toBeVisible();

        for (const name of ['FramingError', 'ParityError', 'BreakError', 'BufferOverrunError']) {
            await page.evaluate(name => (window as any).serialFault(name), name);
            expect(await api('config/read')).toBeTruthy();
            expect((await api('connection')).state).toBe('connected');
        }

        const config = await api('config/read');
        expect(config).toBeTruthy();
        console.log(`config readback keys: ${Object.keys(config).join(', ')}`);
        await api('config/apply', {Props: {minElevation: 10}, Opts: {PVTMsg: ['pos', 'vel', 'time', 'quality', 'epoch']}});
        expect((await api('config/read')).minElevation).toBe(10);
        const catalog = await api('msgfile/catalog');
        expect(catalog.names.length).toBeGreaterThan(10);
        const file = catalog.names.find((v: any) => v.vendor === 'u-blox' && v.file === 'gen9');
        expect(file).toBeTruthy();
        const selected = await api('msgfile/select', {vendor: file.vendor, file: file.file});
        expect(selected.tags.length).toBeGreaterThan(0);
        console.log(`message catalog: ${catalog.names.length} files; gen9: ${selected.tags.length} tags`);
        await api('msgfile/send', {tag: 'ubx-nav-timegps', port: 'uart1', save: false});
        await expect(page.getByText('Connected', {exact: true})).toBeVisible();

        await page.getByRole('button', {name: 'Packets', exact: true}).click();
        const pvt = page.getByRole('cell', {name: 'NAV-PVT', exact: true});
        await expect(pvt).toBeVisible();
        await pvt.click();
        await expect(page.locator('pre')).toContainText('iTOW');
        const telemetry = await page.evaluate(() => ({
            memory: (window as any).wasmMemory.buffer.byteLength,
            opens: (window as any).testPort.opens,
        }));
        console.log(`wasm telemetry: ${JSON.stringify(telemetry)}`);
        await api('config/apply', {Props: {baudRate: 115200}});
        expect((await api('connection')).speed).toBe(115200);
        expect(await page.evaluate(() => (window as any).testPort.opens)).toEqual([38400, 115200]);
        await page.screenshot({path: path.join(repo, 'out/wasm-test/workbench.png'), fullPage: true});

        for (let i = 0; i < 3; i++) {
            await page.getByRole('button', {name: 'Disconnect', exact: true}).click();
            await expect(page.getByText('Disconnected', {exact: true})).toBeVisible();
            await page.getByRole('button', {name: 'Connect', exact: true}).click();
            await expect(page.getByText('Connected', {exact: true})).toBeVisible();
        }
        await page.getByRole('button', {name: 'Disconnect', exact: true}).click();
        await expect(page.getByText('Disconnected', {exact: true})).toBeVisible();
        expect(await page.evaluate(() => (window as any).testPort.closes)).toBe(5);
        expect(stderr).toBe('');
        expect(errors).toEqual([]);
    } finally {
        sim?.kill('SIGTERM');
    }
});

test('vendor URL enables Zhongke probes on every connection and rejects unknown vendors', async ({page}) => {
    await page.addInitScript(() => {
        const port = {
            readable: null as ReadableStream<Uint8Array> | null,
            writable: null as WritableStream<Uint8Array> | null,
            writes: [] as number[][],
            getInfo: () => ({}),
            async open() {
                this.readable = new ReadableStream();
                this.writable = new WritableStream({write: b => { this.writes.push([...b]); }});
            },
            async close() {
                if (this.readable?.locked || this.writable?.locked) throw new Error('Stream lock leaked');
                this.readable = null;
                this.writable = null;
            },
        };
        Object.defineProperty(navigator, 'serial', {value: {
            getPorts: async () => [port],
            requestPort: async () => port,
        }});
        (window as any).testPort = port;
    });
    for (const vendor of ['zHoNgKe', 'CASIC']) {
        await page.goto(`${baseURL}?vendor=${vendor}`);
        await expect(page.getByRole('banner')).toBeVisible();
        expect(await page.evaluate(async () => JSON.parse(await (window as any).satpulseWorkbench.call('msgfile/catalog', '{}')).preselect)).toBe('zhongke');
        for (let i = 0; i < 2; i++) {
            await page.evaluate(() => { (window as any).testPort.writes = []; });
            await page.getByRole('button', {name: 'Connect', exact: true}).click();
            // CFG-RATE is CASIC's identifying poll. Default probing excludes it.
            await expect.poll(() => page.evaluate(() => (window as any).testPort.writes.some((b: number[]) =>
                b[0] === 0xba && b[1] === 0xce && b[4] === 0x06 && b[5] === 0x04))).toBe(true);
            expect(await page.evaluate(() => (window as any).testPort.writes.some((b: number[]) => b[0] === 0xb5 && b[1] === 0x62))).toBe(false);
            await page.getByRole('button', {name: 'Disconnect', exact: true}).click();
            await expect(page.getByText('Disconnected', {exact: true})).toBeVisible();
        }
    }
    await page.goto(`${baseURL}?vendor=not-a-vendor`);
    await expect(page.getByText('unknown vendor: "not-a-vendor"', {exact: true})).toBeVisible();
    await expect(page.getByRole('banner')).toHaveCount(0);
    expect(await page.evaluate(() => (window as any).testPort.writes)).toEqual([]);
});

test('picker cancellation, open failure, pending read and stream errors clean up', async ({page}) => {
    await page.addInitScript(() => {
        const w = window as any;
        let api: any;
        Object.defineProperty(window, 'satpulseWorkbench', {
            get: () => api,
            set(value) {
                api = value;
                const init = api.init;
                api.init = (serial: any, vendor: string) => { w.testSerial = serial; return init(serial, vendor); };
            },
        });
        const port = {
            readable: null as ReadableStream<Uint8Array> | null,
            writable: null as WritableStream<Uint8Array> | null,
            mode: 'normal', closes: 0, controller: null as ReadableStreamDefaultController<Uint8Array> | null,
            getInfo: () => ({}),
            async open() {
                if (this.mode === 'fail') throw new DOMException('Port busy', 'NetworkError');
                this.readable = new ReadableStream({start: c => { this.controller = c; }});
                this.writable = new WritableStream();
            },
            async close() {
                if (!this.readable || !this.writable) throw new Error('Port already closed');
                if (this.readable.locked || this.writable.locked) throw new Error('Stream lock leaked');
                this.readable = null;
                this.writable = null;
                this.closes++;
            },
        };
        let granted = false;
        Object.defineProperty(navigator, 'serial', {value: {
            getPorts: async () => granted ? [port] : [],
            requestPort: async () => {
                if (port.mode === 'cancel') throw new DOMException('Cancelled', 'NotFoundError');
                granted = true;
                return port;
            },
        }});
        w.testPort = port;
    });
    await page.goto(baseURL);
    await expect(page.getByRole('banner')).toBeVisible();
    const choose = async () => {
        await page.getByRole('button', {name: 'Select port'}).click();
        await page.getByRole('button', {name: 'Add a device...'}).click();
        await expect(page.getByRole('button', {name: 'Add a device...'})).toHaveCount(0);
    };
    await page.evaluate(() => { (window as any).testPort.mode = 'cancel'; });
    await choose();
    await expect(page.getByRole('button', {name: 'Connect', exact: true})).toBeDisabled();
    await page.evaluate(() => { (window as any).testPort.mode = 'normal'; });
    await choose();
    await page.evaluate(() => { (window as any).testPort.mode = 'cancel'; });
    await choose();
    await expect(page.getByPlaceholder('select a device')).toHaveValue('serial:1');
    await expect(page.getByRole('button', {name: 'Connect', exact: true})).toBeEnabled();
    await page.evaluate(() => { (window as any).testPort.mode = 'fail'; });
    await page.getByRole('button', {name: 'Connect', exact: true}).click();
    await expect(page.getByText('Port busy', {exact: true})).toBeVisible();
    await expect(page.getByText('Disconnected', {exact: true})).toBeVisible();
    await page.evaluate(() => { (window as any).testPort.mode = 'normal'; });
    await page.getByRole('button', {name: 'Connect', exact: true}).click();
    await page.getByRole('button', {name: 'Disconnect', exact: true}).click();
    await expect(page.getByText('Disconnected', {exact: true})).toBeVisible();
    expect(await page.evaluate(() => (window as any).testPort.closes)).toBe(1);

    const results = await page.evaluate(async () => {
        const w = window as any;
        const serial = w.testSerial;
        const c = await serial.open('serial:1', 38400);
        const timeout = await c.read();
        w.testPort.controller.enqueue(new Uint8Array([1, 2, 3]));
        const bytes = [...await c.read()];
        const pending = c.read();
        c.stop();
        const stopped = [...await pending];
        await c.close();
        await c.close();
        const broken = await serial.open('serial:1', 38400);
        w.testPort.controller.error(new DOMException('Device removed', 'NetworkError'));
        const removed = await broken.read().catch((e: Error) => e.message);
        await broken.close();
        const changing = await serial.open('serial:1', 38400);
        w.testPort.mode = 'fail';
        const reopen = await changing.changeSpeed(115200).catch((e: Error) => e.message);
        await changing.close();
        w.testPort.mode = 'normal';
        const overflow = await serial.open('serial:1', 38400);
        w.testPort.controller.enqueue(new Uint8Array(1024 * 1024 + 1));
        const full = await overflow.read().catch((e: Error) => e.message);
        await overflow.close();
        return {timeout, bytes, stopped, removed, reopen, full, closes: w.testPort.closes};
    });
    expect(results).toEqual({timeout: null, bytes: [1, 2, 3], stopped: [], removed: 'Device removed', reopen: 'Port busy', full: 'Serial input buffer overflow.', closes: 5});
});

test('a newer Connect waits for the superseded port open to close', async ({page}) => {
    await page.addInitScript(() => {
        const w = window as any;
        const port = {
            readable: null as ReadableStream<Uint8Array> | null,
            writable: null as WritableStream<Uint8Array> | null,
            state: 'closed', opens: [] as number[], closes: 0,
            getInfo: () => ({}),
            async open({baudRate}: {baudRate: number}) {
                if (this.state !== 'closed') throw new DOMException('Port busy', 'InvalidStateError');
                this.state = 'opening';
                this.opens.push(baudRate);
                if (this.opens.length === 1) await new Promise<void>(resolve => { w.finishOpen = resolve; });
                this.state = 'open';
                this.readable = new ReadableStream();
                this.writable = new WritableStream();
            },
            async close() {
                if (this.readable?.locked || this.writable?.locked) throw new Error('Stream lock leaked');
                this.readable = null;
                this.writable = null;
                this.state = 'closed';
                this.closes++;
            },
        };
        w.testPort = port;
        Object.defineProperty(navigator, 'serial', {value: {getPorts: async () => [port]}});
    });
    await page.goto(baseURL);
    await expect(page.getByRole('banner')).toBeVisible();
    await page.evaluate(() => {
        const w = window as any;
        w.first = w.satpulseWorkbench.call('connect', JSON.stringify({device: 'serial:1', speed: 38400})).catch((e: Error) => e.message);
    });
    await expect.poll(() => page.evaluate(() => (window as any).testPort.opens)).toEqual([38400]);
    await page.evaluate(() => {
        const w = window as any;
        w.second = w.satpulseWorkbench.call('connect', JSON.stringify({device: 'serial:1', speed: 115200})).catch((e: Error) => e.message);
    });
    await page.evaluate(() => (window as any).finishOpen());
    const results = await page.evaluate(async () => {
        const w = window as any;
        return {first: await w.first, second: await w.second, opens: w.testPort.opens, closes: w.testPort.closes};
    });
    expect(results).toEqual({first: 'connection attempt superseded', second: 'null', opens: [38400, 115200], closes: 1});
    await page.evaluate(() => (window as any).satpulseWorkbench.call('disconnect', '{}'));
    expect(await page.evaluate(() => (window as any).testPort.closes)).toBe(2);
});

test('runtime exit after boot replaces the UI with a visible error', async ({page}) => {
    await page.addInitScript(() => {
        const w = window as any;
        let runtime: any;
        Object.defineProperty(window, 'Go', {
            get: () => runtime,
            set(Go) {
                runtime = class extends Go {
                    run(instance: WebAssembly.Instance) {
                        const exited = new Promise<void>(resolve => { w.exitRuntime = resolve; });
                        return Promise.race([super.run(instance), exited]);
                    }
                };
            },
        });
        Object.defineProperty(navigator, 'serial', {value: {getPorts: async () => []}});
    });
    const errors: string[] = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto(baseURL);
    await expect(page.getByRole('banner')).toBeVisible();
    await page.evaluate(() => (window as any).exitRuntime());
    await expect(page.getByText('Workbench stopped. Reload the page to restart.', {exact: true})).toBeVisible();
    await expect(page.getByRole('banner')).toHaveCount(0);
    expect(errors).toEqual([]);
});

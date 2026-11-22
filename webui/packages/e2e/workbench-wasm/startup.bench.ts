import {test, expect} from '@playwright/test';
import * as fs from 'node:fs';
import * as http from 'node:http';
import * as path from 'node:path';
import {gzipSync, brotliCompressSync} from 'node:zlib';

const repo = path.resolve(__dirname, '../../../..');
const assets = path.join(repo, 'out/workbench-wasm');
let server: http.Server;
let baseURL: string;

test.beforeAll(async () => {
    const files = new Map(fs.readdirSync(assets).filter(name => !/\.(br|gz|json)$/.test(name) && fs.statSync(path.join(assets, name)).isFile()).map(name => {
        const b = fs.readFileSync(path.join(assets, name));
        return [name, {raw: b, br: brotliCompressSync(b), gzip: gzipSync(b)}];
    }));
    server = http.createServer((req, res) => {
        const name = (req.url ?? '').split('?')[0].replace(/^\/browser\//, '') || 'index.html';
        if (!/^[\w.-]+$/.test(name)) { res.writeHead(404).end(); return; }
        try {
            const file = files.get(name);
            if (!file) { res.writeHead(404).end(); return; }
            let b = file.raw;
            const types: Record<string, string> = {'.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.wasm': 'application/wasm'};
            res.setHeader('Content-Type', types[path.extname(name)] ?? 'application/octet-stream');
            res.setHeader('Cache-Control', 'no-store');
            const encoding = req.headers['accept-encoding'] ?? '';
            if (encoding.includes('br')) { b = file.br; res.setHeader('Content-Encoding', 'br'); }
            else if (encoding.includes('gzip')) { b = file.gzip; res.setHeader('Content-Encoding', 'gzip'); }
            res.end(b);
        } catch { res.writeHead(404).end(); }
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    const address = server.address() as {port: number};
    baseURL = `http://127.0.0.1:${address.port}/browser/`;
});

test('measure cold startup over local and throttled compressed delivery', async ({browser}) => {
    const results = [];
    for (const [encoding, mbps] of [['br', 0], ['br', 10], ['gzip', 10], ['br', 2]] as const) {
        const context = await browser.newContext({extraHTTPHeaders: {'Accept-Encoding': encoding}});
        const page = await context.newPage();
        const cdp = await context.newCDPSession(page);
        await cdp.send('Network.enable');
        await cdp.send('Network.setCacheDisabled', {cacheDisabled: true});
        if (mbps) await cdp.send('Network.emulateNetworkConditions', {
            offline: false, latency: 50, downloadThroughput: mbps * 1e6 / 8, uploadThroughput: mbps * 1e6 / 8,
        });
        const started = Date.now();
        await page.goto(baseURL);
        await expect(page.getByRole('banner')).toBeVisible();
        const elapsed = Date.now() - started;
        const transfer = await page.evaluate(() => {
            const p = performance.getEntriesByType('resource').find(p => p.name.endsWith('workbench.wasm')) as PerformanceResourceTiming;
            return {encoded: p.encodedBodySize, decoded: p.decodedBodySize, downloadMs: Math.round(p.duration)};
        });
        results.push({encoding, mbps, elapsed, ...transfer});
        await context.close();
    }
    console.log(`cold startup measurements: ${JSON.stringify(results)}`);
    fs.writeFileSync(path.join(repo, 'out/wasm-test/startup.json'), JSON.stringify(results, null, 2) + '\n');
});

test.afterAll(async () => {
    await new Promise<void>(resolve => server.close(() => resolve()));
});

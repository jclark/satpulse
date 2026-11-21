// Precompress static assets once; serving must set Content-Encoding and retain
// the original Content-Type (application/wasm for the module).
import {readdirSync, readFileSync, writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {gzipSync, brotliCompressSync, constants} from 'node:zlib';

const dir = process.argv[2] ?? 'out/workbench-wasm';
const sizes = [];
for (const file of readdirSync(dir).filter(n => /\.(wasm|js|css|html)$/.test(n))) {
    const b = readFileSync(join(dir, file));
    const gzip = gzipSync(b, {level: 9});
    const br = brotliCompressSync(b, {params: {[constants.BROTLI_PARAM_QUALITY]: 11}});
    writeFileSync(join(dir, file + '.gz'), gzip);
    writeFileSync(join(dir, file + '.br'), br);
    sizes.push({file, raw: b.length, gzip: gzip.length, brotli: br.length});
}
writeFileSync(join(dir, 'sizes.json'), JSON.stringify(sizes, null, 2) + '\n');
console.table(sizes);

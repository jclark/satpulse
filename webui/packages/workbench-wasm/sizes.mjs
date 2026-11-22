// Print the raw, gzip and Brotli size of each built asset, to track how much
// a browser downloads. Nothing is written: the compression is only measured.
import {readdirSync, readFileSync} from 'node:fs';
import {join} from 'node:path';
import {gzipSync, brotliCompressSync, constants} from 'node:zlib';

const dir = process.argv[2] ?? 'out/workbench-wasm';
const sizes = [];
for (const file of readdirSync(dir).filter(n => /\.(wasm|js|css|html)$/.test(n))) {
    const b = readFileSync(join(dir, file));
    const gzip = gzipSync(b, {level: 9}).length;
    const brotli = brotliCompressSync(b, {params: {[constants.BROTLI_PARAM_QUALITY]: 11}}).length;
    sizes.push({file, raw: b.length, gzip, brotli});
}
console.table(sizes);

import {defineConfig} from '@playwright/test';
import config from './playwright.config';

export default defineConfig({
    ...config,
    projects: [{name: 'workbench-wasm-benchmark', testDir: './workbench-wasm', testMatch: '**/*.bench.ts'}],
});

import {h, render} from 'preact';
import {App} from '@satpulse/workbench/src/app';
import {setTransport} from '@satpulse/workbench/src/transport';
import {newWasmTransport} from './wasm-transport';
import '@satpulse/workbench/src/style.css';

const root = document.getElementById('app')!;
render(<p class="p-5 text-text-secondary">Loading SatPulse Workbench...</p>, root);
boot();

async function boot() {
    let failed = false;
    const showError = (e: unknown) => {
        failed = true;
        render(<p class="p-5 text-danger">{e instanceof Error ? e.message : String(e)}</p>, root);
    };
    try {
        if (!window.isSecureContext) throw new Error('Web Serial requires HTTPS or localhost.');
        if (!('serial' in navigator)) throw new Error('Web Serial is unavailable. Open Workbench in a supported browser, such as Chrome or Edge.');
        const t = await newWasmTransport(showError);
        if (failed) return;
        setTransport(t);
        render(<App/>, root);
    } catch (e) {
        showError(e);
    }
}

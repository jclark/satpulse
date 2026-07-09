import {h, render} from 'preact';
import {App} from '@satpulse/workbench/src/app';
import {setTransport} from '@satpulse/workbench/src/transport';
import {newHTTPTransport, HttpError} from './http-transport';
import '@satpulse/workbench/src/style.css';

// The printed URL carries the per-run access token as ?t=. Store it and
// strip it from the URL bar; a tab opened without it (reload, second
// tab) uses the stored one.
const url = new URL(window.location.href);
let token = url.searchParams.get('t');
if (token) {
    localStorage.setItem('satpulsewb-token', token);
    url.searchParams.delete('t');
    history.replaceState(null, '', url);
} else {
    token = localStorage.getItem('satpulsewb-token');
}

const root = document.getElementById('app')!;

// A stored token from a previous run is stale after a restart (which
// generates a fresh token). Validate it with a snapshot request before
// mounting the app: a 401 means the token is wrong, so show a notice
// pointing at the newly printed URL rather than a silently dead UI.
// Any other failure (server still starting) falls through to the app,
// whose own retry handles recovery.
const t = newHTTPTransport(token || '');
t.getConnState().then(mount).catch(err => {
    if (err instanceof HttpError && err.status === 401) {
        localStorage.removeItem('satpulsewb-token');
        render(<AuthNotice/>, root);
    } else {
        mount();
    }
});

function mount() {
    setTransport(t);
    render(<App/>, root);
}

function AuthNotice() {
    return (
        <div style="max-width:36rem;margin:4rem auto;padding:0 1.5rem;font-family:system-ui,sans-serif;line-height:1.5">
            <h1 style="font-size:1.25rem;margin-bottom:0.75rem">SatPulse Workbench</h1>
            <p>The access token for this page is missing or no longer valid.</p>
            <p>If satpulsewb was restarted, it printed a new URL in its terminal; open that URL to reconnect.</p>
        </div>
    );
}

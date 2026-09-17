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
// A failed seat claim is retried while the server becomes reachable;
// later snapshot failures fall through to the app's own recovery.
render(<ConnectingNotice/>, root);
boot();

async function boot() {
    let t;
    try {
        t = await newHTTPTransport(token || '');
    } catch (err) {
        if (showAuthError(err)) return;
        window.setTimeout(boot, 1000);
        return;
    }
    try {
        await t.getConnection();
    } catch (err) {
        if (showAuthError(err)) return;
    }
    setTransport(t);
    // A live token goes stale if satpulsewb restarts. The transport reports it
    // via wb:authlost; show the same notice as a stale token at boot, directly
    // rather than reloading (a reload would read the shared token store and
    // could reclaim a seat another tab just took).
    t.eventsOn('wb:authlost', () => render(<AuthNotice/>, root));
    render(<App/>, root);
}

function showAuthError(err: unknown): boolean {
    if (!(err instanceof HttpError) || err.status !== 401) return false;
    localStorage.removeItem('satpulsewb-token');
    render(<AuthNotice/>, root);
    return true;
}

function ConnectingNotice() {
    return (
        <div style="margin:4rem 1.5rem;font-family:system-ui,sans-serif">
            Connecting to satpulsewb...
        </div>
    );
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

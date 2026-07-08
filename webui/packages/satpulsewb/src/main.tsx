import {h, render} from 'preact';
import {App} from '@satpulse/workbench/src/app';
import {setTransport} from '@satpulse/workbench/src/transport';
import {newHTTPTransport} from './http-transport';
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

setTransport(newHTTPTransport(token || ''));
render(<App/>, document.getElementById('app')!);

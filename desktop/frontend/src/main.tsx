import {h, render} from 'preact';
import {App} from '@satpulse/workbench/src/app';
import {setTransport} from '@satpulse/workbench/src/transport';
import {newWailsTransport} from './wails-transport';
import '@satpulse/workbench/src/style.css';

setTransport(newWailsTransport());
render(<App/>, document.getElementById('app')!);

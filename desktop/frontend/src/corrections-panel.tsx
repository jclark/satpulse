import {h} from 'preact';
import {useState, useEffect, useCallback} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {StartCorrections, StopCorrections} from '../wailsjs/go/main/App';
import type {ConnState} from './app';
import {Button, Input, fieldLabelText} from './ui';
import {RtcmPanel} from './rtcm-panel';

type CorrState = 'stopped' | 'connecting' | 'connected' | 'reconnecting';

interface CorrEvent {
    state: CorrState;
    host?: string;
    port?: number;
    error?: string;
}

interface Props {
    connState: ConnState;
}

const LS_HOST_KEY = 'corr-host';
const LS_PORT_KEY = 'corr-port';

export function CorrectionsPanel({connState}: Props) {
    const [host, setHost] = useState(() => localStorage.getItem(LS_HOST_KEY) || '');
    const [port, setPort] = useState(() => localStorage.getItem(LS_PORT_KEY) || '');
    const [corrState, setCorrState] = useState<CorrState>('stopped');
    const [corrError, setCorrError] = useState('');

    const connected = connState !== 'disconnected';

    // Persist host/port to localStorage
    useEffect(() => { localStorage.setItem(LS_HOST_KEY, host); }, [host]);
    useEffect(() => { localStorage.setItem(LS_PORT_KEY, port); }, [port]);

    // Listen for corrections state events
    useEffect(() => {
        const off = EventsOn('gps:corrections', (evt: CorrEvent) => {
            setCorrState(evt.state);
            setCorrError(evt.state === 'reconnecting' && evt.error ? evt.error : '');
        });
        return () => {
            if (typeof off === 'function') off(); else EventsOff('gps:corrections');
        };
    }, []);

    // Reset on disconnect
    useEffect(() => {
        if (!connected) {
            setCorrState('stopped');
            setCorrError('');
        }
    }, [connected]);

    const handleStart = useCallback(async () => {
        const p = parseInt(port, 10);
        if (!host || isNaN(p) || p <= 0 || p > 65535) return;
        const r = await StartCorrections(host, p);
        if (!r.ok && r.error) setCorrError(r.error);
    }, [host, port]);

    const handleStop = useCallback(async () => {
        await StopCorrections();
    }, []);

    const running = corrState !== 'stopped';
    const portNum = parseInt(port, 10);
    const startDisabled = !connected || !host || isNaN(portNum) || portNum <= 0 || portNum > 65535;

    let statusText = '';
    let statusClass = 'text-text-muted';
    switch (corrState) {
        case 'connected':
            statusText = `Connected to ${host}:${port}`;
            statusClass = 'text-success';
            break;
        case 'connecting':
            statusText = `Connecting to ${host}:${port}...`;
            statusClass = 'text-info';
            break;
        case 'reconnecting':
            statusText = `Reconnecting: ${corrError || 'connection lost'}`;
            statusClass = 'text-warning';
            break;
        case 'stopped':
            statusText = 'Stopped';
            break;
    }

    return (
        <div class="flex h-full flex-col">
            {/* Controls toolbar */}
            <div class="flex shrink-0 items-center gap-3 px-4 pt-4 pb-2">
                <span class={fieldLabelText(!connected)}>Host:</span>
                <Input
                    class="w-48"
                    value={host}
                    onInput={e => setHost((e.target as HTMLInputElement).value)}
                    disabled={!connected}
                    placeholder="e.g. 10.0.0.1"
                />
                <span class={fieldLabelText(!connected)}>Port:</span>
                <Input
                    class="w-20"
                    type="number"
                    value={port}
                    onInput={e => setPort((e.target as HTMLInputElement).value)}
                    disabled={!connected}
                    placeholder="2101"
                />
            </div>
            <div class="flex shrink-0 items-center gap-3 px-4 pb-2">
                <Button
                    variant="primary"
                    disabled={startDisabled}
                    onClick={handleStart}
                >
                    Start
                </Button>
                <Button
                    disabled={!running}
                    onClick={handleStop}
                >
                    Stop
                </Button>
                <span class={`text-xs ${statusClass}`}>{statusText}</span>
            </div>

            {/* RTCM packet table */}
            <RtcmPanel connected={connected} />
        </div>
    );
}

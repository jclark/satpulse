import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {GetCorrectionsState, StartCorrections, StopCorrections} from '../wailsjs/go/main/App';
import type {ConnState} from './app';
import {Button, Input, fieldLabelText, labeledControlText} from './ui';
import {RtcmPanel} from './rtcm-panel';

type CorrState = 'stopped' | 'connecting' | 'connected' | 'reconnecting';
type CorrMode = 'tcp' | 'ntrip';

interface CorrEvent {
    state: CorrState;
    mode?: CorrMode;
    host?: string;
    port?: number;
    mountpoint?: string;
    error?: string;
}

interface Props {
    connState: ConnState;
}

const LS_MODE_KEY = 'corr-mode';
const LS_HOST_KEY = 'corr-host';
const LS_PORT_TCP_KEY = 'corr-port-tcp';
const LS_PORT_NTRIP_KEY = 'corr-port-ntrip';
const LS_MOUNTPOINT_KEY = 'corr-mountpoint';
const LS_USERNAME_KEY = 'corr-username';
const LS_PASSWORD_KEY = 'corr-password';

const NTRIP_DEFAULT_PORT = '2101';

function readMode(): CorrMode {
    return localStorage.getItem(LS_MODE_KEY) === 'tcp' ? 'tcp' : 'ntrip';
}

function portKey(mode: CorrMode): string {
    return mode === 'ntrip' ? LS_PORT_NTRIP_KEY : LS_PORT_TCP_KEY;
}

function readPort(mode: CorrMode): string {
    const stored = localStorage.getItem(portKey(mode));
    if (stored) return stored;
    return mode === 'ntrip' ? NTRIP_DEFAULT_PORT : '';
}

export function CorrectionsPanel({connState}: Props) {
    const [mode, setMode] = useState<CorrMode>(readMode);
    const [host, setHost] = useState(() => localStorage.getItem(LS_HOST_KEY) || '');
    const [port, setPort] = useState(() => readPort(readMode()));
    const [mountpoint, setMountpoint] = useState(() => localStorage.getItem(LS_MOUNTPOINT_KEY) || '');
    const [username, setUsername] = useState(() => localStorage.getItem(LS_USERNAME_KEY) || '');
    const [password, setPassword] = useState(() => localStorage.getItem(LS_PASSWORD_KEY) || '');
    const [corrState, setCorrState] = useState<CorrState | null>(null);
    const [corrError, setCorrError] = useState('');
    const [statusHost, setStatusHost] = useState('');
    const [statusPort, setStatusPort] = useState<number | null>(null);
    const [statusMountpoint, setStatusMountpoint] = useState('');
    const [pending, setPending] = useState<'start' | 'stop' | null>(null);
    const pendingRef = useRef<'start' | 'stop' | null>(null);
    const corrEventSeqRef = useRef(0);
    const setPendingSync = useCallback((p: 'start' | 'stop' | null) => {
        pendingRef.current = p;
        setPending(p);
    }, []);

    const connected = connState !== 'disconnected';

    useEffect(() => { localStorage.setItem(LS_MODE_KEY, mode); }, [mode]);
    useEffect(() => { localStorage.setItem(LS_HOST_KEY, host); }, [host]);
    useEffect(() => { localStorage.setItem(portKey(mode), port); }, [mode, port]);
    useEffect(() => { localStorage.setItem(LS_MOUNTPOINT_KEY, mountpoint); }, [mountpoint]);
    useEffect(() => { localStorage.setItem(LS_USERNAME_KEY, username); }, [username]);
    useEffect(() => { localStorage.setItem(LS_PASSWORD_KEY, password); }, [password]);

    const applyCorrEvent = useCallback((evt: CorrEvent) => {
        setCorrState(evt.state);
        setCorrError(evt.state === 'reconnecting' && evt.error ? evt.error : '');
        setStatusHost(evt.host || '');
        setStatusPort(typeof evt.port === 'number' ? evt.port : null);
        setStatusMountpoint(evt.mountpoint || '');
        if (evt.state === 'stopped') {
            if (pendingRef.current !== null) setPendingSync(null);
        } else if (pendingRef.current === 'start') {
            setPendingSync(null);
        }
    }, [setPendingSync]);

    useEffect(() => {
        const off = EventsOn('gps:corrections', (evt: CorrEvent) => {
            corrEventSeqRef.current++;
            applyCorrEvent(evt);
        });
        return () => {
            if (typeof off === 'function') off(); else EventsOff('gps:corrections');
        };
    }, [applyCorrEvent]);

    useEffect(() => {
        if (!connected) return;
        const seq = corrEventSeqRef.current;
        let cancelled = false;
        GetCorrectionsState().then(evt => {
            if (cancelled) return;
            if (corrEventSeqRef.current !== seq) return;
            applyCorrEvent(evt as CorrEvent);
        }).catch(() => {});
        return () => {
            cancelled = true;
        };
    }, [connected, applyCorrEvent]);

    useEffect(() => {
        if (!connected) {
            setCorrState('stopped');
            setCorrError('');
            setStatusHost('');
            setStatusPort(null);
            setStatusMountpoint('');
            setPendingSync(null);
        }
    }, [connected, setPendingSync]);

    const synced = !connected || corrState !== null;
    const running = corrState !== null && corrState !== 'stopped';
    const portNum = parseInt(port, 10);
    const portOk = !isNaN(portNum) && portNum > 0 && portNum <= 65535;
    const hostOk = !!host.trim();
    const mountpointOk = !!mountpoint.trim();
    const canStart = hostOk && portOk && (mode === 'tcp' || mountpointOk);

    const handleModeChange = useCallback((next: CorrMode) => {
        setMode(next);
        setPort(readPort(next));
    }, []);

    const handleToggle = useCallback(async () => {
        if (pendingRef.current !== null) return;
        if (running) {
            setPendingSync('stop');
            const r = await StopCorrections();
            if (!r.ok) {
                setPendingSync(null);
                if (r.error) setCorrError(r.error);
            }
        } else {
            if (!canStart) return;
            setPendingSync('start');
            setCorrError('');
            const r = await StartCorrections({
                mode,
                host,
                port: portNum,
                mountpoint,
                username,
                password,
            });
            if (!r.ok) {
                setPendingSync(null);
                if (r.error) setCorrError(r.error);
            }
        }
    }, [running, canStart, mode, host, portNum, mountpoint, username, password, setPendingSync]);

    const locked = running || pending !== null;
    const fieldsDisabled = !connected || locked;

    let statusText = '';
    let statusClass = 'text-text-muted';
    const addrHost = statusHost || host;
    const addrPort = statusPort ?? (portOk ? portNum : null);
    const addr = addrPort !== null
        ? `${addrHost}:${addrPort}${statusMountpoint ? '/' + statusMountpoint : ''}`
        : '';
    switch (corrState) {
        case 'connected':
            statusText = `Connected to ${addr}`;
            statusClass = 'text-success';
            break;
        case 'connecting':
            statusText = `Connecting to ${addr}...`;
            statusClass = 'text-info';
            break;
        case 'reconnecting':
            statusText = `Reconnecting: ${corrError || 'connection lost'}`;
            statusClass = 'text-warning';
            break;
        case 'stopped':
            break;
    }

    return (
        <div class="flex h-full flex-col">
            <div class="flex shrink-0 items-center gap-4 px-4 pt-4 pb-2">
                {([['ntrip', 'NTRIP'], ['tcp', 'TCP']] as const).map(([val, label]) => (
                    <label key={val} class={labeledControlText(fieldsDisabled)}>
                        <input
                            type="radio"
                            name="corrMode"
                            class="accent-accent mr-1.5"
                            checked={mode === val}
                            disabled={fieldsDisabled}
                            onChange={() => handleModeChange(val)}
                        />
                        {label}
                    </label>
                ))}
            </div>
            <div class="flex shrink-0 items-center gap-3 px-4 pb-2">
                <span class={fieldLabelText(!connected)}>Host:</span>
                <Input
                    class="w-48"
                    value={host}
                    onInput={e => setHost((e.target as HTMLInputElement).value)}
                    disabled={fieldsDisabled}
                    placeholder="e.g. 10.0.0.1"
                />
                <span class={fieldLabelText(!connected)}>Port:</span>
                <Input
                    class="w-20"
                    inputMode="numeric"
                    value={port}
                    onInput={e => setPort((e.target as HTMLInputElement).value)}
                    disabled={fieldsDisabled}
                    placeholder={mode === 'ntrip' ? NTRIP_DEFAULT_PORT : ''}
                />
                {mode === 'ntrip' && (
                    <>
                        <span class={fieldLabelText(!connected)}>Mountpoint:</span>
                        <Input
                            class="w-40"
                            value={mountpoint}
                            onInput={e => setMountpoint((e.target as HTMLInputElement).value)}
                            disabled={fieldsDisabled}
                        />
                    </>
                )}
            </div>
            {mode === 'ntrip' && (
                <div class="flex shrink-0 items-center gap-3 px-4 pb-2">
                    <span class={fieldLabelText(!connected)}>Username:</span>
                    <Input
                        class="w-32"
                        value={username}
                        onInput={e => setUsername((e.target as HTMLInputElement).value)}
                        disabled={fieldsDisabled}
                    />
                    <span class={fieldLabelText(!connected)}>Password:</span>
                    <Input
                        class="w-32"
                        type="password"
                        value={password}
                        onInput={e => setPassword((e.target as HTMLInputElement).value)}
                        disabled={fieldsDisabled}
                    />
                </div>
            )}
            <div class="flex shrink-0 items-center gap-3 px-4 pb-2">
                <Button
                    variant={running ? 'secondary' : 'primary'}
                    disabled={!synced || pending !== null || !connected || (!running && !canStart)}
                    onClick={handleToggle}
                >
                    {running ? 'Disconnect' : 'Connect'}
                </Button>
                <span class={`text-xs ${statusClass}`}>{statusText}</span>
            </div>

            <RtcmPanel connected={connected} />
        </div>
    );
}

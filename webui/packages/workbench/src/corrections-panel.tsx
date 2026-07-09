import {h} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {GetCorrectionsState, StartCorrections, StopCorrections} from '../wailsjs/go/main/App';
import type {ConnState} from './app';
import {Button, Input, Select, cx, fieldLabelText, labeledControlText} from './ui';
import {CorMsgPanel} from './cor-msg-panel';

function fmtDeg(deg: number, digits: number): string {
    return deg.toFixed(digits);
}

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

interface NMEAPositionEvent {
    valid: boolean;
    lat: number;
    lon: number;
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
const LS_NMEASEND_KEY = 'corr-nmea-send';

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
    const [nmeaSend, setNmeaSend] = useState(() => localStorage.getItem(LS_NMEASEND_KEY) === '1');
    const [nmeaPos, setNmeaPos] = useState<[number, number] | null>(null);
    const [corrState, setCorrState] = useState<CorrState | null>(null);
    const [corrError, setCorrError] = useState('');
    const [pending, setPending] = useState<'start' | 'stop' | null>(null);
    const [sessionSeq, setSessionSeq] = useState(0);
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
    useEffect(() => { localStorage.setItem(LS_NMEASEND_KEY, nmeaSend ? '1' : '0'); }, [nmeaSend]);

    const applyCorrEvent = useCallback((evt: CorrEvent) => {
        setCorrState(evt.state);
        setCorrError(evt.state === 'reconnecting' && evt.error ? evt.error : '');
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
        const off = EventsOn('gps:nmeaPosition', (e: NMEAPositionEvent) =>
            setNmeaPos(e.valid ? [e.lat, e.lon] : null));
        return () => {
            if (typeof off === 'function') off(); else EventsOff('gps:nmeaPosition');
        };
    }, []);

    useEffect(() => {
        if (!connected) {
            setCorrState('stopped');
            setCorrError('');
            setNmeaPos(null);
            setPendingSync(null);
        }
    }, [connected, setPendingSync]);

    const synced = !connected || corrState !== null;
    const running = corrState !== null && corrState !== 'stopped';
    const portNum = parseInt(port, 10);
    const portOk = !isNaN(portNum) && portNum > 0 && portNum <= 65535;
    const hostOk = !!host.trim();
    const mountpointOk = !!mountpoint.trim();
    const nmeaSendActive = mode === 'ntrip' && nmeaSend;
    const canStart = hostOk && portOk && (mode === 'tcp' || mountpointOk) && (!nmeaSendActive || nmeaPos !== null);

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
            setSessionSeq(s => s + 1);
            const r = await StartCorrections({
                mode,
                host,
                port: portNum,
                mountpoint,
                username,
                password,
                nmeaSend: nmeaSendActive,
            });
            if (!r.ok) {
                setPendingSync(null);
                if (r.error) setCorrError(r.error);
            }
        }
    }, [running, canStart, mode, host, portNum, mountpoint, username, password, nmeaSendActive, setPendingSync]);

    const locked = running || pending !== null;
    const fieldsDisabled = !connected || locked;

    let dotClass = 'bg-text-muted';
    if (corrState === 'connected') dotClass = 'bg-success';
    else if (corrState === 'reconnecting') dotClass = 'bg-danger';
    else if (corrState === 'connecting') dotClass = 'bg-warning';

    let statusText = '';
    let statusClass = 'text-text-muted';
    if (corrState === 'reconnecting') {
        statusText = `Reconnecting: ${corrError || 'connection lost'}`;
        statusClass = 'text-warning';
    } else if (corrError) {
        statusText = corrError;
        statusClass = 'text-danger';
    }

    const ntripDisabled = fieldsDisabled || mode !== 'ntrip';

    return (
        <div class="flex h-full flex-col">
            <div class="flex shrink-0 items-center gap-3 px-4 pt-4 pb-2">
                <Select
                    class="w-24"
                    value={mode}
                    onChange={e => handleModeChange((e.target as HTMLSelectElement).value as CorrMode)}
                    disabled={fieldsDisabled}
                >
                    <option value="ntrip">NTRIP</option>
                    <option value="tcp">TCP</option>
                </Select>
                <div class={`h-2.5 w-2.5 shrink-0 rounded-full ${dotClass}`} />
                <span class={cx(fieldLabelText(!connected), 'w-8 shrink-0')}>Host:</span>
                <Input
                    class="w-40"
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
                <span class={fieldLabelText(ntripDisabled)}>Mountpoint:</span>
                <Input
                    class="w-40"
                    value={mountpoint}
                    onInput={e => setMountpoint((e.target as HTMLInputElement).value)}
                    disabled={ntripDisabled}
                />
                <Button
                    class="ml-auto"
                    variant={running ? 'secondary' : 'primary'}
                    disabled={!synced || pending !== null || !connected || (!running && !canStart)}
                    onClick={handleToggle}
                >
                    {running ? 'Disconnect' : 'Connect'}
                </Button>
            </div>
            <div class="flex shrink-0 items-center gap-3 px-4 pb-2">
                <div class="w-24 shrink-0 invisible" />
                <div class="h-2.5 w-2.5 shrink-0 invisible" />
                <span class={cx(fieldLabelText(ntripDisabled), 'w-8 shrink-0')}>User:</span>
                <Input
                    class="w-40"
                    value={username}
                    onInput={e => setUsername((e.target as HTMLInputElement).value)}
                    disabled={ntripDisabled}
                />
                <span class={fieldLabelText(ntripDisabled)}>Password:</span>
                <Input
                    class="w-32"
                    type="password"
                    value={password}
                    onInput={e => setPassword((e.target as HTMLInputElement).value)}
                    disabled={ntripDisabled}
                />
                <span class={cx('ml-auto text-xs', statusClass)}>{statusText}</span>
            </div>
            <div class="flex shrink-0 items-center gap-3 px-4 pb-2">
                <div class="w-24 shrink-0 invisible" />
                <div class="h-2.5 w-2.5 shrink-0 invisible" />
                {mode === 'ntrip' && (
                    <label class={cx('flex items-center gap-1.5', labeledControlText(fieldsDisabled))}>
                        <input
                            type="checkbox"
                            class="accent-accent"
                            checked={nmeaSend}
                            disabled={fieldsDisabled}
                            onChange={e => setNmeaSend((e.target as HTMLInputElement).checked)}
                        />
                        Send position as NMEA
                    </label>
                )}
                <span class={cx(fieldLabelText(!connected), mode === 'ntrip' ? 'ml-4' : '')}>Position:</span>
                <span class="font-mono text-xs text-text-primary">
                    {nmeaPos ? `${fmtDeg(nmeaPos[0], 7)}, ${fmtDeg(nmeaPos[1], 7)}` : ''}
                </span>
            </div>

            <CorMsgPanel connected={connected} sessionSeq={sessionSeq} running={running} />
        </div>
    );
}

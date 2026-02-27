import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Connect, Disconnect, GetAllSignals, GetConnState, GetReceiverState, ListPorts} from '../wailsjs/go/main/App';
import {ConnectionPanel, PortInfo} from './connection-panel';
import {CollapsibleSection} from './collapsible-section';
import {ConfigPanel} from './config-panel';
import {MonitorPanel} from './monitor-panel';
import {LoggingPanel} from './logging-panel';
import {SurveyPanel} from './survey-panel';
import {MsgFilePanel} from './msgfile-panel';
import {PVTPanel} from './pvt-panel';
import type {PosRow, PosGeoRow, PosECEFRow, VelRow, VelGeoRow, VelECEFRow, TimeRow} from './pvt-panel';
import {MapPanel} from './map-panel';
import {ClockPanel} from './clock-panel';
import {SkyViewPanel} from './sky-view-panel';

export type ConnState = 'disconnected' | 'connecting' | 'connected' | 'configuring' | 'sending';

export type ReceiverState =
    | {status: 'disconnected'}
    | {status: 'probing'}
    | {status: 'identified'; vendor: string; hardware: string; firmware: string; supportedGNSS: string[]; packetFormats: string[]}
    | {status: 'unidentified'; packetFormats: string[]; warning: string}
    | {status: 'error'; error: string};

interface Toast {
    id: number;
    message: string;
    type: 'success' | 'error';
}

export interface LogEntry {
    level: string;
    message: string;
    time: string;
    component?: string;
    attrs?: Record<string, any>;
}

export type OperationStatus = 'idle' | 'running' | 'success' | 'failed';

export interface OperationState {
    status: OperationStatus;
    label: string;
    error?: string;
    startTime?: string;
}

export interface PacketEntry {
    tag: string;
    msg?: string;
    bin?: string;
    ascii?: string;
    timestamp: string;
}

export interface TimeMsg {
    taiTime?: string;
    utcTime?: string;
    accuracy?: number;
    gnss?: string;
}

export interface SurveyMsg {
    position?: [number, number, number];
    accuracy: number;
    obsCount: number;
    obsTime: number;
    valid: boolean;
    inProgress: boolean;
}

export interface LeapSecondState {
    utcOff: number;
}

export interface SatellitesMsg {
    tag?: string;
    nativeMsgID?: string;
    info: SVInfo[];
    usedValidity?: number;
}

export interface SVInfo {
    id: string;
    lookAngles?: {azimuth: number; elevation: number};
    signals: SignalInfo[];
    used?: boolean;
}

export interface SignalInfo {
    id?: string;
    cn0: number;
    used?: boolean;
}

interface MsgEvent {
    kind: string;
    msg: any;
    time: string;
}

export interface MsgFileTag {
    tag: string;
    desc?: string;
    msgCount: number;
}

export interface MsgSendEvent {
    status: 'sent' | 'delaying' | 'delayed' | 'done' | 'cancelled' | 'error';
    current?: number;
    total?: number;
    error?: string;
}

export interface SendLine {
    status: 'sending' | 'delaying' | 'done' | 'error';
    index: number;
    total: number;
    error?: string;
}

type TabID = 'monitor' | 'packets' | 'config' | 'messages';

const tabBtnBase = 'px-5 py-2 text-sm font-medium border-b-2 cursor-pointer bg-transparent';
const tabBtnActive = tabBtnBase + ' border-accent text-accent bg-surface-1';
const tabBtnInactive = tabBtnBase + ' border-transparent text-text-secondary hover:text-text-primary hover:bg-surface-1';
const tabBtnDisabled = tabBtnBase + ' border-transparent text-text-muted cursor-not-allowed';

const connStateLabel: Record<ConnState, string> = {
    disconnected: 'Disconnected',
    connecting: 'Connecting...',
    connected: 'Connected',
    configuring: 'Configuring...',
    sending: 'Sending...',
};

let toastId = 0;

export function App() {
    const [connState, setConnState] = useState<ConnState>('disconnected');
    const [device, setDevice] = useState('');
    const [speed, setSpeed] = useState(9600);
    const [ports, setPorts] = useState<PortInfo[]>([]);
    const [receiver, setReceiver] = useState<ReceiverState>({status: 'disconnected'});
    const [configProps, setConfigProps] = useState<Record<string, any> | null>(null);
    const [signalCatalog, setSignalCatalog] = useState<Record<string, string[]>>({});
    const [selectedSignals, setSelectedSignals] = useState<Set<string>>(new Set());
    const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
    const [packetEntries, setPacketEntries] = useState<PacketEntry[]>([]);
    const [toasts, setToasts] = useState<Toast[]>([]);
    const [, setOperation] = useState<OperationState>({status: 'idle', label: ''});
    const [timeMsg, setTimeMsg] = useState<TimeMsg | null>(null);
    const [surveyMsg, setSurveyMsg] = useState<SurveyMsg | null>(null);
    const [leapSecond, setLeapSecond] = useState<LeapSecondState | null>(null);
    const [posRows, setPosRows] = useState<Map<string, PosRow>>(new Map());
    const [velRows, setVelRows] = useState<Map<string, VelRow>>(new Map());
    const [timeRows, setTimeRows] = useState<Map<string, TimeRow>>(new Map());
    const [pvtOpen, setPvtOpen] = useState(false);
    const pvtAutoExpanded = useRef(false);
    const [satsMsg, setSatsMsg] = useState<SatellitesMsg | null>(null);
    const [mapPos, setMapPos] = useState<{lat: number; lon: number} | null>(null);
    const [mapCourse, setMapCourse] = useState<{course: number; groundSpeed: number} | null>(null);
    const [noFixSecs, setNoFixSecs] = useState(0);
    const [activeTab, setActiveTab] = useState<TabID>('monitor');
    const [surveyOpen, setSurveyOpen] = useState(false);
    const surveyAutoExpanded = useRef(false);
    const [logHeight, setLogHeight] = useState(150);
    const dragging = useRef(false);

    // Message file state
    const [msgFilePath, setMsgFilePath] = useState('');
    const [msgFileTags, setMsgFileTags] = useState<MsgFileTag[]>([]);
    const [selectedTagIndex, setSelectedTagIndex] = useState(-1);
    const [sendLines, setSendLines] = useState<SendLine[]>([]);
    const [sendState, setSendState] = useState<'idle' | 'sending' | 'done' | 'error'>('idle');

    const onSplitterMouseDown = useCallback((e: MouseEvent) => {
        e.preventDefault();
        dragging.current = true;
        const startY = e.clientY;
        const startH = logHeight;
        const onMove = (e: MouseEvent) => {
            if (!dragging.current) return;
            setLogHeight(Math.max(60, startH - (e.clientY - startY)));
        };
        const onUp = () => {
            dragging.current = false;
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
        };
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onUp);
    }, [logHeight]);

    // Auto-expand survey section on first survey data
    useEffect(() => {
        if (surveyMsg && !surveyAutoExpanded.current) {
            surveyAutoExpanded.current = true;
            setSurveyOpen(true);
        }
    }, [surveyMsg]);

    // Auto-expand PVT section on first PVT data
    useEffect(() => {
        if ((posRows.size > 0 || velRows.size > 0 || timeRows.size > 0) && !pvtAutoExpanded.current) {
            pvtAutoExpanded.current = true;
            setPvtOpen(true);
        }
    }, [posRows.size, velRows.size, timeRows.size]);

    const addToast = useCallback((message: string, type: 'success' | 'error') => {
        const id = ++toastId;
        setToasts(prev => [...prev, {id, message, type}]);
        setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 3000);
    }, []);

    const refreshPorts = useCallback(() => {
        ListPorts().then(list => {
            const ps: PortInfo[] = (list || []).map(p => ({device: p.device, display: p.display}));
            setPorts(ps);
            return ps;
        }).catch(() => []);
    }, []);

    // Fetch ports on startup; auto-select if exactly one
    useEffect(() => {
        ListPorts().then(list => {
            const ps: PortInfo[] = (list || []).map(p => ({device: p.device, display: p.display}));
            setPorts(ps);
            if (ps.length === 1) setDevice(ps[0].device);
        }).catch(() => {});
    }, []);

    useEffect(() => {
        const offLog = EventsOn('gps:log', (evt: LogEntry) => {
            setLogEntries(prev => [...prev.slice(-199), evt]);
        });
        const offPkt = EventsOn('gps:packet', (pkt: PacketEntry) => {
            setPacketEntries(prev => [...prev.slice(-199), pkt]);
        });
        const offRcv = EventsOn('gps:receiver', (evt: any) => {
            if (!evt.ok && !evt.error) {
                setReceiver({status: 'probing'});
            } else if (evt.error) {
                setReceiver({status: 'error', error: evt.error});
            } else {
                const info = evt.Info;
                const vendor: string = info?.vendor || '';
                if (vendor) {
                    const gnss: string[] = info?.supportedGNSS || [];
                    setReceiver({
                        status: 'identified',
                        vendor,
                        hardware: info?.hardware || '',
                        firmware: info?.firmware || '',
                        supportedGNSS: gnss,
                        packetFormats: evt.packetFormats || [],
                    });
                    GetAllSignals(gnss).then(cat => {
                        if (cat) setSignalCatalog(cat);
                    }).catch(() => {});
                } else {
                    setReceiver({
                        status: 'unidentified',
                        packetFormats: evt.packetFormats || [],
                        warning: evt.warning || '',
                    });
                }
            }
        });
        const offMsg = EventsOn('gps:msg', (evt: MsgEvent) => {
            switch (evt.kind) {
                case 'time': {
                    const msg = evt.msg;
                    if (msg.nativeMsgID) {
                        setTimeRows(prev => {
                            const next = new Map(prev);
                            next.set(msg.nativeMsgID, {
                                tag: msg.tag || '',
                                nativeMsgID: msg.nativeMsgID,
                                ref: msg.ref ?? 0,
                                taiTime: msg.taiTime,
                                utcTime: msg.utcTime,
                                accuracy: msg.accuracy,
                                utcOffset: msg.utcOffset,
                                gnss: msg.gnss,
                            } as TimeRow);
                            return next;
                        });
                    }
                    break;
                }
                case 'survey':
                    setSurveyMsg(evt.msg as SurveyMsg);
                    break;
                case 'leapSecond': {
                    const ls = evt.msg;
                    if (ls && typeof ls.UTCOffAfter === 'number') {
                        setLeapSecond({utcOff: ls.UTCOffAfter});
                    }
                    break;
                }
                case 'posGeo': {
                    const msg = evt.msg;
                    setPosRows(prev => {
                        const next = new Map(prev);
                        next.set(msg.nativeMsgID, {
                            kind: 'posGeo',
                            tag: msg.tag,
                            nativeMsgID: msg.nativeMsgID,
                            latLon: msg.latLon,
                            height: msg.height,
                            heightMSL: msg.heightMSL,
                        } as PosGeoRow);
                        return next;
                    });
                    break;
                }
                case 'posECEF': {
                    const msg = evt.msg;
                    setPosRows(prev => {
                        const next = new Map(prev);
                        next.set(msg.nativeMsgID, {
                            kind: 'posECEF',
                            tag: msg.tag,
                            nativeMsgID: msg.nativeMsgID,
                            pos: msg.pos,
                        } as PosECEFRow);
                        return next;
                    });
                    break;
                }
                case 'velGeo': {
                    const msg = evt.msg;
                    setVelRows(prev => {
                        const next = new Map(prev);
                        next.set(msg.nativeMsgID, {
                            kind: 'velGeo',
                            tag: msg.tag,
                            nativeMsgID: msg.nativeMsgID,
                            velNED: msg.velNED,
                            groundSpeed: msg.groundSpeed,
                            speed3D: msg.speed3D,
                            course: msg.course,
                        } as VelGeoRow);
                        return next;
                    });
                    break;
                }
                case 'velECEF': {
                    const msg = evt.msg;
                    setVelRows(prev => {
                        const next = new Map(prev);
                        next.set(msg.nativeMsgID, {
                            kind: 'velECEF',
                            tag: msg.tag,
                            nativeMsgID: msg.nativeMsgID,
                            vel: msg.vel,
                        } as VelECEFRow);
                        return next;
                    });
                    break;
                }
                case 'satellites':
                    setSatsMsg(evt.msg as SatellitesMsg);
                    break;
            }
        });
        const offState = EventsOn('gps:state', (state: ConnState) => {
            setConnState(state);
            if (state === 'disconnected') {
                setTimeMsg(null);
                setSurveyMsg(null);
                setSatsMsg(null);
                setLeapSecond(null);
                setPosRows(new Map());
                setVelRows(new Map());
                setTimeRows(new Map());
                pvtAutoExpanded.current = false;
                setMapPos(null);
                setMapCourse(null);
                setNoFixSecs(0);
            }
        });
        const offInitialPos = EventsOn('gps:initialPos', (ll: [number, number]) => {
            setMapPos({lat: ll[0], lon: ll[1]});
        });
        const offEpochPVT = EventsOn('gps:epochPVT', (nav: any) => {
            if (nav.posGeo) {
                setMapPos({lat: nav.posGeo.latLon[0], lon: nav.posGeo.latLon[1]});
                setNoFixSecs(0);
            } else {
                setNoFixSecs(prev => prev + 1);
            }
            if (nav.velGeo && nav.velGeo.groundSpeed != null && nav.velGeo.course != null) {
                setMapCourse({course: nav.velGeo.course, groundSpeed: nav.velGeo.groundSpeed});
            } else {
                setMapCourse(null);
            }
        });
        const offTime = EventsOn('gps:time', (msg: any) => {
            setTimeMsg(msg as TimeMsg);
        });
        const offMsgSend = EventsOn('gps:msgsend', (evt: MsgSendEvent) => {
            const {status, current, total, error} = evt;
            switch (status) {
                case 'sent':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        lines[idx] = {status: 'sending', index: current ?? 1, total: total ?? 1};
                        return lines;
                    });
                    break;
                case 'delaying':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        if (lines[idx]) lines[idx] = {...lines[idx], status: 'delaying'};
                        return lines;
                    });
                    break;
                case 'delayed':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        if (lines[idx]) lines[idx] = {...lines[idx], status: 'done'};
                        return lines;
                    });
                    break;
                case 'done':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        if (lines[idx]) lines[idx] = {...lines[idx], status: 'done'};
                        return lines;
                    });
                    setSendState('done');
                    break;
                case 'cancelled':
                    setSendState('done');
                    break;
                case 'error':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        lines[idx] = {status: 'error', index: current ?? 1, total: total ?? 1, error: error};
                        return lines;
                    });
                    setSendState('error');
                    break;
            }
        });
        return () => {
            if (typeof offLog === 'function') offLog(); else EventsOff('gps:log');
            if (typeof offPkt === 'function') offPkt(); else EventsOff('gps:packet');
            if (typeof offRcv === 'function') offRcv(); else EventsOff('gps:receiver');
            if (typeof offMsg === 'function') offMsg(); else EventsOff('gps:msg');
            if (typeof offState === 'function') offState(); else EventsOff('gps:state');
            if (typeof offInitialPos === 'function') offInitialPos(); else EventsOff('gps:initialPos');
            if (typeof offEpochPVT === 'function') offEpochPVT(); else EventsOff('gps:epochPVT');
            if (typeof offTime === 'function') offTime(); else EventsOff('gps:time');
            if (typeof offMsgSend === 'function') offMsgSend(); else EventsOff('gps:msgsend');
        };
    }, []);

    // Sync connection state with Go backend on mount (after HMR reload)
    useEffect(() => {
        GetConnState().then(async (s: string) => {
            setConnState(s as ConnState);
            if (s === 'disconnected') return;
            const r = await GetReceiverState();
            if (r.ok) {
                const info = (r as any).Info;
                const vendor: string = info?.vendor || '';
                if (vendor) {
                    const gnss: string[] = info?.supportedGNSS || [];
                    setReceiver({
                        status: 'identified',
                        vendor,
                        hardware: info?.hardware || '',
                        firmware: info?.firmware || '',
                        supportedGNSS: gnss,
                        packetFormats: r.packetFormats || [],
                    });
                    const catalog = await GetAllSignals(gnss);
                    if (catalog) setSignalCatalog(catalog);
                } else {
                    setReceiver({
                        status: 'unidentified',
                        packetFormats: r.packetFormats || [],
                        warning: (r as any).warning || '',
                    });
                }
            }
        }).catch(() => {});
    }, []);

    const handleConfigReadback = useCallback((props: Record<string, any>) => {
        setConfigProps(props);
    }, []);

    const handleConnect = useCallback(async () => {
        if (connState !== 'disconnected') {
            await Disconnect();
            return;
        }
        const r = await Connect(device, speed);
        if (r.ok) {
            // connection status visible via indicator dot
        } else {
            addToast(r.error || 'Connection failed', 'error');
        }
    }, [connState, device, speed, addToast]);

    // Receiver identity string for connection bar
    const receiverIdent = receiver.status === 'identified'
        ? `${receiver.hardware} (FW ${receiver.firmware})`
        : receiver.status === 'unidentified'
            ? receiver.packetFormats.length > 0
                ? `Unknown (${receiver.packetFormats.join(', ')})`
                : 'Unknown'
            : receiver.status === 'probing' ? 'Identifying...' : '';

    const configDisabled = receiver.status !== 'identified';
    const connected = connState !== 'disconnected';

    return (
        <>
            {/* Connection bar */}
            <ConnectionPanel
                connected={connected}
                device={device}
                setDevice={setDevice}
                speed={speed}
                setSpeed={setSpeed}
                onConnect={handleConnect}
                receiverIdent={receiverIdent}
                ports={ports}
                onRefreshPorts={refreshPorts}
            />

            {/* Tab bar */}
            <div class="flex shrink-0 border-b border-border-subtle bg-surface-2">
                <button
                    class={activeTab === 'monitor' ? tabBtnActive : tabBtnInactive}
                    onClick={() => setActiveTab('monitor')}
                >
                    Monitor
                </button>
                <button
                    class={activeTab === 'packets' ? tabBtnActive : tabBtnInactive}
                    onClick={() => setActiveTab('packets')}
                >
                    Packets
                </button>
                <button
                    class={configDisabled ? tabBtnDisabled : (activeTab === 'config' ? tabBtnActive : tabBtnInactive)}
                    onClick={() => { if (!configDisabled) setActiveTab('config'); }}
                >
                    Configuration
                </button>
                <button
                    class={activeTab === 'messages' ? tabBtnActive : tabBtnInactive}
                    onClick={() => setActiveTab('messages')}
                >
                    Message file
                </button>
            </div>

            {/* Tab content area */}
            <div class="flex-1 overflow-hidden" style="min-height: 80px;">
                {/* Monitor tab */}
                <div class={`h-full overflow-y-auto ${activeTab === 'monitor' ? '' : 'hidden'}`}>
                    <div class="flex gap-4 p-4">
                        <div class="flex flex-col gap-4">
                            <ClockPanel msg={timeMsg} />
                            <MapPanel pos={mapPos} course={mapCourse} noFixSecs={noFixSecs} />
                        </div>
                        {satsMsg && <SkyViewPanel msg={satsMsg} />}
                    </div>
                    <CollapsibleSection title="PVT Messages" variant="panel" open={pvtOpen} onToggle={setPvtOpen}>
                        <PVTPanel posRows={posRows} velRows={velRows} timeRows={timeRows} leapSecond={leapSecond} />
                    </CollapsibleSection>
                    <CollapsibleSection title="Survey" variant="panel" open={surveyOpen} onToggle={setSurveyOpen}>
                        <SurveyPanel msg={surveyMsg} />
                    </CollapsibleSection>
                </div>

                {/* Packets tab */}
                <div class={`h-full ${activeTab === 'packets' ? '' : 'hidden'}`}>
                    <MonitorPanel
                        packetEntries={packetEntries}
                        setPacketEntries={setPacketEntries}
                        visible={activeTab === 'packets'}
                    />
                </div>

                {/* Configuration tab */}
                <div class={`h-full ${activeTab === 'config' ? '' : 'hidden'}`}>
                    <ConfigPanel
                        connState={connState}
                        visible={activeTab === 'config'}
                        configProps={configProps}
                        signalCatalog={signalCatalog}
                        selectedSignals={selectedSignals}
                        setSelectedSignals={setSelectedSignals}
                        setOperation={setOperation}
                        addToast={addToast}
                        onConfigReadback={handleConfigReadback}
                        speed={speed}
                    />
                </div>

                {/* Messages tab */}
                <div class={`h-full ${activeTab === 'messages' ? '' : 'hidden'}`}>
                    <MsgFilePanel
                        connState={connState}
                        msgFilePath={msgFilePath}
                        setMsgFilePath={setMsgFilePath}
                        msgFileTags={msgFileTags}
                        setMsgFileTags={setMsgFileTags}
                        selectedTagIndex={selectedTagIndex}
                        setSelectedTagIndex={setSelectedTagIndex}
                        sendLines={sendLines}
                        setSendLines={setSendLines}
                        sendState={sendState}
                        setSendState={setSendState}
                        addToast={addToast}
                    />
                </div>
            </div>

            {/* Drag splitter */}
            <div
                class="shrink-0 h-1 cursor-row-resize border-y border-border-subtle"
                onMouseDown={onSplitterMouseDown}
            />

            {/* Activity log (always visible) */}
            <div class="shrink-0 overflow-hidden" style={{height: logHeight + 'px'}}>
                <LoggingPanel logEntries={logEntries} setLogEntries={setLogEntries} />
            </div>

            {/* Status bar */}
            <div class="shrink-0 border-t border-border-subtle bg-surface-2 px-5 py-1 text-xs text-text-secondary">
                {connStateLabel[connState]}
            </div>

            {/* Toasts */}
            <div class="fixed bottom-10 right-5 flex flex-col gap-2 z-50">
                {toasts.map(t => (
                    <div
                        key={t.id}
                        class={`animate-[fadeIn_0.2s] rounded px-4 py-2 text-sm text-surface-2 ${
                            t.type === 'success' ? 'bg-success' : 'bg-danger'
                        }`}
                    >
                        {t.message}
                    </div>
                ))}
            </div>
        </>
    );
}

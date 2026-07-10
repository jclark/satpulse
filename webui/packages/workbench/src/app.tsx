import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {transport} from './transport';
import type {ConnState, MsgFileTag, PortInfo} from './transport';
import type {TimeMsg, SurveyMsg, SatellitesMsg, SVInfo, SignalInfo} from '@satpulse/gps/gpsprot';
import {ConnectionPanel} from './connection-panel';
import {CollapsibleSection} from './collapsible-section';
import {ConfigPanel} from './config-panel';
import {PacketPanel} from './packet-panel';
import {LoggingPanel} from './logging-panel';
import {SurveyPanel} from './survey-panel';
import type {NavEpochMsg} from '@satpulse/gps/gpsprot';
import {MsgFilePanel} from './msgfile-panel';
import {CorrectionsPanel} from './corrections-panel';
import {PVTPanel} from './pvt-panel';
import type {PosRow, PosGeoRow, PosECEFRow, VelRow, VelGeoRow, VelECEFRow, TimeRow} from './pvt-panel';
import {MapPanel} from './map-panel';
import {ClockPanel} from './clock-panel';
import {SkyViewPanel, SkyViewLegend} from './sky-view-panel';
import {SummaryPanel} from './summary-panel';
import {ScatterPanel} from './scatter-panel';
import {SignalsPanel} from './signals-panel';
export type {TimeMsg, SurveyMsg, SatellitesMsg, SVInfo, SignalInfo};
export type {ConnState, MsgFileTag};

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

export interface LeapSecondState {
    utcOff: number;
}

interface MsgEvent {
    kind: string;
    msg: any;
    time: string;
}

export interface MsgSendEvent {
    session: number;
    status: 'started' | 'sent' | 'done' | 'cancelled' | 'error';
    current?: number;
    total?: number;
    error?: string;
}

export interface SendLine {
    session: number;
    status: 'sending' | 'done' | 'error';
    index: number;
    total: number;
    error?: string;
}

export interface ResponseLine {
    session: number;
    kind: 'ack' | 'other' | 'maybe';
    responseTo: number;      // 0-based index of sent message, or -1
    msgCount?: number;       // ack only: total messages sent for this tag
    ackError?: string;       // ack only
    tag?: string;            // protocol tag
    msgID?: string;          // message ID
    text?: string;           // text protocols
    bin?: string;            // binary protocols (hex)
}

type TabID = 'monitor' | 'packets' | 'corrections' | 'config' | 'messages';

const tabBtnBase = 'px-5 py-2 text-sm font-medium border-b-2 cursor-pointer bg-transparent';
const tabBtnActive = tabBtnBase + ' border-accent text-accent bg-surface-1';
const tabBtnInactive = tabBtnBase + ' border-transparent text-text-secondary hover:text-text-primary hover:bg-surface-1';
const tabBtnDisabled = tabBtnBase + ' border-transparent text-text-muted cursor-not-allowed';

const connStateLabel: Record<ConnState, string> = {
    disconnected: 'Disconnected',
    connecting: 'Connecting...',
    reconnecting: 'Reconnecting...',
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
    const [toasts, setToasts] = useState<Toast[]>([]);
    const [, setOperation] = useState<OperationState>({status: 'idle', label: ''});
    const [timeMsg, setTimeMsg] = useState<TimeMsg | null>(null);
    const [navEpochMsg, setNavEpochMsg] = useState<NavEpochMsg | null>(null);
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
    const [scatterECEF, setScatterECEF] = useState<[number, number, number] | null>(null);
    const [baseARPs, setBaseARPs] = useState<Map<number, [number, number, number]>>(new Map());
    const pvtEpoch = useRef(0);
    const [activeTab, setActiveTab] = useState<TabID>('monitor');
    const [trackGen, setTrackGen] = useState(0);
    const [surveyOpen, setSurveyOpen] = useState(false);
    const surveyAutoExpanded = useRef(false);
    const [logHeight, setLogHeight] = useState(140);
    const dragging = useRef(false);
    const monitorRowRef = useRef<HTMLDivElement>(null);

    // Drive --row-scale on the monitor row 2 from its current width, anchored
    // at 1009 px (the row width at a 1024 px window). Every dimension inside
    // the row uses calc(... * var(--row-scale)) so the layout grows or shrinks
    // proportionally.
    useEffect(() => {
        const el = monitorRowRef.current;
        if (!el) return;
        const update = () => {
            const w = el.getBoundingClientRect().width;
            if (w > 0) el.style.setProperty('--row-scale', String(w / 1009));
        };
        update();
        const ro = new ResizeObserver(update);
        ro.observe(el);
        return () => ro.disconnect();
    }, []);

    // Message file state
    const [msgFilePath, setMsgFilePath] = useState('');
    const [msgFileTags, setMsgFileTags] = useState<MsgFileTag[]>([]);
    const [selectedTagIndex, setSelectedTagIndex] = useState(-1);
    const [activeTagIndex, setActiveTagIndex] = useState(-1);
    const [tagArmed, setTagArmed] = useState(false);
    const [sendLines, setSendLines] = useState<SendLine[]>([]);
    const [responseLines, setResponseLines] = useState<ResponseLine[]>([]);
    const [selectedResponseIndex, setSelectedResponseIndex] = useState(-1);
    const respSessionRef = useRef(0);

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
        transport.connection?.listPorts().then(list => setPorts(list || [])).catch(() => {});
    }, []);

    // Fetch ports on startup; auto-select if exactly one port
    useEffect(() => {
        const conn = transport.connection;
        if (!conn) return;
        conn.listPorts().then(list => {
            const ps = list || [];
            setPorts(ps);
            if (ps.length === 1) setDevice(ps[0].device);
        }).catch(() => {});
    }, []);

    useEffect(() => {
        const offLog = transport.eventsOn('gps:log', (evt: LogEntry) => {
            setLogEntries(prev => [...prev.slice(-199), evt]);
        });
        const offRcv = transport.eventsOn('gps:receiver', (evt: any) => {
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
                    transport.getAllSignals(gnss).then(cat => {
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
        const offSpeed = transport.eventsOn('gps:speed', (newSpeed: number) => {
            if (newSpeed) setSpeed(newSpeed);
        });
        const offMsg = transport.eventsOn('gps:msg', (evt: MsgEvent) => {
            switch (evt.kind) {
                case 'time': {
                    const msg = evt.msg;
                    if (msg.nativeMsgID) {
                        const row: TimeRow = {...msg, nativeMsgID: msg.nativeMsgID, epoch: pvtEpoch.current};
                        setTimeRows(prev => new Map(prev).set(msg.nativeMsgID, row));
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
                    const row: PosGeoRow = {kind: 'posGeo', epoch: pvtEpoch.current, ...msg};
                    setPosRows(prev => new Map(prev).set(msg.nativeMsgID, row));
                    break;
                }
                case 'posECEF': {
                    const msg = evt.msg;
                    const row: PosECEFRow = {kind: 'posECEF', epoch: pvtEpoch.current, ...msg};
                    setPosRows(prev => new Map(prev).set(msg.nativeMsgID, row));
                    break;
                }
                case 'velGeo': {
                    const msg = evt.msg;
                    const row: VelGeoRow = {kind: 'velGeo', epoch: pvtEpoch.current, ...msg};
                    setVelRows(prev => new Map(prev).set(msg.nativeMsgID, row));
                    break;
                }
                case 'velECEF': {
                    const msg = evt.msg;
                    const row: VelECEFRow = {kind: 'velECEF', epoch: pvtEpoch.current, ...msg};
                    setVelRows(prev => new Map(prev).set(msg.nativeMsgID, row));
                    break;
                }
                case 'navEpoch':
                    setNavEpochMsg(evt.msg as NavEpochMsg);
                    break;
                case 'satellites':
                    setSatsMsg(evt.msg as SatellitesMsg);
                    break;
            }
        });
        const offState = transport.eventsOn('gps:state', (state: ConnState) => {
            setConnState(state);
            if (state === 'configuring' || state === 'disconnected') {
                respSessionRef.current = 0;
            }
            if (state === 'disconnected') {
                setTimeMsg(null);
                setNavEpochMsg(null);
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
                setScatterECEF(null);
                setBaseARPs(new Map());
                setTrackGen(g => g + 1);
            }
        });
        const offInitialPos = transport.eventsOn('gps:initialPos', (ll: [number, number]) => {
            setMapPos({lat: ll[0], lon: ll[1]});
        });
        const offEpochPVT = transport.eventsOn('gps:epochPVT', (nav: any) => {
            pvtEpoch.current++;
            if (nav.posGeo) {
                setMapPos({lat: nav.posGeo.latLon[0], lon: nav.posGeo.latLon[1]});
                setNoFixSecs(0);
            } else {
                setNoFixSecs(prev => prev + 1);
            }
            if (nav.posECEF) {
                setScatterECEF(nav.posECEF.pos);
            }
            if (nav.velGeo && nav.velGeo.groundSpeed != null && nav.velGeo.course != null) {
                setMapCourse({course: nav.velGeo.course, groundSpeed: nav.velGeo.groundSpeed});
            } else {
                setMapCourse(null);
            }
            // Evict stale PVT rows: in each table, if any row was updated
            // this epoch, remove rows that weren't.
            const evict = <T extends {epoch: number}>(prev: Map<string, T>): Map<string, T> => {
                let maxEpoch = 0;
                for (const r of prev.values()) {
                    if (r.epoch > maxEpoch) maxEpoch = r.epoch;
                }
                if (maxEpoch === 0) return prev;
                let changed = false;
                const next = new Map<string, T>();
                for (const [k, r] of prev) {
                    if (r.epoch >= maxEpoch) next.set(k, r);
                    else changed = true;
                }
                return changed ? next : prev;
            };
            setPosRows(evict);
            setVelRows(evict);
            setTimeRows(evict);
        });
        const offBaseARP = transport.eventsOn('gps:basearp', (evt: {stationID: number; ecef: [number, number, number]}) => {
            setBaseARPs(prev => {
                const next = new Map(prev);
                next.set(evt.stationID, evt.ecef);
                return next;
            });
        });
        // The base ARP belongs to the active correction stream. Drop it when
        // a session starts fresh or stops, so a stale RTCM ARP doesn't linger
        // after switching to a source that sends no 1005/1006 (e.g. SPARTN).
        const offCorrARP = transport.eventsOn('gps:corrections', (evt: {state: string}) => {
            if (evt.state === 'connecting' || evt.state === 'stopped') {
                setBaseARPs(new Map());
            }
        });
        const offTime = transport.eventsOn('gps:time', (msg: any) => {
            setTimeMsg(msg as TimeMsg);
        });
        const offMsgSend = transport.eventsOn('gps:msgsend', (evt: MsgSendEvent) => {
            const {session, status, current, total, error} = evt;
            switch (status) {
                case 'started':
                    respSessionRef.current = session;
                    return;
                case 'sent':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        lines[idx] = {session, status: 'sending', index: current ?? 1, total: total ?? 1};
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
                    break;
                case 'cancelled':
                    break;
                case 'error':
                    setSendLines(prev => {
                        const lines = [...prev];
                        const idx = (current ?? 1) - 1;
                        lines[idx] = {session, status: 'error', index: current ?? 1, total: total ?? 1, error: error};
                        return lines;
                    });
                    break;
            }
        });
        const offResponse = transport.eventsOn('gps:response', (evt: ResponseLine) => {
            if (evt.session !== respSessionRef.current) return;
            setResponseLines(prev => {
                const next = [...prev, evt];
                const hasContent = !!(evt.text || evt.bin);
                if (hasContent && !prev.some(r => r.text || r.bin)) {
                    setSelectedResponseIndex(next.length - 1);
                }
                return next;
            });
        });
        return () => {
            offLog();
            offRcv();
            offSpeed();
            offMsg();
            offState();
            offInitialPos();
            offEpochPVT();
            offBaseARP();
            offCorrARP();
            offTime();
            offMsgSend();
            offResponse();
        };
    }, []);

    // Sync connection state with the backend on mount (late-joining
    // client, or HMR reload)
    useEffect(() => {
        transport.getConnState().then(async (s: string) => {
            setConnState(s as ConnState);
            if (s === 'disconnected') return;
            const r = await transport.getReceiverState();
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
                    const catalog = await transport.getAllSignals(gnss);
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
        const conn = transport.connection;
        if (!conn) return;
        if (connState !== 'disconnected') {
            respSessionRef.current = 0;
            try {
                await conn.disconnect();
            } catch (e) {
                addToast(e instanceof Error ? e.message : 'Disconnect failed', 'error');
            }
            return;
        }
        try {
            await conn.connect(device, speed);
            // connection status visible via indicator dot
        } catch (e) {
            addToast(e instanceof Error ? e.message : 'Connection failed', 'error');
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

    const handleTabChange = useCallback((tab: TabID) => {
        setActiveTab(tab);
    }, []);

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
                    onClick={() => handleTabChange('monitor')}
                >
                    Monitor
                </button>
                <button
                    class={activeTab === 'packets' ? tabBtnActive : tabBtnInactive}
                    onClick={() => handleTabChange('packets')}
                >
                    Packets
                </button>
                <button
                    class={activeTab === 'corrections' ? tabBtnActive : tabBtnInactive}
                    onClick={() => handleTabChange('corrections')}
                >
                    Corrections
                </button>
                <button
                    class={configDisabled ? tabBtnDisabled : (activeTab === 'config' ? tabBtnActive : tabBtnInactive)}
                    onClick={() => { if (!configDisabled) handleTabChange('config'); }}
                >
                    Configuration
                </button>
                {transport.msgFile && (
                    <button
                        class={activeTab === 'messages' ? tabBtnActive : tabBtnInactive}
                        onClick={() => handleTabChange('messages')}
                    >
                        Message file
                    </button>
                )}
            </div>

            {/* Tab content area */}
            <div class="flex-1 overflow-hidden" style="min-height: 80px;">
                {/* Monitor tab */}
                <div class={`h-full overflow-y-auto ${activeTab === 'monitor' ? '' : 'hidden'}`}>
                    <div
                        ref={monitorRowRef}
                        class="flex items-stretch"
                        style={{
                            height: 'calc(512px * var(--row-scale, 1))',
                            padding: 'calc(16px * var(--row-scale, 1))',
                            gap: 'calc(16px * var(--row-scale, 1))',
                            fontSize: 'calc(14px * var(--row-scale, 1))',
                        }}
                    >
                        {/* Column A: clock + summary on top, map below */}
                        <div class="flex-1 min-w-0 flex flex-col" style="gap: calc(16px * var(--row-scale, 1));">
                            <div class="flex shrink-0 items-start" style="gap: calc(16px * var(--row-scale, 1));">
                                <div class="shrink-0" style="height: calc(90px * var(--row-scale, 1));">
                                    <ClockPanel msg={timeMsg} />
                                </div>
                                <SummaryPanel msg={navEpochMsg} groundSpeed={mapCourse?.groundSpeed ?? null} />
                            </div>
                            <div class="flex-1 min-h-0">
                                <MapPanel pos={mapPos} course={mapCourse} noFixSecs={noFixSecs} />
                            </div>
                        </div>
                        {/* Column B: sky view (square) with legend overlaid in the bottom-right corner */}
                        <div class="shrink-0 aspect-square h-full relative">
                            <SkyViewPanel msg={satsMsg} />
                            <div class="absolute bottom-1 right-1">
                                <SkyViewLegend msg={satsMsg} />
                            </div>
                        </div>
                    </div>
                    <CollapsibleSection title="Position Scatter" variant="panel" defaultOpen={false}>
                        <ScatterPanel key={trackGen} ecef={scatterECEF} baseARPs={baseARPs} />
                    </CollapsibleSection>
                    <CollapsibleSection title="PVT Messages" variant="panel" open={pvtOpen} onToggle={setPvtOpen}>
                        <PVTPanel posRows={posRows} velRows={velRows} timeRows={timeRows} leapSecond={leapSecond} />
                    </CollapsibleSection>
                    <CollapsibleSection title="Satellite Signals" variant="panel" defaultOpen={false}>
                        <SignalsPanel msg={satsMsg} />
                    </CollapsibleSection>
                    <CollapsibleSection title="Survey" variant="panel" open={surveyOpen} onToggle={setSurveyOpen}>
                        <SurveyPanel msg={surveyMsg} />
                    </CollapsibleSection>
                </div>

                {/* Packets tab */}
                <div class={`h-full ${activeTab === 'packets' ? '' : 'hidden'}`}>
                    <PacketPanel visible={activeTab === 'packets'} connState={connState} />
                </div>

                {/* Corrections tab */}
                <div class={`h-full ${activeTab === 'corrections' ? '' : 'hidden'}`}>
                    <CorrectionsPanel connState={connState} />
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
                        clearRespSession={() => { respSessionRef.current = 0; }}
                        speed={speed}
                    />
                </div>

                {/* Messages tab */}
                {transport.msgFile && <div class={`h-full ${activeTab === 'messages' ? '' : 'hidden'}`}>
                    <MsgFilePanel
                        connState={connState}
                        msgFilePath={msgFilePath}
                        setMsgFilePath={setMsgFilePath}
                        msgFileTags={msgFileTags}
                        setMsgFileTags={setMsgFileTags}
                        selectedTagIndex={selectedTagIndex}
                        setSelectedTagIndex={setSelectedTagIndex}
                        activeTagIndex={activeTagIndex}
                        setActiveTagIndex={setActiveTagIndex}
                        tagArmed={tagArmed}
                        setTagArmed={setTagArmed}
                        sendLines={sendLines}
                        setSendLines={setSendLines}
                        responseLines={responseLines}
                        setResponseLines={setResponseLines}
                        selectedResponseIndex={selectedResponseIndex}
                        setSelectedResponseIndex={setSelectedResponseIndex}
                        clearRespSession={() => { respSessionRef.current = 0; }}
                        addToast={addToast}
                        defaultPort={typeof configProps?.port === 'string' ? configProps.port : ''}
                    />
                </div>}
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

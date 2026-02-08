import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Connect, Disconnect, GetAllSignals, GetReceiverState, IsConnected} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {ConnectionPanel} from './connection-panel';
import {CollapsibleSection} from './collapsible-section';
import {ConfigPanel} from './config-panel';
import {MonitorPanel} from './monitor-panel';
import {LoggingPanel} from './logging-panel';
import {TimePanel} from './time-panel';
import {SurveyPanel} from './survey-panel';

export type ReceiverState =
    | {status: 'disconnected'}
    | {status: 'probing'}
    | {status: 'identified'; vendor: string; hardware: string; firmware: string; supportedGNSS: string[]; packetFormats: string[]}
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

interface MsgEvent {
    kind: string;
    msg: any;
    time: string;
}

type TabID = 'monitor' | 'packets' | 'config';

const tabBtnBase = 'px-5 py-2 text-sm font-medium border-b-2 cursor-pointer bg-transparent';
const tabBtnActive = tabBtnBase + ' border-blue-600 text-blue-600 bg-gray-50 dark:bg-gray-900';
const tabBtnInactive = tabBtnBase + ' border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-900';
const tabBtnDisabled = tabBtnBase + ' border-transparent text-gray-300 dark:text-gray-600 cursor-not-allowed';

let toastId = 0;

export function App() {
    const [connected, setConnected] = useState(false);
    const [device, setDevice] = useState('/dev/cu.usbmodem312301');
    const [speed, setSpeed] = useState(9600);
    const [statusText, setStatusText] = useState('Disconnected');
    const [receiver, setReceiver] = useState<ReceiverState>({status: 'disconnected'});
    const [receiverInfo, setReceiverInfo] = useState<main.ReceiverInfo | null>(null);
    const [signalCatalog, setSignalCatalog] = useState<main.GNSSInfo[]>([]);
    const [selectedSignals, setSelectedSignals] = useState<Set<number>>(new Set());
    const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
    const [packetEntries, setPacketEntries] = useState<PacketEntry[]>([]);
    const [toasts, setToasts] = useState<Toast[]>([]);
    const [, setOperation] = useState<OperationState>({status: 'idle', label: ''});
    const [timeMsg, setTimeMsg] = useState<TimeMsg | null>(null);
    const [surveyMsg, setSurveyMsg] = useState<SurveyMsg | null>(null);
    const [leapSecond, setLeapSecond] = useState<LeapSecondState | null>(null);
    const [activeTab, setActiveTab] = useState<TabID>('monitor');
    const [timeOpen, setTimeOpen] = useState(true);
    const [surveyOpen, setSurveyOpen] = useState(false);
    const surveyAutoExpanded = useRef(false);
    const [logHeight, setLogHeight] = useState(150);
    const dragging = useRef(false);

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

    const addToast = useCallback((message: string, type: 'success' | 'error') => {
        const id = ++toastId;
        setToasts(prev => [...prev, {id, message, type}]);
        setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 3000);
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
                setReceiver({
                    status: 'identified',
                    vendor: evt.vendor || '',
                    hardware: evt.hardware || '',
                    firmware: evt.firmware || '',
                    supportedGNSS: evt.supportedGNSS || [],
                    packetFormats: evt.packetFormats || [],
                });
            }
        });
        const offMsg = EventsOn('gps:msg', (evt: MsgEvent) => {
            switch (evt.kind) {
                case 'time':
                    setTimeMsg(evt.msg as TimeMsg);
                    break;
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
            }
        });
        return () => {
            if (typeof offLog === 'function') offLog(); else EventsOff('gps:log');
            if (typeof offPkt === 'function') offPkt(); else EventsOff('gps:packet');
            if (typeof offRcv === 'function') offRcv(); else EventsOff('gps:receiver');
            if (typeof offMsg === 'function') offMsg(); else EventsOff('gps:msg');
        };
    }, []);

    // Sync connection state with Go backend on mount (after HMR reload)
    useEffect(() => {
        IsConnected().then(async c => {
            if (!c) return;
            setConnected(true);
            setStatusText('Connected');
            const r = await GetReceiverState();
            if (r.ok) {
                setReceiver({
                    status: 'identified',
                    vendor: r.vendor || '',
                    hardware: r.hardware || '',
                    firmware: r.firmware || '',
                    supportedGNSS: r.supportedGNSS || [],
                    packetFormats: r.packetFormats || [],
                });
            }
            const catalog = await GetAllSignals();
            if (catalog) setSignalCatalog(catalog);
        }).catch(() => {});
    }, []);

    const handleConfigReadback = useCallback((info: main.ReceiverInfo) => {
        setReceiverInfo(prev => {
            if (!prev) return info;
            return {...prev, config: info.config, signals: info.signals, signalIndices: info.signalIndices};
        });
    }, []);

    const handleConnect = useCallback(async () => {
        if (connected) {
            await Disconnect();
            setConnected(false);
            setStatusText('Disconnected');
            return;
        }
        setStatusText('Connecting...');
        const r = await Connect(device, speed);
        if (r.ok) {
            setConnected(true);
            setStatusText('Connected');
            addToast('Connected to ' + device, 'success');
            try {
                const catalog = await GetAllSignals();
                setSignalCatalog(catalog);
            } catch (e) { /* ignore */ }
        } else {
            setStatusText('Connection failed');
            addToast(r.error || 'Connection failed', 'error');
        }
    }, [connected, device, speed, addToast]);

    // Receiver identity string for connection bar
    const receiverIdent = receiver.status === 'identified'
        ? `${receiver.hardware} (FW ${receiver.firmware})`
        : receiver.status === 'probing' ? 'Identifying...' : '';

    const configDisabled = receiver.status !== 'identified';

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
            />

            {/* Tab bar */}
            <div class="flex border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 shrink-0">
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
            </div>

            {/* Tab content area */}
            <div class="flex-1 overflow-hidden" style="min-height: 80px;">
                {/* Monitor tab */}
                <div class={`h-full overflow-y-auto ${activeTab === 'monitor' ? '' : 'hidden'}`}>
                    <CollapsibleSection title="Time" variant="panel" open={timeOpen} onToggle={setTimeOpen}>
                        <TimePanel msg={timeMsg} leapSecond={leapSecond} />
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
                    />
                </div>

                {/* Configuration tab */}
                <div class={`h-full ${activeTab === 'config' ? '' : 'hidden'}`}>
                    <ConfigPanel
                        connected={connected}
                        visible={activeTab === 'config'}
                        receiverInfo={receiverInfo}
                        signalCatalog={signalCatalog}
                        selectedSignals={selectedSignals}
                        setSelectedSignals={setSelectedSignals}
                        setStatusText={setStatusText}
                        setOperation={setOperation}
                        addToast={addToast}
                        onConfigReadback={handleConfigReadback}
                    />
                </div>
            </div>

            {/* Drag splitter */}
            <div
                class="shrink-0 h-1 cursor-row-resize border-y border-gray-200 dark:border-gray-700"
                onMouseDown={onSplitterMouseDown}
            />

            {/* Activity log (always visible) */}
            <div class="shrink-0 overflow-hidden" style={{height: logHeight + 'px'}}>
                <LoggingPanel logEntries={logEntries} setLogEntries={setLogEntries} />
            </div>

            {/* Status bar */}
            <div class="bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 px-5 py-1 text-xs text-gray-500 dark:text-gray-400 shrink-0">
                {statusText}
            </div>

            {/* Toasts */}
            <div class="fixed bottom-10 right-5 flex flex-col gap-2 z-50">
                {toasts.map(t => (
                    <div
                        key={t.id}
                        class={`px-4 py-2 rounded text-sm text-black animate-[fadeIn_0.2s] ${
                            t.type === 'success' ? 'bg-green-400' : 'bg-red-400'
                        }`}
                    >
                        {t.message}
                    </div>
                ))}
            </div>
        </>
    );
}

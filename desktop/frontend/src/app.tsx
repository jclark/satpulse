import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback} from 'preact/hooks';
import {Group, Panel, Separator} from 'react-resizable-panels';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Connect, Disconnect, GetAllSignals} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {ConnectionPanel} from './connection-panel';
import {ReceiverPanel, ReceiverState} from './receiver-panel';
import {ConfigPanel} from './config-panel';
import {MonitorPanel} from './monitor-panel';
import {LoggingPanel} from './logging-panel';
import {TimePanel} from './time-panel';
import {SurveyPanel} from './survey-panel';

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

export interface PanelVisibility {
    receiver: boolean;
    config: boolean;
    time: boolean;
    survey: boolean;
    monitor: boolean;
    logging: boolean;
}

export type PanelID = keyof PanelVisibility;

const separatorH = 'group relative flex items-center justify-center bg-gray-200 dark:bg-gray-700 hover:bg-blue-500/30 active:bg-blue-500/40 transition-colors w-1 cursor-col-resize';
const separatorV = 'group relative flex items-center justify-center bg-gray-200 dark:bg-gray-700 hover:bg-blue-500/30 active:bg-blue-500/40 transition-colors h-1 cursor-row-resize';

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
    const [operation, setOperation] = useState<OperationState>({status: 'idle', label: ''});
    const [timeMsg, setTimeMsg] = useState<TimeMsg | null>(null);
    const [surveyMsg, setSurveyMsg] = useState<SurveyMsg | null>(null);
    const [leapSecond, setLeapSecond] = useState<LeapSecondState | null>(null);
    const [panels, setPanels] = useState<PanelVisibility>({
        receiver: true, config: true, time: true, survey: true, monitor: true, logging: true,
    });

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
                    // LeapSecond embeds ptime.LeapSecond with UTCOffAfter as current offset
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

    const togglePanel = useCallback((id: PanelID) => {
        setPanels(prev => ({...prev, [id]: !prev[id]}));
    }, []);

    const showLeft = panels.receiver || panels.config;
    const showCenter = panels.time || panels.survey;
    const showRight = panels.monitor;
    const showMiddle = showLeft || showCenter || showRight;

    // Collect visible center panels for dynamic sub-splitting
    const centerPanels: preact.ComponentChildren[] = [];
    if (panels.time) centerPanels.push(<TimePanel msg={timeMsg} leapSecond={leapSecond} />);
    if (panels.survey) centerPanels.push(<SurveyPanel msg={surveyMsg} />);

    return (
        <>
            {/* Connection strip (fixed height, outside panel groups) */}
            <ConnectionPanel
                connected={connected}
                device={device}
                setDevice={setDevice}
                speed={speed}
                setSpeed={setSpeed}
                onConnect={handleConnect}
                panelVisibility={panels}
                onTogglePanel={togglePanel}
            />

            {/* Main resizable area */}
            <Group orientation="vertical" className="flex-1">
                {showMiddle && (
                    <Panel id="middle" minSize="20%">
                        <Group orientation="horizontal" className="h-full">
                            {showLeft && (
                                <Panel id="left" defaultSize="35%" minSize="15%">
                                    {panels.receiver && panels.config ? (
                                        <Group orientation="vertical" className="h-full">
                                            <Panel id="receiver" defaultSize="40%" minSize="10%">
                                                <ReceiverPanel receiver={receiver} />
                                            </Panel>
                                            <Separator className={separatorV} />
                                            <Panel id="config" minSize="10%">
                                                <ConfigPanel
                                                    connected={connected}
                                                    receiverInfo={receiverInfo}
                                                    signalCatalog={signalCatalog}
                                                    selectedSignals={selectedSignals}
                                                    setSelectedSignals={setSelectedSignals}
                                                    setStatusText={setStatusText}
                                                    setOperation={setOperation}
                                                    addToast={addToast}
                                                    onConfigReadback={handleConfigReadback}
                                                />
                                            </Panel>
                                        </Group>
                                    ) : panels.receiver ? (
                                        <ReceiverPanel receiver={receiver} />
                                    ) : (
                                        <ConfigPanel
                                            connected={connected}
                                            receiverInfo={receiverInfo}
                                            signalCatalog={signalCatalog}
                                            selectedSignals={selectedSignals}
                                            setSelectedSignals={setSelectedSignals}
                                            setStatusText={setStatusText}
                                            setOperation={setOperation}
                                            addToast={addToast}
                                            onConfigReadback={handleConfigReadback}
                                        />
                                    )}
                                </Panel>
                            )}
                            {showLeft && showCenter && <Separator className={separatorH} />}
                            {showCenter && (
                                <Panel id="center" defaultSize="30%" minSize="15%">
                                    {centerPanels.length > 1 ? (
                                        <Group orientation="vertical" className="h-full">
                                            <Panel id="time" defaultSize="50%" minSize="10%">
                                                {centerPanels[0]}
                                            </Panel>
                                            <Separator className={separatorV} />
                                            <Panel id="survey" minSize="10%">
                                                {centerPanels[1]}
                                            </Panel>
                                        </Group>
                                    ) : centerPanels[0]}
                                </Panel>
                            )}
                            {(showLeft || showCenter) && showRight && <Separator className={separatorH} />}
                            {showRight && (
                                <Panel id="right" minSize="20%">
                                    <MonitorPanel
                                        packetEntries={packetEntries}
                                        setPacketEntries={setPacketEntries}
                                    />
                                </Panel>
                            )}
                        </Group>
                    </Panel>
                )}
                {showMiddle && panels.logging && <Separator className={separatorV} />}
                {panels.logging && (
                    <Panel id="logging" defaultSize="25%" minSize="10%">
                        <LoggingPanel logEntries={logEntries} setLogEntries={setLogEntries} operation={operation} />
                    </Panel>
                )}
            </Group>

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

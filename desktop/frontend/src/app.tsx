import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Connect, Disconnect, GetAllSignals, StartCapture, StopCapture} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {ReceiverPanel} from './receiver-panel';
import {ConfigPanel} from './config-panel';
import {MonitorPanel} from './monitor-panel';

type TabID = 'receiver' | 'config' | 'monitor';

interface Toast {
    id: number;
    message: string;
    type: 'success' | 'error';
}

export interface LogEntry {
    level: string;
    message: string;
    time: string;
}

export interface PacketEntry {
    tag: string;
    data: string;
    timestamp: string;
}

const speeds = [9600, 38400, 57600, 115200, 230400, 460800, 921600];
let toastId = 0;

export function App() {
    const [connected, setConnected] = useState(false);
    const [device, setDevice] = useState('/dev/cu.usbmodem312301');
    const [speed, setSpeed] = useState(9600);
    const [statusText, setStatusText] = useState('Disconnected');
    const [activeTab, setActiveTab] = useState<TabID>('receiver');
    const [receiverInfo, setReceiverInfo] = useState<main.ReceiverInfo | null>(null);
    const [signalCatalog, setSignalCatalog] = useState<main.GNSSInfo[]>([]);
    const [selectedSignals, setSelectedSignals] = useState<Set<number>>(new Set());
    const [capturing, setCapturing] = useState(false);
    const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
    const [packetEntries, setPacketEntries] = useState<PacketEntry[]>([]);
    const [toasts, setToasts] = useState<Toast[]>([]);

    const addToast = useCallback((message: string, type: 'success' | 'error') => {
        const id = ++toastId;
        setToasts(prev => [...prev, {id, message, type}]);
        setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 3000);
    }, []);

    // Wails event subscriptions
    useEffect(() => {
        const offLog = EventsOn('gps:log', (evt: LogEntry) => {
            setLogEntries(prev => [...prev.slice(-199), evt]);
        });
        const offPkt = EventsOn('gps:packet', (pkt: PacketEntry) => {
            setPacketEntries(prev => [...prev.slice(-999), pkt]);
        });
        return () => {
            if (typeof offLog === 'function') offLog(); else EventsOff('gps:log');
            if (typeof offPkt === 'function') offPkt(); else EventsOff('gps:packet');
        };
    }, []);

    const handleConnect = useCallback(async () => {
        if (connected) {
            await Disconnect();
            setConnected(false);
            setCapturing(false);
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

    const handleToggleCapture = useCallback(async () => {
        if (capturing) {
            await StopCapture();
            setCapturing(false);
            setStatusText('Capture stopped');
            return;
        }
        const r = await StartCapture();
        if (r.ok) {
            setCapturing(true);
            setStatusText('Capturing packets...');
        } else {
            addToast(r.error || 'Capture failed', 'error');
        }
    }, [capturing, addToast]);

    const tabs: {id: TabID; label: string}[] = [
        {id: 'receiver', label: 'Receiver'},
        {id: 'config', label: 'Configure'},
        {id: 'monitor', label: 'Packet monitor'},
    ];

    return (
        <>
            {/* Header */}
            <header class="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-5 py-3 flex items-center gap-4 shrink-0">
                <h1 class="text-base font-semibold whitespace-nowrap">SatPulse GPS</h1>
                <div class="flex items-center gap-2 ml-auto">
                    <div class={`w-2.5 h-2.5 rounded-full shrink-0 ${connected ? 'bg-green-400' : 'bg-gray-400'}`} />
                    <label class="text-xs text-gray-500 dark:text-gray-400">Device</label>
                    <input
                        type="text"
                        class="bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs w-44"
                        value={device}
                        onInput={(e) => setDevice((e.target as HTMLInputElement).value)}
                    />
                    <label class="text-xs text-gray-500 dark:text-gray-400">Speed</label>
                    <select
                        class="bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs w-24"
                        value={speed}
                        onChange={(e) => setSpeed(parseInt((e.target as HTMLSelectElement).value))}
                    >
                        {speeds.map(s => <option key={s} value={s}>{s}</option>)}
                    </select>
                    <button
                        class={`px-3.5 py-1 rounded text-xs font-medium border cursor-pointer whitespace-nowrap ${
                            connected
                                ? 'bg-gray-200 dark:bg-gray-700 border-gray-300 dark:border-gray-600 text-gray-900 dark:text-gray-100 hover:bg-gray-300 dark:hover:bg-gray-600'
                                : 'bg-blue-600 border-blue-600 text-white hover:bg-blue-700'
                        }`}
                        onClick={handleConnect}
                    >
                        {connected ? 'Disconnect' : 'Connect'}
                    </button>
                </div>
            </header>

            {/* Tab bar */}
            <div class="flex border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 shrink-0">
                {tabs.map(tab => (
                    <button
                        key={tab.id}
                        class={`px-5 py-2 text-xs cursor-pointer border-b-2 bg-transparent ${
                            activeTab === tab.id
                                ? 'text-gray-900 dark:text-gray-100 border-blue-600 dark:border-blue-500'
                                : 'text-gray-500 dark:text-gray-400 border-transparent hover:text-gray-900 dark:hover:text-gray-100'
                        }`}
                        onClick={() => setActiveTab(tab.id)}
                    >
                        {tab.label}
                    </button>
                ))}
            </div>

            {/* Main content */}
            <main class="flex-1 overflow-y-auto p-5">
                {activeTab === 'receiver' && (
                    <ReceiverPanel
                        connected={connected}
                        receiverInfo={receiverInfo}
                        setReceiverInfo={setReceiverInfo}
                        setSelectedSignals={setSelectedSignals}
                        setStatusText={setStatusText}
                        logEntries={logEntries}
                        setLogEntries={setLogEntries}
                        addToast={addToast}
                    />
                )}
                {activeTab === 'config' && (
                    <ConfigPanel
                        connected={connected}
                        receiverInfo={receiverInfo}
                        signalCatalog={signalCatalog}
                        selectedSignals={selectedSignals}
                        setSelectedSignals={setSelectedSignals}
                        setStatusText={setStatusText}
                        logEntries={logEntries}
                        addToast={addToast}
                    />
                )}
                {activeTab === 'monitor' && (
                    <MonitorPanel
                        connected={connected}
                        capturing={capturing}
                        onToggleCapture={handleToggleCapture}
                        packetEntries={packetEntries}
                        setPacketEntries={setPacketEntries}
                    />
                )}
            </main>

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

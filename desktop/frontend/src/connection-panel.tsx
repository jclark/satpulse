import {h, Fragment} from 'preact';
import {useState, useRef, useEffect} from 'preact/hooks';

interface PanelVisibility {
    receiver: boolean;
    config: boolean;
    time: boolean;
    survey: boolean;
    monitor: boolean;
    logging: boolean;
}

type PanelID = keyof PanelVisibility;

interface Props {
    connected: boolean;
    device: string;
    setDevice: (d: string) => void;
    speed: number;
    setSpeed: (s: number) => void;
    onConnect: () => void;
    panelVisibility: PanelVisibility;
    onTogglePanel: (id: PanelID) => void;
}

const speeds = [9600, 38400, 57600, 115200, 230400, 460800, 921600];

const panelLabels: {id: PanelID; label: string}[] = [
    {id: 'receiver', label: 'Receiver'},
    {id: 'config', label: 'Configure'},
    {id: 'time', label: 'Time'},
    {id: 'survey', label: 'Survey'},
    {id: 'monitor', label: 'Packet monitor'},
    {id: 'logging', label: 'Activity log'},
];

export function ConnectionPanel({connected, device, setDevice, speed, setSpeed, onConnect, panelVisibility, onTogglePanel}: Props) {
    const [menuOpen, setMenuOpen] = useState(false);
    const menuRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (!menuOpen) return;
        const handler = (e: MouseEvent) => {
            if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
        };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, [menuOpen]);

    return (
        <header class="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-5 py-3 flex items-center gap-4 shrink-0">
            <h1 class="text-base font-semibold whitespace-nowrap">SatPulse GPS</h1>
            <div class="relative" ref={menuRef}>
                <button
                    class="px-3 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-gray-300 dark:hover:bg-gray-600"
                    onClick={() => setMenuOpen(v => !v)}
                >
                    Panels
                </button>
                {menuOpen && (
                    <div class="absolute left-0 top-full mt-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded shadow-lg z-50 min-w-40 py-1">
                        {panelLabels.map(p => (
                            <label key={p.id} class="flex items-center gap-2 px-3 py-1.5 text-xs cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700">
                                <input
                                    type="checkbox"
                                    class="accent-blue-600"
                                    checked={panelVisibility[p.id]}
                                    onChange={() => onTogglePanel(p.id)}
                                />
                                {p.label}
                            </label>
                        ))}
                    </div>
                )}
            </div>
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
                    onClick={onConnect}
                >
                    {connected ? 'Disconnect' : 'Connect'}
                </button>
            </div>
        </header>
    );
}

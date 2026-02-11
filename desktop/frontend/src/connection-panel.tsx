import {h} from 'preact';
import {useState, useRef, useEffect, useCallback} from 'preact/hooks';

export interface PortInfo {
    device: string;
    display: string;
}

interface Props {
    connected: boolean;
    device: string;
    setDevice: (d: string) => void;
    speed: number;
    setSpeed: (s: number) => void;
    onConnect: () => void;
    receiverIdent: string;
    ports: PortInfo[];
    onRefreshPorts: () => void;
}

const speeds = [9600, 38400, 57600, 115200, 230400, 460800, 921600];

const inputCls = 'bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs';

export function ConnectionPanel({connected, device, setDevice, speed, setSpeed, onConnect, receiverIdent, ports, onRefreshPorts}: Props) {
    const [open, setOpen] = useState(false);
    const wrapRef = useRef<HTMLDivElement>(null);

    // Close dropdown on outside click
    useEffect(() => {
        if (!open) return;
        const handler = (e: MouseEvent) => {
            if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
        };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, [open]);

    const toggleDropdown = useCallback(() => {
        if (!open) onRefreshPorts();
        setOpen(v => !v);
    }, [open, onRefreshPorts]);

    const selectPort = useCallback((p: PortInfo) => {
        setDevice(p.device);
        setOpen(false);
    }, [setDevice]);

    return (
        <header class="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-5 py-3 flex items-center gap-4 shrink-0">
            <h1 class="text-base font-semibold whitespace-nowrap">SatPulse</h1>
            {receiverIdent && (
                <span class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{receiverIdent}</span>
            )}
            <div class="flex items-center gap-2 ml-auto">
                <div class={`w-2.5 h-2.5 rounded-full shrink-0 ${connected ? 'bg-green-400' : 'bg-gray-400'}`} />
                <label class="text-xs text-gray-500 dark:text-gray-400">Device</label>
                <div class="relative" ref={wrapRef}>
                    <div class="flex">
                        <input
                            type="text"
                            class={inputCls + ' w-44 rounded-r-none border-r-0'}
                            value={device}
                            onInput={(e) => setDevice((e.target as HTMLInputElement).value)}
                            placeholder="device path"
                        />
                        <button
                            type="button"
                            class={inputCls + ' rounded-l-none px-1.5 cursor-pointer hover:bg-gray-200 dark:hover:bg-gray-700'}
                            onClick={toggleDropdown}
                            aria-label="Select port"
                        >
                            <svg class="w-3 h-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M3 5l3 3 3-3" />
                            </svg>
                        </button>
                    </div>
                    {open && (
                        <ul class="absolute z-50 mt-0.5 left-0 right-0 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded shadow-lg max-h-48 overflow-y-auto">
                            {ports.length === 0 && (
                                <li class="px-2 py-1.5 text-xs text-gray-400 italic">No ports found</li>
                            )}
                            {ports.map(p => (
                                <li
                                    key={p.device}
                                    class="px-2 py-1.5 text-xs cursor-pointer hover:bg-blue-50 dark:hover:bg-gray-700 text-gray-900 dark:text-gray-100"
                                    onClick={() => selectPort(p)}
                                >
                                    {p.display}
                                </li>
                            ))}
                        </ul>
                    )}
                </div>
                <label class="text-xs text-gray-500 dark:text-gray-400">Speed</label>
                <select
                    class={inputCls + ' w-24'}
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

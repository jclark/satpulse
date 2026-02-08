import {h} from 'preact';

interface Props {
    connected: boolean;
    device: string;
    setDevice: (d: string) => void;
    speed: number;
    setSpeed: (s: number) => void;
    onConnect: () => void;
    receiverIdent: string;
}

const speeds = [9600, 38400, 57600, 115200, 230400, 460800, 921600];

export function ConnectionPanel({connected, device, setDevice, speed, setSpeed, onConnect, receiverIdent}: Props) {
    return (
        <header class="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-5 py-3 flex items-center gap-4 shrink-0">
            <h1 class="text-base font-semibold whitespace-nowrap">SatPulse</h1>
            {receiverIdent && (
                <span class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{receiverIdent}</span>
            )}
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

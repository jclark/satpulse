import {h} from 'preact';
import {useEffect, useRef} from 'preact/hooks';
import type {PacketEntry} from './app';

interface Props {
    connected: boolean;
    capturing: boolean;
    onToggleCapture: () => void;
    packetEntries: PacketEntry[];
    setPacketEntries: (fn: (prev: PacketEntry[]) => PacketEntry[]) => void;
}

export function MonitorPanel({connected, capturing, onToggleCapture, packetEntries, setPacketEntries}: Props) {
    const logRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        const el = logRef.current;
        if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 100) {
            el.scrollTop = el.scrollHeight;
        }
    }, [packetEntries]);

    return (
        <div>
            <div class="flex gap-2 mb-3">
                <button
                    class="px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100"
                    disabled={!connected}
                    onClick={onToggleCapture}
                >
                    {capturing ? 'Stop capture' : 'Start capture'}
                </button>
                <button
                    class="px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white"
                    onClick={() => setPacketEntries(() => [])}
                >
                    Clear
                </button>
            </div>
            <div
                ref={logRef}
                class="font-mono text-xs leading-relaxed bg-gray-900 dark:bg-black border border-gray-200 dark:border-gray-700 rounded p-2.5 overflow-y-auto whitespace-pre-wrap break-all"
                style="height: calc(100vh - 200px)"
            >
                {packetEntries.map((pkt, i) => (
                    <div key={i}>
                        <span class="text-gray-500 dark:text-gray-400">{pkt.timestamp}</span>{' '}
                        <span class="text-blue-600 dark:text-blue-500">[{pkt.tag}]</span>{' '}
                        <span class="text-green-400">{pkt.data.trim()}</span>
                    </div>
                ))}
            </div>
        </div>
    );
}

import {h, Fragment} from 'preact';
import {useEffect, useRef} from 'preact/hooks';
import type {PacketEntry} from './app';

interface Props {
    packetEntries: PacketEntry[];
    setPacketEntries: (fn: (prev: PacketEntry[]) => PacketEntry[]) => void;
}

function stripTrailingEOL(s: string): string {
    if (s.endsWith('\r\n')) return s.slice(0, -2);
    if (s.endsWith('\r') || s.endsWith('\n')) return s.slice(0, -1);
    return s;
}

const controlPictures: Record<string, string> = {'\t': '\u2409', '\n': '\u240A', '\r': '\u240D'};

function escapeControl(s: string): string {
    return stripTrailingEOL(s).replace(/[\t\r\n]/g, ch => controlPictures[ch]);
}

export function MonitorPanel({packetEntries, setPacketEntries}: Props) {
    const logRef = useRef<HTMLDivElement>(null);
    const mounted = useRef(true);

    useEffect(() => {
        mounted.current = true;
        return () => { mounted.current = false; };
    }, []);

    useEffect(() => {
        const el = logRef.current;
        if (!el) return;
        if (mounted.current) {
            // Always scroll to bottom on first render (tab switch / mount)
            mounted.current = false;
            el.scrollTop = el.scrollHeight;
            return;
        }
        if (el.scrollHeight - el.scrollTop - el.clientHeight < 100) {
            el.scrollTop = el.scrollHeight;
        }
    }, [packetEntries]);

    return (
        <div class="flex flex-col h-full">
            <div class="flex gap-2 px-3 py-1.5 shrink-0">
                <button
                    class="px-2.5 py-0.5 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white"
                    onClick={() => setPacketEntries(() => [])}
                >
                    Clear
                </button>
            </div>
            <div
                ref={logRef}
                class="flex-1 font-mono text-xs leading-relaxed bg-gray-900 dark:bg-black border-t border-gray-200 dark:border-gray-700 px-2.5 py-1.5 overflow-y-auto whitespace-pre-wrap break-all"
            >
                {packetEntries.map((pkt, i) => (
                    <div key={i}>
                        <span class="text-gray-500 dark:text-gray-400">{pkt.timestamp}</span>{' '}
                        <span class="text-blue-600 dark:text-blue-500">[{pkt.tag || (pkt.bin ? 'bin' : 'ascii')}]</span>{' '}
                        {pkt.msg && <><span class="text-yellow-400">{pkt.msg}</span>{' '}</>}
                        <span class="text-green-400">{pkt.ascii ? (pkt.tag ? stripTrailingEOL(pkt.ascii) : escapeControl(pkt.ascii)) : pkt.bin}</span>
                    </div>
                ))}
            </div>
        </div>
    );
}

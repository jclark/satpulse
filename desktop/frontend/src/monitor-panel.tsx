import {h, Fragment} from 'preact';
import {useState, useEffect, useRef, useMemo, useCallback} from 'preact/hooks';
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

function matchesFilter(pkt: PacketEntry, q: string): boolean {
    if (pkt.tag && pkt.tag.toLowerCase().includes(q)) return true;
    if (pkt.msg && pkt.msg.toLowerCase().includes(q)) return true;
    if (pkt.ascii && pkt.ascii.toLowerCase().includes(q)) return true;
    if (pkt.bin && pkt.bin.toLowerCase().includes(q)) return true;
    return false;
}

const btnClass = 'px-2.5 py-0.5 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white';
const selectClass = 'bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-1.5 py-0.5 rounded text-xs';

export function MonitorPanel({packetEntries, setPacketEntries}: Props) {
    const logRef = useRef<HTMLDivElement>(null);
    const [frozen, setFrozen] = useState(false);
    const [filter, setFilter] = useState('');
    const frozenSnapshotRef = useRef<PacketEntry[]>([]);

    // When freezing, snapshot the current entries
    const handleFreeze = useCallback(() => {
        setFrozen(prev => {
            if (!prev) {
                frozenSnapshotRef.current = packetEntries;
            }
            return !prev;
        });
    }, [packetEntries]);

    // When resuming, scroll to bottom
    useEffect(() => {
        if (!frozen) {
            const el = logRef.current;
            if (el) el.scrollTop = el.scrollHeight;
        }
    }, [frozen]);

    // Auto-scroll when not frozen
    useEffect(() => {
        if (frozen) return;
        const el = logRef.current;
        if (!el) return;
        if (el.scrollHeight - el.scrollTop - el.clientHeight < 100) {
            el.scrollTop = el.scrollHeight;
        }
    }, [packetEntries, frozen]);

    const displayEntries = frozen ? frozenSnapshotRef.current : packetEntries;
    const filtered = useMemo(() => {
        if (!filter) return displayEntries;
        const q = filter.toLowerCase();
        return displayEntries.filter(pkt => matchesFilter(pkt, q));
    }, [displayEntries, filter]);

    return (
        <div class="flex flex-col h-full">
            <div
                ref={logRef}
                class="flex-1 font-mono text-xs leading-relaxed bg-gray-900 dark:bg-black px-2.5 py-1.5 overflow-y-auto whitespace-pre-wrap break-all"
            >
                {filtered.map((pkt, i) => (
                    <div key={i}>
                        <span class="text-gray-500 dark:text-gray-400">{pkt.timestamp}</span>{' '}
                        <span class="text-blue-600 dark:text-blue-500">[{pkt.tag || (pkt.bin ? 'bin' : 'ascii')}]</span>{' '}
                        {pkt.msg && <><span class="text-yellow-400">{pkt.msg}</span>{' '}</>}
                        <span class="text-green-400">{pkt.ascii ? (pkt.tag ? stripTrailingEOL(pkt.ascii) : escapeControl(pkt.ascii)) : pkt.bin}</span>
                    </div>
                ))}
            </div>
            {/* Bottom toolbar */}
            <div class="flex items-center gap-2 px-3 py-1.5 shrink-0 border-t border-gray-200 dark:border-gray-700">
                <button
                    class={`${btnClass} ${frozen ? 'bg-amber-500! border-amber-500! text-black!' : ''}`}
                    onClick={handleFreeze}
                >
                    {frozen ? 'Frozen' : 'Freeze'}
                </button>
                <input
                    type="text"
                    class={selectClass + ' w-36'}
                    placeholder="Filter..."
                    value={filter}
                    onInput={e => setFilter((e.target as HTMLInputElement).value)}
                />
                <button
                    class={btnClass}
                    onClick={() => setPacketEntries(() => [])}
                >
                    Clear
                </button>
                <span class="text-xs text-gray-500 dark:text-gray-400 ml-auto">
                    {filtered.length}/{displayEntries.length}
                </span>
            </div>
        </div>
    );
}

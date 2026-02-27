import {h, Fragment} from 'preact';
import {useState, useEffect, useRef, useMemo, useCallback} from 'preact/hooks';
import type {PacketEntry} from './app';
import {Button, Input} from './ui';

interface Props {
    packetEntries: PacketEntry[];
    setPacketEntries: (fn: (prev: PacketEntry[]) => PacketEntry[]) => void;
    visible: boolean;
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

export function MonitorPanel({packetEntries, setPacketEntries, visible}: Props) {
    const logRef = useRef<HTMLDivElement>(null);
    const [frozen, setFrozen] = useState(false);
    const [filter, setFilter] = useState('');
    const frozenSnapshotRef = useRef<PacketEntry[]>([]);

    const handleFreeze = useCallback(() => {
        setFrozen(prev => {
            if (!prev) {
                frozenSnapshotRef.current = packetEntries;
            }
            return !prev;
        });
    }, [packetEntries]);

    useEffect(() => {
        if (visible && !frozen) {
            const el = logRef.current;
            if (el) el.scrollTop = el.scrollHeight;
        }
    }, [visible, frozen]);

    useEffect(() => {
        if (!frozen) {
            const el = logRef.current;
            if (el) el.scrollTop = el.scrollHeight;
        }
    }, [frozen]);

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
        <div class="flex h-full flex-col">
            <div ref={logRef} class="flex-1 overflow-y-auto break-all whitespace-pre-wrap bg-surface-1 px-2.5 py-1.5 font-mono text-xs leading-relaxed">
                {filtered.map((pkt, i) => (
                    <div key={i}>
                        <span class="text-text-muted">{pkt.timestamp}</span>{' '}
                        <span class="text-accent">[{pkt.tag || (pkt.bin ? 'bin' : 'ascii')}]</span>{' '}
                        {pkt.msg && (
                            <Fragment>
                                <span class="text-warning">{pkt.msg}</span>{' '}
                            </Fragment>
                        )}
                        <span class="text-text-primary">{pkt.ascii ? (pkt.tag ? stripTrailingEOL(pkt.ascii) : escapeControl(pkt.ascii)) : pkt.bin}</span>
                    </div>
                ))}
            </div>
            <div class="flex shrink-0 items-center gap-2 border-t border-border-subtle px-3 py-1.5">
                <Button
                    class={frozen ? 'border-warning bg-warning text-surface-1 enabled:hover:border-warning enabled:hover:bg-warning' : ''}
                    onClick={handleFreeze}
                >
                    {frozen ? 'Frozen' : 'Freeze'}
                </Button>
                <Input
                    type="text"
                    class="w-36 px-1.5 py-0.5"
                    placeholder="Filter..."
                    value={filter}
                    onInput={e => setFilter((e.target as HTMLInputElement).value)}
                />
                <Button onClick={() => setPacketEntries(() => [])}>Clear</Button>
            </div>
        </div>
    );
}

import {h, Fragment} from 'preact';
import {useState, useEffect, useRef, useMemo, useCallback} from 'preact/hooks';
import {transport} from './transport';
import type {PacketLogEntry} from '@satpulse/gps/gpsio';
import {Button, Card} from './ui';

interface MsgTypeState {
    tag: string;
    msg: string;
    out: boolean;
    count: number;
    recentEntries: PacketLogEntry[];
}

interface DecodeTarget {
    key: string;
    ascii?: string;
    bin?: string;
    out: boolean;
}

interface Props {
    visible: boolean;
    connState: string;
}

const ACTIVE_WINDOW_MS = 1500;
const EPOCH_GAP_MS = 900;

function stripTrailingEOL(s: string): string {
    if (s.endsWith('\r\n')) return s.slice(0, -2);
    if (s.endsWith('\r') || s.endsWith('\n')) return s.slice(0, -1);
    return s;
}

function formatTime(iso: string): string {
    const tIdx = iso.indexOf('T');
    if (tIdx < 0) return iso;
    const time = iso.substring(tIdx + 1).replace('Z', '');
    const dot = time.indexOf('.');
    if (dot < 0) return time;
    return time.substring(0, dot + 4); // HH:MM:SS.mmm
}

function entryData(pkt: PacketLogEntry): string {
    if (pkt.ascii) return stripTrailingEOL(pkt.ascii);
    return pkt.bin || '';
}

function isActive(pkt: PacketLogEntry, now?: number): boolean {
    return (now ?? Date.now()) - new Date(pkt.t).getTime() < ACTIVE_WINDOW_MS;
}

const chevronSvg = (
    <svg class="w-3 h-3 shrink-0 transition-transform duration-150" viewBox="0 0 12 12" fill="currentColor">
        <path d="M4 2l4 4-4 4z" />
    </svg>
);

export function PacketPanel({visible, connState}: Props) {
    const liveRef = useRef<Map<string, MsgTypeState>>(new Map());
    const [displayed, setDisplayed] = useState<Map<string, MsgTypeState>>(new Map());
    const frozenRef = useRef(false);
    const frozenAtRef = useRef(0);
    const [isFrozen, setIsFrozen] = useState(false);
    const [expanded, setExpanded] = useState<Set<string>>(new Set());
    const [decodeTarget, setDecodeTarget] = useState<DecodeTarget | null>(null);
    const [decodeContent, setDecodeContent] = useState<string>('');
    const [snapshotOpen, setSnapshotOpen] = useState(false);
    const [snapshotEntries, setSnapshotEntries] = useState<(PacketLogEntry & {key: string})[]>([]);

    // Register gps:packet listener only while the tab is visible: the
    // subscription is what makes the backend stream the high-rate
    // packet events.
    useEffect(() => {
        if (!visible) return;
        const off = transport.eventsOn('gps:packet', (pkt: PacketLogEntry) => {
            const tag = pkt.tag || '';
            const msg = pkt.msg || '';
            const out = !!pkt.out;
            const key = `${tag}:${msg}:${out ? 'o' : 'i'}`;
            const live = liveRef.current;
            const existing = live.get(key);
            if (existing) {
                existing.count++;
                const pktTime = new Date(pkt.t).getTime();
                existing.recentEntries = existing.recentEntries.filter(
                    e => pktTime - new Date(e.t).getTime() < EPOCH_GAP_MS,
                );
                existing.recentEntries.push(pkt);
            } else {
                live.set(key, {tag, msg, out, count: 1, recentEntries: [pkt]});
            }
            if (!frozenRef.current) {
                setDisplayed(new Map(live));
            }
        });
        return off;
    }, [visible]);

    // Sorted rows
    const sortedRows = useMemo(() => {
        const arr = Array.from(displayed.values());
        arr.sort((a, b) => {
            if (a.out !== b.out) return a.out ? -1 : 1;
            const tc = a.tag.localeCompare(b.tag);
            if (tc !== 0) return tc;
            return a.msg.localeCompare(b.msg);
        });
        return arr;
    }, [displayed]);

    // Freeze/unfreeze
    const handleFreeze = useCallback(() => {
        const wasFrozen = frozenRef.current;
        frozenRef.current = !wasFrozen;
        setIsFrozen(!wasFrozen);
        if (wasFrozen) {
            // Resuming: sync display to live
            setDisplayed(new Map(liveRef.current));
        } else {
            // Freezing: deep-copy so in-place mutations to live don't leak through
            frozenAtRef.current = Date.now();
            const snap = new Map<string, MsgTypeState>();
            for (const [k, v] of liveRef.current) {
                snap.set(k, {...v, recentEntries: v.recentEntries.slice()});
            }
            setDisplayed(snap);
        }
    }, []);

    // Clear
    const handleClear = useCallback(() => {
        liveRef.current = new Map();
        frozenRef.current = false;
        setIsFrozen(false);
        setDisplayed(new Map());
        setExpanded(new Set());
        setDecodeTarget(null);
        setDecodeContent('');
        setSnapshotOpen(false);
    }, []);

    // Clear when leaving disconnected (new connection starting)
    const prevConnState = useRef(connState);
    useEffect(() => {
        if (prevConnState.current === 'disconnected' && connState !== 'disconnected') {
            handleClear();
        }
        prevConnState.current = connState;
    }, [connState, handleClear]);

    // Toggle expand
    const toggleExpand = useCallback((key: string, e: Event) => {
        e.stopPropagation();
        setExpanded(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    }, []);

    // Decode a single packet entry
    const decodeEntry = useCallback((key: string, entry: PacketLogEntry) => {
        const target: DecodeTarget = {
            key, ascii: entry.ascii, bin: entry.bin, out: !!entry.out,
        };
        setDecodeTarget(target);
        const out = !!entry.out;
        const raw = entry.ascii || entry.bin;
        if (!raw) return;
        const hex = !entry.ascii;
        const fallback = hex ? raw : stripTrailingEOL(raw);
        setDecodeContent('Decoding...');
        transport.decodePacket(raw, {hex, out}).then(result => {
            if (result) {
                const keys = Object.keys(result);
                const display = keys.length === 1 && keys[0] === 'payload' ? result.payload : result;
                setDecodeContent(JSON.stringify(display, null, 2));
            } else {
                setDecodeContent(fallback);
            }
        }).catch(() => {
            setDecodeContent(fallback);
        });
    }, []);

    // Row click -> decode most recent entry
    const handleRowClick = useCallback((state: MsgTypeState) => {
        const last = state.recentEntries[state.recentEntries.length - 1];
        const key = `${state.tag}:${state.msg}:${state.out ? 'o' : 'i'}`;
        decodeEntry(key, last);
    }, [decodeEntry]);

    // Snapshot
    const captureSnapshot = useCallback(() => {
        const live = liveRef.current;
        const entries: (PacketLogEntry & {key: string})[] = [];
        for (const [key, state] of live) {
            for (const e of state.recentEntries) {
                if (isActive(e)) {
                    entries.push({...e, key});
                }
            }
        }
        entries.sort((a, b) => a.t.localeCompare(b.t));
        setSnapshotEntries(entries);
    }, []);

    const handleSnapshot = useCallback(() => {
        captureSnapshot();
        setSnapshotOpen(true);
    }, [captureSnapshot]);

    // Close snapshot on Escape
    useEffect(() => {
        if (!snapshotOpen) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setSnapshotOpen(false);
        };
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [snapshotOpen]);

    return (
        <div class="flex h-full flex-col text-xs">
            {/* Table area */}
            <div class="mx-3 mt-3 flex-2 overflow-y-auto rounded border border-border-subtle bg-surface-2">
                <table class="w-full border-collapse">
                    <thead class="sticky top-0 z-10 bg-surface-2">
                        <tr class="text-left text-text-secondary">
                            <th class="w-6 px-1 py-1.5"></th>
                            <th class="whitespace-nowrap px-2 py-1.5">Protocol</th>
                            <th class="whitespace-nowrap px-2 py-1.5">Message</th>
                            <th class="whitespace-nowrap px-2 py-1.5"></th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Count</th>
                            <th class="whitespace-nowrap px-2 py-1.5">Last timestamp</th>
                            <th class="w-full px-2 py-1.5">Last message</th>
                        </tr>
                    </thead>
                    <tbody class="font-mono">
                        {sortedRows.map(state => {
                            const key = `${state.tag}:${state.msg}:${state.out ? 'o' : 'i'}`;
                            const last = state.recentEntries[state.recentEntries.length - 1];
                            const active = isActive(last, isFrozen ? frozenAtRef.current : undefined);
                            const textClass = active ? 'text-text-primary' : 'text-text-muted';
                            const canExpand = state.recentEntries.length > 1;
                            const isExpanded = expanded.has(key);
                            const selected = decodeTarget?.key === key;
                            return (
                                <Fragment key={key}>
                                    <tr
                                        class={`cursor-pointer hover:bg-surface-3 ${selected ? 'bg-surface-3' : ''}`}
                                        onClick={() => handleRowClick(state)}
                                    >
                                        <td class="align-baseline w-6 px-1 py-0.5 text-center">
                                            {canExpand && (
                                                <button
                                                    class={`inline-flex cursor-pointer border-none bg-transparent p-0 text-text-secondary ${isExpanded ? '[&>svg]:rotate-90' : ''}`}
                                                    onClick={e => toggleExpand(key, e)}
                                                >
                                                    {chevronSvg}
                                                </button>
                                            )}
                                        </td>
                                        <td class={`align-baseline whitespace-nowrap px-2 py-0.5 ${textClass}`}>{state.tag}</td>
                                        <td class={`align-baseline whitespace-nowrap px-2 py-0.5 ${textClass}`}>{state.msg}</td>
                                        <td class={`align-baseline whitespace-nowrap px-2 py-0.5 ${textClass}`}>{state.out ? 'Tx' : 'Rx'}</td>
                                        <td class={`align-baseline whitespace-nowrap px-2 py-0.5 text-right tabular-nums ${textClass}`}>{state.count}</td>
                                        <td class={`align-baseline whitespace-nowrap px-2 py-0.5 tabular-nums ${textClass}`}>{formatTime(last.t)}</td>
                                        <td class={`align-baseline px-2 py-0.5 break-all ${textClass}`}>{entryData(last)}</td>
                                    </tr>
                                    {isExpanded && state.recentEntries.map((e, i) => (
                                        <tr
                                            key={`${key}-${i}`}
                                            class="cursor-pointer text-text-secondary hover:bg-surface-3"
                                            onClick={() => decodeEntry(key, e)}
                                        >
                                            <td></td>
                                            <td></td>
                                            <td></td>
                                            <td></td>
                                            <td></td>
                                            <td class="px-2 py-0.5 tabular-nums">{formatTime(e.t)}</td>
                                            <td class="px-2 py-0.5 break-all">{entryData(e)}</td>
                                        </tr>
                                    ))}
                                </Fragment>
                            );
                        })}
                    </tbody>
                </table>
            </div>

            {/* Toolbar */}
            <div class="mx-3 flex shrink-0 items-center gap-2 py-1.5">
                <Button
                    class="w-20"
                    variant={isFrozen ? 'primary' : undefined}
                    onClick={handleFreeze}
                >
                    {isFrozen ? 'Unfreeze' : 'Freeze'}
                </Button>
                <Button class="w-20" onClick={handleSnapshot}>Snapshot</Button>
                <div class="flex-1" />
                <Button class="w-20" onClick={handleClear}>Clear</Button>
            </div>

            {/* Decode area */}
            <Card class="mx-3 mb-3 min-h-[4em] flex-1 overflow-y-auto p-2 font-mono text-text-primary">
                {decodeContent ? (
                    <pre class="m-0 whitespace-pre-wrap">{decodeContent}</pre>
                ) : (
                    <span class="text-text-muted">Click a row to decode</span>
                )}
            </Card>

            {/* Snapshot dialog */}
            {snapshotOpen && (
                <div class="fixed inset-0 z-50 flex items-center justify-center bg-surface-1/70" onClick={() => setSnapshotOpen(false)}>
                    <Card class="flex max-h-[80vh] w-225 max-w-[90vw] flex-col rounded-lg" onClick={e => e.stopPropagation()}>
                        <div class="flex-1 overflow-y-auto p-2">
                            <table class="w-full border-collapse text-xs">
                                <thead class="sticky top-0 bg-surface-2">
                                    <tr class="text-left text-text-secondary">
                                        <th class="whitespace-nowrap px-2 py-1.5">Received</th>
                                        <th class="whitespace-nowrap px-2 py-1.5">Protocol</th>
                                        <th class="whitespace-nowrap px-2 py-1.5">Message</th>
                                        <th class="whitespace-nowrap px-2 py-1.5"></th>
                                        <th class="w-full px-2 py-1.5">Data</th>
                                    </tr>
                                </thead>
                                <tbody class="font-mono">
                                    {snapshotEntries.map((e, i) => (
                                        <tr key={i} class="text-text-primary">
                                            <td class="whitespace-nowrap px-2 py-0.5 tabular-nums">{formatTime(e.t)}</td>
                                            <td class="whitespace-nowrap px-2 py-0.5">{e.tag || ''}</td>
                                            <td class="whitespace-nowrap px-2 py-0.5">{e.msg || ''}</td>
                                            <td class="whitespace-nowrap px-2 py-0.5">{e.out ? 'Tx' : 'Rx'}</td>
                                            <td class="px-2 py-0.5 break-all">{entryData(e)}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                        <div class="flex justify-end gap-2 border-t border-border-subtle px-4 py-3">
                            <Button onClick={captureSnapshot}>Refresh</Button>
                            <Button variant="primary" onClick={() => setSnapshotOpen(false)}>Close</Button>
                        </div>
                    </Card>
                </div>
            )}
        </div>
    );
}

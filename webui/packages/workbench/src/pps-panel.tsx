import {h} from 'preact';
import {useState, useEffect, useCallback, useRef, useMemo} from 'preact/hooks';
import {transport} from './transport';
import type {PPSState, PulseEdge, PortInfo} from './transport';
import type {ConnState} from './app';
import {Button, Input, Select, Badge, Card, cx, fieldLabelText, labeledControlText} from './ui';

// EdgeRec is one received edge, parsed once: the wire timestamp split
// into a whole-second epoch and a microsecond fraction (Date carries
// only milliseconds), and the derived offset from the nearest second.
interface EdgeRec {
    utc: string;      // HH:MM:SS.ffffff from the wire string
    epochMs: number;  // epoch of the whole second, in ms
    micros: number;   // fractional second in microseconds [0, 1e6)
    offsetUs: number; // signed offset from the nearest second, µs
    uncertainty: number; // seconds; 0 when none
    settling: boolean;
}

const EDGE_CAP = 4096;
const TABLE_ROWS = 200;

function parseEdge(e: PulseEdge): EdgeRec | null {
    const m = /^(.*)T(\d\d:\d\d:\d\d)\.(\d+)Z$/.exec(e.t);
    if (!m) return null;
    const epochMs = Date.parse(`${m[1]}T${m[2]}Z`);
    if (isNaN(epochMs)) return null;
    const micros = parseInt(m[3].padEnd(6, '0').slice(0, 6), 10);
    return {
        utc: `${m[2]}.${m[3].padEnd(6, '0').slice(0, 6)}`,
        epochMs,
        micros,
        offsetUs: micros < 500000 ? micros : micros - 1000000,
        uncertainty: e.uncertainty ?? 0,
        settling: e.settling ?? false,
    };
}

function localTime(rec: EdgeRec): string {
    const d = new Date(rec.epochMs);
    const p = (n: number, w = 2) => String(n).padStart(w, '0');
    return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(rec.micros, 6)}`;
}

// fmtUs renders a microsecond quantity as a plain number; the unit is
// always microseconds and appears once in the surrounding label.
function fmtUs(us: number): string {
    return String(Math.round(us));
}

function fmtSeconds(s: number): string {
    return s === 0 ? '' : fmtUs(s * 1e6);
}


interface Props {
    connState: ConnState;
    readOnly: boolean;
    device: string;
    ports: PortInfo[];
}

export function PPSPanel({connState, readOnly, device, ports}: Props) {
    // Until the user picks a pin, the default follows the selected device:
    // CTS on a USB serial adapter, DCD on a native port.
    const [pinChoice, setPinChoice] = useState('');
    const [method, setMethod] = useState('');
    const [preWarm, setPreWarm] = useState('');
    const [invert, setInvert] = useState(false);
    const port = ports.find(p => p.device === device || p.aliases?.includes(device));
    const pin = pinChoice || (port?.usb ? 'cts' : 'dcd');
    const [ppsState, setPPSState] = useState<PPSState | null>(null);
    const [startError, setStartError] = useState('');
    const [pending, setPending] = useState<'start' | 'stop' | null>(null);
    const [edges, setEdges] = useState<EdgeRec[]>([]);
    const pendingRef = useRef<'start' | 'stop' | null>(null);
    const stateSeqRef = useRef(0);
    const tableRef = useRef<HTMLDivElement>(null);
    const tableAutoScroll = useRef(true);
    const setPendingSync = useCallback((p: 'start' | 'stop' | null) => {
        pendingRef.current = p;
        setPending(p);
    }, []);

    const connected = connState !== 'disconnected';


    const applyState = useCallback((st: PPSState) => {
        setPPSState(st);
        if (st.state === 'stopped' || st.state === 'failed') {
            if (pendingRef.current !== null) setPendingSync(null);
        } else if (pendingRef.current === 'start') {
            setPendingSync(null);
        }
    }, [setPendingSync]);

    useEffect(() => {
        const offState = transport.eventsOn('gps:pps', (st: PPSState) => {
            stateSeqRef.current++;
            applyState(st);
        });
        const offEdge = transport.eventsOn('gps:pulseEdge', (e: PulseEdge) => {
            const rec = parseEdge(e);
            if (!rec) return;
            setEdges(prev => {
                const next = prev.length >= EDGE_CAP ? prev.slice(prev.length - EDGE_CAP + 1) : prev.slice();
                next.push(rec);
                return next;
            });
        });
        return () => { offState(); offEdge(); };
    }, [applyState]);

    useEffect(() => {
        const seq = stateSeqRef.current;
        let cancelled = false;
        transport.pps!.getPPSState().then(st => {
            if (cancelled || stateSeqRef.current !== seq) return;
            applyState(st);
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [applyState]);

    const sim = ppsState?.sim ?? false;
    const synced = ppsState !== null;
    const running = ppsState?.state === 'running';
    const preWarmNum = preWarm.trim() === '' ? 0 : Number(preWarm);
    const preWarmOk = !isNaN(preWarmNum) && preWarmNum >= 0 && preWarmNum < 1;
    const available = connected || sim;
    const locked = running || pending !== null;
    const fieldsDisabled = !available || locked || readOnly;

    const handleToggle = useCallback(async () => {
        if (pendingRef.current !== null) return;
        if (running) {
            setPendingSync('stop');
            try {
                await transport.pps!.stopPPS();
            } catch (e) {
                setPendingSync(null);
                if (e instanceof Error && e.message) setStartError(e.message);
            }
        } else {
            if (!preWarmOk) return;
            setPendingSync('start');
            setStartError('');
            try {
                await transport.pps!.startPPS({
                    pin,
                    invertPolarity: invert,
                    method,
                    pollPreWarm: preWarmNum,
                });
            } catch (e) {
                setPendingSync(null);
                if (e instanceof Error && e.message) setStartError(e.message);
            }
        }
    }, [running, pin, invert, method, preWarmNum, preWarmOk, setPendingSync]);

    const handleClear = useCallback(() => setEdges([]), []);

    const handleTableScroll = useCallback(() => {
        const el = tableRef.current;
        if (!el) return;
        tableAutoScroll.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    }, []);

    useEffect(() => {
        const el = tableRef.current;
        if (el && tableAutoScroll.current) {
            el.scrollTop = el.scrollHeight;
        }
    }, [edges]);

    let dotClass = 'bg-text-muted';
    if (ppsState?.state === 'running') dotClass = 'bg-success';
    else if (ppsState?.state === 'failed') dotClass = 'bg-danger';

    let statusText = '';
    let statusClass = 'text-text-muted';
    if (ppsState?.state === 'failed') {
        statusText = `Failed: ${ppsState.error || 'detection failed'}`;
        statusClass = 'text-danger';
    } else if (startError) {
        statusText = startError;
        statusClass = 'text-danger';
    } else if (!available && synced) {
        statusText = 'Connect a receiver over a serial port to detect PPS';
        statusClass = 'text-text-muted';
    }

    // Offset statistics over settled edges only, following the PHC stats
    // model (statsobs, clocklog.py): settling edges are excluded the way
    // out-of-sync PHC samples are, and counted separately. Unlike PHC,
    // the bias is not typically zero, so the mean is reported and the
    // scatter is measured about it.
    const stats = useMemo(() => {
        const offs: number[] = [];
        let settling = 0;
        for (const e of edges) {
            if (e.settling) settling++;
            else offs.push(e.offsetUs);
        }
        const n = offs.length;
        if (n === 0) return {n: 0, settling, mean: 0, sd: 0, maxAbs: 0, p95: 0};
        const mean = offs.reduce((a, b) => a + b, 0) / n;
        const sd = Math.sqrt(offs.reduce((a, b) => a + (b - mean) * (b - mean), 0) / n);
        const abs = offs.map(Math.abs).sort((a, b) => a - b);
        return {n, settling, mean, sd, maxAbs: abs[n - 1], p95: abs[Math.min(Math.floor(n * 0.95), n - 1)]};
    }, [edges]);

    return (
        <div class="flex h-full flex-col">
            {/* Config row */}
            <div class="flex shrink-0 flex-wrap items-center gap-3 px-4 pt-4 pb-2">
                <span class={fieldLabelText(!available)}>Pin:</span>
                <Select
                    class="w-24"
                    value={pin}
                    onChange={e => setPinChoice((e.target as HTMLSelectElement).value)}
                    disabled={fieldsDisabled}
                >
                    <option value="cts">CTS (8)</option>
                    <option value="dcd">DCD (1)</option>
                    <option value="dsr">DSR (6)</option>
                    <option value="ri">RI (9)</option>
                </Select>
                <span class={fieldLabelText(!available)}>Method:</span>
                <Select
                    class="w-28"
                    value={running && ppsState?.method ? ppsState.method : method}
                    onChange={e => setMethod((e.target as HTMLSelectElement).value)}
                    disabled={fieldsDisabled}
                >
                    <option value="">Automatic</option>
                    <option value="poll">Poll</option>
                    <option value="wait">Wait</option>
                    <option value="kernel">Kernel</option>
                    {running && ppsState?.method === 'replay' && <option value="replay">Replay</option>}
                </Select>
                <span class={fieldLabelText(!available || (method !== '' && method !== 'poll'))}>Poll pre-warm (s):</span>
                <Input
                    class="w-20"
                    inputMode="decimal"
                    value={preWarm}
                    invalid={!preWarmOk}
                    onInput={e => setPreWarm((e.target as HTMLInputElement).value)}
                    disabled={fieldsDisabled || (method !== '' && method !== 'poll')}
                    placeholder="0"
                />
                <label class={cx('flex items-center gap-1.5', labeledControlText(fieldsDisabled))}>
                    <input
                        type="checkbox"
                        class="accent-accent"
                        checked={invert}
                        disabled={fieldsDisabled}
                        onChange={e => setInvert((e.target as HTMLInputElement).checked)}
                    />
                    Invert polarity
                </label>
                <div class={`h-2.5 w-2.5 shrink-0 rounded-full ${dotClass}`} />
                <span class={cx('ml-auto text-xs', statusClass)}>{statusText}</span>
                <Button
                    variant={running ? 'secondary' : 'primary'}
                    disabled={!synced || pending !== null || !available || (!running && !preWarmOk) || readOnly}
                    onClick={handleToggle}
                >
                    {running ? 'Stop' : 'Start'}
                </Button>
            </div>

            {/* Offset graph */}
            <div class="shrink-0 px-4 pb-2">
                <Card class="p-2">
                    <PPSGraph edges={edges} />
                </Card>
            </div>

            {/* Edge table with statistics beside it */}
            <div class="flex min-h-0 flex-1 items-stretch gap-4 px-4 pb-4 text-xs">
                <div
                    ref={tableRef}
                    onScroll={handleTableScroll}
                    class="h-full max-w-full overflow-y-auto rounded border border-border-subtle bg-surface-2"
                >
                    <table class="border-collapse">
                        <thead class="sticky top-0 z-10 bg-surface-2">
                            <tr class="text-left text-text-secondary">
                                <th class="whitespace-nowrap px-2 py-1.5">UTC</th>
                                <th class="whitespace-nowrap px-2 py-1.5">Local</th>
                                <th class="whitespace-nowrap px-2 py-1.5 text-right">Offset (µs)</th>
                                <th class="whitespace-nowrap px-2 py-1.5 text-right">Uncertainty (µs)</th>
                                <th class="w-20 px-2 py-1.5"></th>
                            </tr>
                        </thead>
                        <tbody class="font-mono">
                            {edges.slice(-TABLE_ROWS).map(e => (
                                <tr key={e.epochMs * 1000 + (e.micros % 1000)}>
                                    <td class="whitespace-nowrap px-2 py-0.5">{e.utc}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5">{localTime(e)}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{fmtUs(e.offsetUs)}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{fmtSeconds(e.uncertainty)}</td>
                                    <td class="px-2 py-0.5">{e.settling && <Badge tone="warning">settling</Badge>}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                    {edges.length === 0 && (
                        <div class="px-3 py-6 text-center text-text-muted">No edges received</div>
                    )}
                </div>
                <div class="flex min-w-0 flex-1 items-start justify-center">
                    <div class="flex flex-col gap-8">
                        <div>
                            <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-text-secondary">Offset <span class="normal-case">(µs)</span></h4>
                            <table class="border-separate" style="border-spacing: 0 2px;">
                                <tbody>
                                    <StatRow label="Mean" value={stats.n ? stats.mean.toFixed(1) : ''} />
                                    <StatRow label="Std dev" value={stats.n ? stats.sd.toFixed(1) : ''} />
                                    <StatRow label="Max |offset|" value={stats.n ? fmtUs(stats.maxAbs) : ''} />
                                    <StatRow label="95% |offset|" value={stats.n ? fmtUs(stats.p95) : ''} />
                                </tbody>
                            </table>
                        </div>
                        <div>
                            <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-text-secondary">Edges</h4>
                            <table class="border-separate" style="border-spacing: 0 2px;">
                                <tbody>
                                    <StatRow label="Settled" value={String(stats.n)} />
                                    <StatRow label="Settling" value={String(stats.settling)} />
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
                <div class="flex shrink-0 flex-col justify-end">
                    <Button disabled={edges.length === 0} onClick={handleClear}>Clear</Button>
                </div>
            </div>
        </div>
    );
}

function StatRow({label, value}: {label: string; value: string}) {
    return (
        <tr>
            <td class="text-text-secondary pr-4">{label}</td>
            <td class="font-mono text-right">{value}</td>
        </tr>
    );
}

// PPSGraph plots the offset from the nearest second against wall time.
const GH = 200;
const ML = 48;  // left margin for y labels
const MR = 10;
const MT = 16;  // top margin; leaves room for the axis unit caption
const MB = 22;  // bottom margin for x labels
const XSPAN_MIN = 60_000; // ms; edges accumulate from the left until the window fills

// niceStep returns a 1/2/5 * 10^k step giving at most n intervals over span.
function niceStep(span: number, n: number): number {
    const raw = span / n;
    const mag = Math.pow(10, Math.floor(Math.log10(raw)));
    for (const m of [1, 2, 5]) {
        if (raw <= m * mag) return m * mag;
    }
    return 10 * mag;
}

function PPSGraph({edges}: {edges: EdgeRec[]}) {
    const containerRef = useRef<HTMLDivElement>(null);
    const [gw, setGW] = useState(800);
    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;
        const update = () => {
            const w = el.getBoundingClientRect().width;
            if (w > 0) setGW(w);
        };
        update();
        const ro = new ResizeObserver(update);
        ro.observe(el);
        return () => ro.disconnect();
    }, []);
    if (edges.length === 0) {
        return (
            <div ref={containerRef} class="flex items-center justify-center text-xs text-text-muted" style={{height: GH + 'px'}}>
                No edges received
            </div>
        );
    }
    const GW = gw;
    const xs = edges.map(e => e.epochMs + e.micros / 1000);
    const ys = edges.map(e => e.offsetUs);
    const x0 = xs[0], x1 = Math.max(xs[xs.length - 1], x0 + XSPAN_MIN);
    let y0 = Math.min(...ys), y1 = Math.max(...ys);
    const pad = Math.max((y1 - y0) * 0.1, 5);
    y0 -= pad;
    y1 += pad;
    const px = (x: number) => ML + ((x - x0) / (x1 - x0)) * (GW - ML - MR);
    const py = (y: number) => MT + (1 - (y - y0) / (y1 - y0)) * (GH - MT - MB);
    const step = niceStep(y1 - y0, 4);
    const yLines = [];
    for (let v = Math.ceil(y0 / step) * step; v <= y1; v += step) {
        yLines.push(
            <g key={'y' + v}>
                <line x1={ML} y1={py(v)} x2={GW - MR} y2={py(v)} class="stroke-border-subtle" stroke-width="1" />
                <text x={ML - 6} y={py(v) + 3} text-anchor="end" class="fill-text-secondary" font-size="10">
                    {fmtUs(v)}
                </text>
            </g>,
        );
    }
    const xLabels = [];
    const xTicks = 4;
    for (let i = 0; i <= xTicks; i++) {
        const v = x0 + ((x1 - x0) * i) / xTicks;
        const d = new Date(v);
        const p = (n: number) => String(n).padStart(2, '0');
        xLabels.push(
            <text
                key={'x' + i}
                x={px(v)}
                y={GH - 6}
                text-anchor={i === 0 ? 'start' : i === xTicks ? 'end' : 'middle'}
                class="fill-text-secondary"
                font-size="10"
            >
                {`${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`}
            </text>,
        );
    }
    return (
        <div ref={containerRef}>
        <svg width={GW} height={GH} viewBox={`0 0 ${GW} ${GH}`}>
            <text x={ML - 6} y={10} text-anchor="end" class="fill-text-secondary" font-size="10">µs</text>
            {yLines}
            {y0 < 0 && y1 > 0 && (
                <line x1={ML} y1={py(0)} x2={GW - MR} y2={py(0)} class="stroke-text-muted" stroke-width="1" stroke-dasharray="4 3" />
            )}
            {xLabels}
            {edges.map((e, i) => (
                e.settling
                    ? <circle key={i} cx={px(xs[i])} cy={py(ys[i])} r="2.5" class="fill-none stroke-accent" stroke-width="1.2" />
                    : <circle key={i} cx={px(xs[i])} cy={py(ys[i])} r="2.5" class="fill-accent" />
            ))}
        </svg>
        </div>
    );
}

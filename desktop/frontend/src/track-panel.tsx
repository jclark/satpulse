import {h} from 'preact';
import {useState, useCallback, useRef} from 'preact/hooks';
import {Button} from './ui';

interface TrackPanelProps {
    pos: {lat: number; lon: number} | null;
}

const VB = 500; // viewBox units (square)
const INITIAL_SPAN = 0.05; // 5 cm in meters
const MARGIN_FRAC = 0.15;
const MAX_POINTS = 1_000_000;

const SCALE_STEPS = [
    0.01, 0.02, 0.05, 0.1, 0.2, 0.5,
    1, 2, 5, 10, 20, 50, 100, 200, 500, 1000,
];

function pickScale(spanMeters: number): {lengthVB: number; label: string; step: number} {
    const target = spanMeters * 0.25;
    let step = SCALE_STEPS[0];
    for (const s of SCALE_STEPS) {
        step = s;
        if (s >= target) break;
    }
    const lengthVB = (step / spanMeters) * VB;
    let label: string;
    if (step < 1) label = (step * 100).toFixed(0) + ' cm';
    else if (step < 1000) label = step.toFixed(0) + ' m';
    else label = (step / 1000).toFixed(0) + ' km';
    return {lengthVB, label, step};
}

export function TrackPanel({pos}: TrackPanelProps) {
    const firstRef = useRef<{lat: number; lon: number} | null>(null);
    const cosLat = useRef(1);
    const bufRef = useRef(new Float64Array(2048));
    const countRef = useRef(0);
    const [, setTick] = useState(0);
    const spanRef = useRef(INITIAL_SPAN);
    const centerRef = useRef({e: 0, n: 0});

    const addPoint = useCallback((lat: number, lon: number) => {
        if (!firstRef.current) {
            firstRef.current = {lat, lon};
            cosLat.current = Math.cos(lat * Math.PI / 180);
        }
        const first = firstRef.current;
        const east = (lon - first.lon) * 111320 * cosLat.current;
        const north = (lat - first.lat) * 110540;
        let buf = bufRef.current;
        let n = countRef.current;
        if (n >= MAX_POINTS) {
            buf.copyWithin(0, 2);
            n = MAX_POINTS - 1;
        }
        if (n * 2 + 2 > buf.length) {
            const next = new Float64Array(buf.length * 2);
            next.set(buf);
            bufRef.current = next;
            buf = next;
        }
        buf[n * 2] = east;
        buf[n * 2 + 1] = north;
        countRef.current = n + 1;
        // When a point falls near the edge, recenter and zoom out if needed
        const span = spanRef.current;
        const half = span / 2;
        const margin = span * MARGIN_FRAC;
        const ce = centerRef.current.e, cn = centerRef.current.n;
        if (Math.abs(east - ce) > half - margin || Math.abs(north - cn) > half - margin) {
            let minE = Infinity, maxE = -Infinity, minN = Infinity, maxN = -Infinity;
            for (let i = 0; i < (n + 1) * 2; i += 2) {
                const e = buf[i], nn = buf[i + 1];
                if (e < minE) minE = e;
                if (e > maxE) maxE = e;
                if (nn < minN) minN = nn;
                if (nn > maxN) maxN = nn;
            }
            centerRef.current = {e: (minE + maxE) / 2, n: (minN + maxN) / 2};
            const newSpan = Math.max(Math.max(maxE - minE, maxN - minN) * 3, INITIAL_SPAN);
            if (newSpan > spanRef.current) spanRef.current = newSpan;
        }
        setTick(t => t + 1);
    }, []);

    const lastPosRef = useRef<{lat: number; lon: number} | null>(null);
    if (pos && (pos !== lastPosRef.current)) {
        lastPosRef.current = pos;
        addPoint(pos.lat, pos.lon);
    }

    const handleClear = useCallback(() => {
        firstRef.current = null;
        countRef.current = 0;
        spanRef.current = INITIAL_SPAN;
        centerRef.current = {e: 0, n: 0};
        lastPosRef.current = null;
        setTick(t => t + 1);
    }, []);

    const n = countRef.current;
    const span = spanRef.current;
    const buf = bufRef.current;
    const center = centerRef.current;

    const scale = pickScale(span);
    const dotOpacity = Math.max(0.05, Math.min(1, 5 / Math.sqrt(n || 1)));
    const half = span / 2;

    const scalePx = (scale.lengthVB / VB) * 500;

    return (
        <div class="flex items-start gap-3">
            <div class="rounded border border-border-subtle bg-surface-2" style="width: 500px; height: 500px;">
                {n === 0 ? (
                    <div class="flex h-full w-full items-center justify-center text-sm text-text-muted">
                        Waiting for position
                    </div>
                ) : (
                    <svg
                        viewBox={`0 0 ${VB} ${VB}`}
                        width="500"
                        height="500"
                        xmlns="http://www.w3.org/2000/svg"
                    >
                        {Array.from({length: n}, (_, i) => {
                            const e = buf[i * 2];
                            const nn = buf[i * 2 + 1];
                            const px = ((e - center.e) / half) * (VB / 2) + VB / 2;
                            const py = VB / 2 - ((nn - center.n) / half) * (VB / 2);
                            const last = i === n - 1;
                            return (
                                <circle
                                    key={i}
                                    cx={px}
                                    cy={py}
                                    r={last ? 2.5 : 1.5}
                                    fill={last ? 'var(--accent)' : 'var(--track-dot)'}
                                    opacity={last ? 1 : dotOpacity}
                                />
                            );
                        })}
                    </svg>
                )}
            </div>
            {n > 0 && (
                <div class="flex flex-col justify-end gap-2" style="height: 500px;">
                        {(() => {
                            // Subdivide by leading digit: 1->1, 2->2, 5->5
                            const log = Math.log10(scale.step);
                            const frac = log - Math.floor(log);
                            const leading = Math.round(Math.pow(10, frac));
                            const nTicks = leading + 1;
                            const tickH = 5;
                            const svgW = scalePx + 2;
                            const svgH = tickH + 4 + 14;
                            return (
                                <svg width={svgW} height={svgH} xmlns="http://www.w3.org/2000/svg">
                                    <line x1="1" y1={tickH} x2={1 + scalePx} y2={tickH} stroke="var(--border-strong)" stroke-width="1.5" />
                                    {Array.from({length: nTicks}, (_, i) => {
                                        const x = 1 + (i / (nTicks - 1)) * scalePx;
                                        return <line key={i} x1={x} y1={0} x2={x} y2={tickH} stroke="var(--border-strong)" stroke-width="1.5" />;
                                    })}
                                    <text x={(2 + scalePx) / 2} y={svgH - 1} text-anchor="middle" fill="var(--text-secondary)" font-size="11">
                                        {scale.label}
                                    </text>
                                </svg>
                            );
                        })()}
                        <Button class="self-start" onClick={handleClear}>Clear</Button>
                </div>
            )}
        </div>
    );
}

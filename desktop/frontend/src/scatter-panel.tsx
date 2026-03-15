import {h} from 'preact';
import {useState, useCallback, useRef, useEffect} from 'preact/hooks';
import {ECEFtoLLH} from '../wailsjs/go/main/App';
import {Button} from './ui';

interface ScatterPanelProps {
    ecef: [number, number, number] | null;
}

const VB = 500;
const INITIAL_SPAN = 0.05; // 5 cm
const MARGIN_FRAC = 0.15;
const MAX_POINTS = 1_000_000;

const SCALE_STEPS = [
    0.01, 0.02, 0.05, 0.1, 0.2, 0.5,
    1, 2, 5, 10, 20, 50, 100, 200, 500, 1000,
];

function pickRingStep(halfSpan: number): number {
    for (const s of SCALE_STEPS) {
        if (halfSpan / s <= 4) return s;
    }
    return SCALE_STEPS[SCALE_STEPS.length - 1];
}

function formatDist(m: number): string {
    if (m < 0.01) return (m * 1000).toFixed(1) + ' mm';
    if (m < 1) {
        const cm = m * 100;
        return (Number.isInteger(cm) ? cm.toFixed(0) : cm.toFixed(1)) + ' cm';
    }
    if (m < 1000) {
        return (Number.isInteger(m) ? m.toFixed(0) : m.toFixed(1)) + ' m';
    }
    return (m / 1000).toFixed(1) + ' km';
}

function formatPrecision(m: number): string {
    if (m < 1) return (m * 1000).toFixed(1) + ' mm';
    return m.toFixed(3) + ' m';
}

function formatECEF(m: number): string {
    return m.toFixed(4) + ' m';
}

export function ScatterPanel({ecef}: ScatterPanelProps) {
    const bufRef = useRef(new Float64Array(3072)); // interleaved X,Y,Z
    const countRef = useRef(0);
    const meanRef = useRef({x: 0, y: 0, z: 0});
    const sinCosRef = useRef<{sinLat: number; cosLat: number; sinLon: number; cosLon: number} | null>(null);
    const spanRef = useRef(INITIAL_SPAN);
    const centerRef = useRef({e: 0, n: 0});
    const [, setTick] = useState(0);
    const lastEcefRef = useRef<[number, number, number] | null>(null);
    const pendingLLHRef = useRef(false);

    // Compute ENU offsets for all points from current mean using current sinCos
    const computeENU = useCallback(() => {
        const sc = sinCosRef.current;
        if (!sc) return null;
        const n = countRef.current;
        const buf = bufRef.current;
        const mean = meanRef.current;
        const enu = new Float64Array(n * 3);
        for (let i = 0; i < n; i++) {
            const dx = buf[i * 3] - mean.x;
            const dy = buf[i * 3 + 1] - mean.y;
            const dz = buf[i * 3 + 2] - mean.z;
            enu[i * 3] = -sc.sinLon * dx + sc.cosLon * dy;
            enu[i * 3 + 1] = -sc.sinLat * sc.cosLon * dx - sc.sinLat * sc.sinLon * dy + sc.cosLat * dz;
            enu[i * 3 + 2] = sc.cosLat * sc.cosLon * dx + sc.cosLat * sc.sinLon * dy + sc.sinLat * dz;
        }
        return enu;
    }, []);

    const updateView = useCallback((enu: Float64Array, n: number) => {
        if (n === 0) return;
        const lastE = enu[(n - 1) * 3];
        const lastN = enu[(n - 1) * 3 + 1];
        const span = spanRef.current;
        const half = span / 2;
        const margin = span * MARGIN_FRAC;
        const ce = centerRef.current.e, cn = centerRef.current.n;
        if (Math.abs(lastE - ce) > half - margin || Math.abs(lastN - cn) > half - margin) {
            let minE = Infinity, maxE = -Infinity, minN = Infinity, maxN = -Infinity;
            for (let i = 0; i < n; i++) {
                const e = enu[i * 3], nn = enu[i * 3 + 1];
                if (e < minE) minE = e;
                if (e > maxE) maxE = e;
                if (nn < minN) minN = nn;
                if (nn > maxN) maxN = nn;
            }
            centerRef.current = {e: (minE + maxE) / 2, n: (minN + maxN) / 2};
            const newSpan = Math.max(Math.max(maxE - minE, maxN - minN) * 1.5, INITIAL_SPAN);
            if (newSpan > spanRef.current) spanRef.current = newSpan;
        }
    }, []);

    const addPoint = useCallback((x: number, y: number, z: number) => {
        let buf = bufRef.current;
        let n = countRef.current;
        // Buffer full: discard oldest
        if (n >= MAX_POINTS) {
            buf.copyWithin(0, 3);
            // Recompute mean from remaining points
            n = MAX_POINTS - 1;
            let sx = 0, sy = 0, sz = 0;
            for (let i = 0; i < n * 3; i += 3) {
                sx += buf[i]; sy += buf[i + 1]; sz += buf[i + 2];
            }
            meanRef.current = {x: sx / n, y: sy / n, z: sz / n};
        }
        // Grow buffer if needed
        if (n * 3 + 3 > buf.length) {
            const next = new Float64Array(buf.length * 2);
            next.set(buf);
            bufRef.current = next;
            buf = next;
        }
        buf[n * 3] = x;
        buf[n * 3 + 1] = y;
        buf[n * 3 + 2] = z;
        // Incremental mean update
        const newN = n + 1;
        const mean = meanRef.current;
        meanRef.current = {
            x: mean.x + (x - mean.x) / newN,
            y: mean.y + (y - mean.y) / newN,
            z: mean.z + (z - mean.z) / newN,
        };
        countRef.current = newN;
        // Request LLH for the new mean (async)
        if (!pendingLLHRef.current) {
            pendingLLHRef.current = true;
            const m = meanRef.current;
            ECEFtoLLH(m.x, m.y, m.z).then(llh => {
                const latRad = llh.lat * Math.PI / 180;
                const lonRad = llh.lon * Math.PI / 180;
                sinCosRef.current = {
                    sinLat: Math.sin(latRad),
                    cosLat: Math.cos(latRad),
                    sinLon: Math.sin(lonRad),
                    cosLon: Math.cos(lonRad),
                };
                pendingLLHRef.current = false;
                // Now recompute ENU with updated rotation
                const enu = computeENU();
                if (enu) updateView(enu, countRef.current);
                setTick(t => t + 1);
            }).catch(() => {
                pendingLLHRef.current = false;
            });
        }
        // If we already have sinCos, compute ENU and update view immediately
        const enu = computeENU();
        if (enu) updateView(enu, newN);
        setTick(t => t + 1);
    }, [computeENU, updateView]);

    // Process new ECEF prop
    if (ecef && ecef !== lastEcefRef.current) {
        lastEcefRef.current = ecef;
        addPoint(ecef[0], ecef[1], ecef[2]);
    }

    const handleClear = useCallback(() => {
        countRef.current = 0;
        meanRef.current = {x: 0, y: 0, z: 0};
        sinCosRef.current = null;
        spanRef.current = INITIAL_SPAN;
        centerRef.current = {e: 0, n: 0};
        lastEcefRef.current = null;
        pendingLLHRef.current = false;
        setTick(t => t + 1);
    }, []);

    const n = countRef.current;
    const span = spanRef.current;
    const center = centerRef.current;
    const mean = meanRef.current;
    const half = span / 2;
    const dotOpacity = Math.max(0.05, Math.min(1, 5 / Math.sqrt(n || 1)));

    // Compute ENU for rendering and statistics
    const enu = computeENU();

    // Compute statistics
    let cep50 = 0, cep95 = 0, rmsH = 0, rmsV = 0, rms3D = 0;
    if (enu && n > 0) {
        const hDists = new Float64Array(n);
        let sumH2 = 0, sumV2 = 0, sumD2 = 0;
        for (let i = 0; i < n; i++) {
            const e = enu[i * 3], nn2 = enu[i * 3 + 1], u = enu[i * 3 + 2];
            const h2 = e * e + nn2 * nn2;
            const d2 = h2 + u * u;
            hDists[i] = Math.sqrt(h2);
            sumH2 += h2;
            sumV2 += u * u;
            sumD2 += d2;
        }
        rmsH = Math.sqrt(sumH2 / n);
        rmsV = Math.sqrt(sumV2 / n);
        rms3D = Math.sqrt(sumD2 / n);
        // Sort for CEP
        hDists.sort();
        cep50 = hDists[Math.floor(n * 0.5)];
        cep95 = hDists[Math.min(Math.floor(n * 0.95), n - 1)];
    }

    // Distance rings
    const ringStep = pickRingStep(half);
    const rings: number[] = [];
    for (let r = ringStep; r <= half; r += ringStep) {
        rings.push(r);
    }
    // Label the outermost ring that leaves room for text (ratio < 0.85)
    let labelRing = 0;
    for (let i = rings.length - 1; i >= 0; i--) {
        if (rings[i] / half < 0.85) { labelRing = rings[i]; break; }
    }

    return (
        <div class="flex items-stretch" style="min-height: 300px;">
            <div class="rounded border border-border-subtle bg-surface-2 shrink-0" style="width: 300px; height: 300px;">
                {n === 0 || !enu ? (
                    <div class="flex h-full w-full items-center justify-center text-sm text-text-muted">
                        Waiting for position
                    </div>
                ) : (
                    <svg
                        viewBox={`0 0 ${VB} ${VB}`}
                        width="300"
                        height="300"
                        xmlns="http://www.w3.org/2000/svg"
                    >
                        {/* Crosshair */}
                        <line x1={0} y1={VB / 2} x2={VB} y2={VB / 2} stroke="var(--border-subtle)" stroke-width="0.5" />
                        <line x1={VB / 2} y1={0} x2={VB / 2} y2={VB} stroke="var(--border-subtle)" stroke-width="0.5" />
                        {/* Distance rings */}
                        {rings.map(r => {
                            const rVB = (r / half) * (VB / 2);
                            return (
                                <circle
                                    key={r}
                                    cx={VB / 2}
                                    cy={VB / 2}
                                    r={rVB}
                                    fill="none"
                                    stroke="var(--border-subtle)"
                                    stroke-width="0.5"
                                />
                            );
                        })}
                        {/* Ring label */}
                        {labelRing > 0 && (
                            <text
                                x={VB / 2 + (labelRing / half) * (VB / 2)}
                                y={VB / 2 - 4}
                                text-anchor="middle"
                                fill="var(--text-secondary)"
                                font-size="16"
                            >
                                {formatDist(labelRing)}
                            </text>
                        )}
                        {/* Position dots */}
                        {Array.from({length: n}, (_, i) => {
                            const e = enu[i * 3];
                            const nn2 = enu[i * 3 + 1];
                            const px = ((e - center.e) / half) * (VB / 2) + VB / 2;
                            const py = VB / 2 - ((nn2 - center.n) / half) * (VB / 2);
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
                <div class="flex min-w-0 flex-1 items-start justify-center text-xs" style="height: 300px;">
                    <div class="flex flex-col justify-between" style="height: 300px;">
                        <div>
                            <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-text-secondary">Precision</h4>
                            <table class="border-separate" style="border-spacing: 0 2px;">
                                <tbody>
                                    <StatRow label="50% CEP" value={formatPrecision(cep50)} />
                                    <StatRow label="95% CEP" value={formatPrecision(cep95)} />
                                    <StatRow label="RMS Horiz" value={formatPrecision(rmsH)} />
                                    <StatRow label="RMS Vert" value={formatPrecision(rmsV)} />
                                    <StatRow label="RMS 3D" value={formatPrecision(rms3D)} />
                                </tbody>
                            </table>
                        </div>
                        <div>
                            <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-text-secondary">Points</h4>
                            <span class="font-mono">{n}</span>
                        </div>
                        <div>
                            <h4 class="mb-1 text-xs font-semibold uppercase tracking-wider text-text-secondary">Mean ECEF</h4>
                            <table class="border-separate" style="border-spacing: 0 2px;">
                                <tbody>
                                    <StatRow label="X" value={formatECEF(mean.x)} />
                                    <StatRow label="Y" value={formatECEF(mean.y)} />
                                    <StatRow label="Z" value={formatECEF(mean.z)} />
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
            )}
            {n > 0 && (
                <div class="flex flex-col justify-end shrink-0" style="height: 300px;">
                    <Button onClick={handleClear}>Clear</Button>
                </div>
            )}
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

import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import type {SatellitesMsg, SVInfo, SignalInfo} from './app';

interface Props {
    msg: SatellitesMsg | null;
}

const SIZE = 200;
const RADIUS = SIZE / 2;
const STROKE_PAD = 1;
const MIN_ELEVATION = -3;
const COMPASS_PADDING = 2;
const CAP_X_HEIGHT_DIFF = 0.5;
const DOT_RADIUS = 2.0;
const DOT_INNER_RADIUS = 0.9;

function toXY(az: number, el: number): [number, number] {
    const r = ((90 - Math.max(el, MIN_ELEVATION)) / 90) * RADIUS;
    const rad = (az - 90) * (Math.PI / 180);
    return [RADIUS + r * Math.cos(rad), RADIUS + r * Math.sin(rad)];
}

// simplifySignals collapses each satellite's signals to a single
// representative CN0 for display. A satellite with no signal-strength info
// keeps an empty signals array, so callers can tell "no strength reported"
// apart from a reported strength of zero.
function simplifySignals(satellites: SVInfo[]): SVInfo[] {
    return satellites.map(sv => ({
        ...sv,
        signals: sv.signals && sv.signals.length > 0 ? [{cn0: signalsCN0(sv.signals)}] : [],
    }));
}

function signalsCN0(signals: SignalInfo[]): number {
    if (!signals) return 0;
    const anon = signals.find(s => s.id === '' || s.id === undefined);
    if (anon && anon.cn0 > 0) return anon.cn0;
    return signals.reduce((max, s) => Math.max(max, s.cn0), 0);
}

function opacityClassFor(cn0: number): string {
    if (cn0 < 25) return 'opacity-20';
    if (cn0 < 30) return 'opacity-40';
    if (cn0 < 35) return 'opacity-60';
    if (cn0 < 42) return 'opacity-75';
    if (cn0 < 50) return 'opacity-90';
    return 'opacity-100';
}

type GnssKey = 'gps' | 'galileo' | 'beidou' | 'glonass' | 'qzss' | 'navic' | 'unknown';

const GNSS_LABEL: Record<GnssKey, string> = {
    gps: 'GPS',
    galileo: 'GAL',
    beidou: 'BDS',
    glonass: 'GLO',
    qzss: 'QZSS',
    navic: 'NAVIC',
    unknown: '?',
};

const GNSS_ORDER: GnssKey[] = ['gps', 'galileo', 'beidou', 'glonass', 'qzss', 'navic', 'unknown'];

const GNSS_BG: Record<GnssKey, string> = {
    gps: 'bg-gnss-gps',
    galileo: 'bg-gnss-galileo',
    beidou: 'bg-gnss-beidou',
    glonass: 'bg-gnss-glonass',
    qzss: 'bg-gnss-qzss',
    navic: 'bg-gnss-navic',
    unknown: 'bg-gnss-unknown',
};

const GNSS_FILL: Record<GnssKey, string> = {
    gps: 'fill-gnss-gps',
    galileo: 'fill-gnss-galileo',
    beidou: 'fill-gnss-beidou',
    glonass: 'fill-gnss-glonass',
    qzss: 'fill-gnss-qzss',
    navic: 'fill-gnss-navic',
    unknown: 'fill-gnss-unknown',
};

function gnssKey(svid: string): GnssKey {
    switch (svid[0]) {
        case 'G': case 'S': return 'gps';
        case 'E': return 'galileo';
        case 'C': return 'beidou';
        case 'R': return 'glonass';
        case 'J': return 'qzss';
        case 'I': return 'navic';
        default: return 'unknown';
    }
}

function tooltipText(sv: SVInfo): string {
    const az = sv.lookAngles ? sv.lookAngles.azimuth.toFixed(1) + '\u00b0' : '?';
    const el = sv.lookAngles ? sv.lookAngles.elevation.toFixed(1) + '\u00b0' : '?';
    const cn0 = sv.signals && sv.signals.length > 0 ? sv.signals[0].cn0.toFixed(1) : '?';
    const used = sv.used ? 'used' : 'unused';
    return `${sv.id}  az ${az}  el ${el}  CN0 ${cn0}  ${used}`;
}

export function SkyViewPanel({msg}: Props) {
    const satellites = useMemo(() => simplifySignals(msg?.info || []), [msg]);
    const usedValid = satellites.some(s => s.used === true);
    const svgRef = useRef<SVGSVGElement>(null);

    useEffect(() => {
        const svg = svgRef.current;
        if (!svg) return;
        const update = () => {
            const w = svg.getBoundingClientRect().width;
            document.documentElement.style.setProperty('--sky-unit-px', `${w / SIZE}px`);
        };
        update();
        const ro = new ResizeObserver(update);
        ro.observe(svg);
        return () => ro.disconnect();
    }, []);

    return (
        <div class="h-full w-full flex items-center justify-center bg-surface-1">
            <svg
                ref={svgRef}
                viewBox={`${-STROKE_PAD} ${-STROKE_PAD} ${SIZE + 2 * STROKE_PAD} ${SIZE + 2 * STROKE_PAD}`}
                preserveAspectRatio="xMidYMid meet"
                class="h-full max-h-full"
                style="aspect-ratio: 1 / 1;"
                xmlns="http://www.w3.org/2000/svg"
            >
                {/* Horizon circle */}
                <circle cx={RADIUS} cy={RADIUS} r={RADIUS} class="fill-none stroke-border-strong stroke-[1]" />

                {/* Elevation rings */}
                {[15, 30, 45, 60].map(el => (
                    <circle
                        key={el}
                        cx={RADIUS}
                        cy={RADIUS}
                        r={((90 - el) / 90) * RADIUS}
                        class="fill-none stroke-border-subtle stroke-[0.5]"
                    />
                ))}

                {/* Radial lines */}
                {Array.from({length: 12}, (_, i) => {
                    const angle = i * 30;
                    const rad = (angle - 90) * (Math.PI / 180);
                    const outerR = RADIUS;
                    const innerR = ((90 - 60) / 90) * RADIUS;
                    return (
                        <line
                            key={`r-${angle}`}
                            x1={RADIUS + outerR * Math.cos(rad)}
                            y1={RADIUS + outerR * Math.sin(rad)}
                            x2={RADIUS + innerR * Math.cos(rad)}
                            y2={RADIUS + innerR * Math.sin(rad)}
                            class="stroke-border-subtle stroke-[0.5]"
                        />
                    );
                })}

                {/* Compass markers */}
                <text x={RADIUS} y={COMPASS_PADDING} text-anchor="middle" dominant-baseline="hanging" class="fill-text-secondary text-[6px]">N</text>
                <text x={RADIUS} y={SIZE - COMPASS_PADDING} text-anchor="middle" class="fill-text-secondary text-[6px]">S</text>
                <text x={SIZE - COMPASS_PADDING} y={RADIUS + CAP_X_HEIGHT_DIFF} text-anchor="end" dominant-baseline="middle" class="fill-text-secondary text-[6px]">E</text>
                <text x={COMPASS_PADDING} y={RADIUS + CAP_X_HEIGHT_DIFF} text-anchor="start" dominant-baseline="middle" class="fill-text-secondary text-[6px]">W</text>

                {/* Satellite dots */}
                {satellites.map(sv => {
                    if (!sv.lookAngles) return null;
                    const [x, y] = toXY(sv.lookAngles.azimuth, sv.lookAngles.elevation);
                    const key = gnssKey(sv.id);
                    const unused = usedValid && !sv.used;
                    // Fade by signal strength when it is known; a satellite with no
                    // signal-strength info gets no fade class, so it stays fully visible.
                    const opacity = sv.signals && sv.signals.length > 0 ? opacityClassFor(sv.signals[0].cn0) : '';
                    return (
                        <g key={sv.id} class={opacity}>
                            <circle cx={x} cy={y} r={DOT_RADIUS} class={GNSS_FILL[key]}>
                                <title>{tooltipText(sv)}</title>
                            </circle>
                            {unused && (
                                <circle cx={x} cy={y} r={DOT_INNER_RADIUS} class="fill-surface-1 pointer-events-none" />
                            )}
                        </g>
                    );
                })}
            </svg>
        </div>
    );
}

interface LegendProps {
    msg: SatellitesMsg | null;
}

export function SkyViewLegend({msg}: LegendProps) {
    const presentKeys = useMemo(() => {
        const seen = new Set<GnssKey>();
        for (const sv of msg?.info || []) seen.add(gnssKey(sv.id));
        return GNSS_ORDER.filter(k => seen.has(k));
    }, [msg]);
    if (presentKeys.length === 0) return null;
    const box = DOT_RADIUS * 2;
    const px = `calc(var(--sky-unit-px, 2px) * ${box})`;
    return (
        <ul class="flex flex-col gap-1 text-xs text-text-secondary">
            {presentKeys.map(k => (
                <li key={k} class="flex items-center gap-1.5">
                    <svg style={{width: px, height: px}} viewBox={`${-box / 2} ${-box / 2} ${box} ${box}`} class="shrink-0">
                        <circle cx="0" cy="0" r={DOT_RADIUS} class={GNSS_FILL[k]} />
                    </svg>
                    {GNSS_LABEL[k]}
                </li>
            ))}
        </ul>
    );
}

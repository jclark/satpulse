import {h} from 'preact';
import {useMemo} from 'preact/hooks';
import type {SatellitesMsg, SVInfo, SignalInfo} from './app';

interface Props {
    msg: SatellitesMsg;
}

const SIZE = 200;
const RADIUS = SIZE / 2;
const STROKE_PAD = 1;
const MIN_ELEVATION = -3;
const COMPASS_PADDING = 2;
const CAP_X_HEIGHT_DIFF = 0.5;

function toXY(az: number, el: number): [number, number] {
    const r = ((90 - Math.max(el, MIN_ELEVATION)) / 90) * RADIUS;
    const rad = (az - 90) * (Math.PI / 180);
    return [RADIUS + r * Math.cos(rad), RADIUS + r * Math.sin(rad)];
}

function simplifySignals(satellites: SVInfo[]): SVInfo[] {
    return satellites
        .map(sv => ({...sv, signals: [{cn0: signalsCN0(sv.signals)}]}))
        .filter(sv => sv.signals[0].cn0 > 0);
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

function colorClassFor(svid: string): string {
    switch (svid[0]) {
        case 'G': case 'S': return 'fill-blue-600 dark:fill-blue-400';
        case 'E': return 'fill-green-600 dark:fill-green-400';
        case 'C': return 'fill-red-600 dark:fill-red-400';
        case 'R': return 'fill-fuchsia-600 dark:fill-fuchsia-400';
        case 'J': return 'fill-amber-600 dark:fill-amber-400';
        case 'I': return 'fill-yellow-600 dark:fill-yellow-300';
        default: return 'fill-gray-600 dark:fill-gray-400';
    }
}

function tooltipText(sv: SVInfo): string {
    const az = sv.lookAngles ? sv.lookAngles.azimuth.toFixed(1) + '\u00b0' : '?';
    const el = sv.lookAngles ? sv.lookAngles.elevation.toFixed(1) + '\u00b0' : '?';
    const cn0 = sv.signals[0].cn0.toFixed(1);
    const used = sv.used ? 'used' : 'unused';
    return `${sv.id}  az ${az}  el ${el}  CN0 ${cn0}  ${used}`;
}

export function SkyViewPanel({msg}: Props) {
    const satellites = useMemo(() => simplifySignals(msg.info || []), [msg]);
    const usedValid = satellites.some(s => s.used === true);

    return (
        <div class="flex-2 min-w-0 flex items-center justify-center bg-gray-100 dark:bg-gray-800">
            <svg
                viewBox={`${-STROKE_PAD} ${-STROKE_PAD} ${SIZE + 2 * STROKE_PAD} ${SIZE + 2 * STROKE_PAD}`}
                preserveAspectRatio="xMidYMid meet"
                class="w-full h-full max-h-full"
                style="aspect-ratio: 1 / 1;"
                xmlns="http://www.w3.org/2000/svg"
            >
                {/* Horizon circle */}
                <circle cx={RADIUS} cy={RADIUS} r={RADIUS} class="stroke-gray-400 fill-none stroke-[1]" />

                {/* Elevation rings */}
                {[15, 30, 45, 60].map(el => (
                    <circle
                        key={el}
                        cx={RADIUS}
                        cy={RADIUS}
                        r={((90 - el) / 90) * RADIUS}
                        class="stroke-gray-200 fill-none stroke-[0.5]"
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
                            class="stroke-gray-300 stroke-[0.5]"
                        />
                    );
                })}

                {/* Compass markers */}
                <text x={RADIUS} y={COMPASS_PADDING} text-anchor="middle" dominant-baseline="hanging" class="fill-gray-500 text-[6px]">N</text>
                <text x={RADIUS} y={SIZE - COMPASS_PADDING} text-anchor="middle" class="fill-gray-500 text-[6px]">S</text>
                <text x={SIZE - COMPASS_PADDING} y={RADIUS + CAP_X_HEIGHT_DIFF} text-anchor="end" dominant-baseline="middle" class="fill-gray-500 text-[6px]">E</text>
                <text x={COMPASS_PADDING} y={RADIUS + CAP_X_HEIGHT_DIFF} text-anchor="start" dominant-baseline="middle" class="fill-gray-500 text-[6px]">W</text>

                {/* Satellite labels */}
                {satellites.map(sv => {
                    if (!sv.lookAngles) return null;
                    const [x, y] = toXY(sv.lookAngles.azimuth, sv.lookAngles.elevation);
                    const unused = usedValid && !sv.used;
                    return (
                        <text
                            key={sv.id}
                            x={x}
                            y={y}
                            text-anchor="middle"
                            dominant-baseline="middle"
                            class={`text-[3px] font-bold ${colorClassFor(sv.id)} ${opacityClassFor(sv.signals[0].cn0)}`}
                        >
                            <title>{tooltipText(sv)}</title>
                            {unused ? <tspan class="opacity-0">-</tspan> : ''}
                            {sv.id}
                            {unused ? '-' : ''}
                        </text>
                    );
                })}
            </svg>
        </div>
    );
}

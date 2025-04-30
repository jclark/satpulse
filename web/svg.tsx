export interface SVInfo {
    azimuth: number;     // 0 to 360 degrees
    elevation: number;   // -90 to 90 degrees (negative unusual)
    id: string;        // e.g., "G01"
    signals: SignalInfo[];  // Array of signal information
    used?: boolean;      // true if known to be used in navigation solution
}

export interface SignalInfo {
    cn0: number;         // Signal carrier-to-noise ratio
    id?: string;         // Optional signal identifier
}

// Generate the SVG element for the SkyView.
// Assumes satellites has already been simplified with simplifySignals.
export function SkyView(satellites: SVInfo[]) {
    const SIZE = 200;
    const RADIUS = SIZE / 2;
    const STROKE_PAD = 1;
    const MIN_ELEVATION = -3; // minimum elevation to display; show just outside the circle
    const COMPASS_PADDING = 2; // padding for compass points from the edge
    const CAP_X_HEIGHT_DIFF = 0.5; // Difference between middle of cap height and middle of x-height
    
    const toXY = (az: number, el: number): [number, number] => {
        const r = ((90 - Math.max(el, MIN_ELEVATION)) / 90) * RADIUS;
        const rad = (az - 90) * (Math.PI / 180); // 0° = east in SVG
        const x = RADIUS + r * Math.cos(rad);
        const y = RADIUS + r * Math.sin(rad);
        return [x, y];
    };

    return (
        <svg
            viewBox={`${-STROKE_PAD} ${-STROKE_PAD} ${SIZE + 2 * STROKE_PAD} ${SIZE + 2 * STROKE_PAD}`}
            preserveAspectRatio="xMidYMid meet"
            class="w-full h-auto"
            xmlns="http://www.w3.org/2000/svg"
        >
            <circle cx={RADIUS} cy={RADIUS} r={RADIUS} class="stroke-gray-400 fill-none stroke-[1]" />
            {[15, 30, 45, 60].map((el) => (
                <circle
                    key={el}
                    cx={RADIUS}
                    cy={RADIUS}
                    r={((90 - el) / 90) * RADIUS}
                    class="stroke-gray-200 fill-none stroke-[0.5]"
                />
            ))}
            {(() => {
                const outerR = RADIUS; // outermost (0° elevation)
                const innerR = ((90 - 60) / 90) * RADIUS; // innermost (75° elevation)
                return Array.from({ length: 12 }, (_, i) => {
                    const angle = i * 30;
                    const rad = (angle - 90) * (Math.PI / 180);
                    const x1 = RADIUS + outerR * Math.cos(rad);
                    const y1 = RADIUS + outerR * Math.sin(rad);
                    const x2 = RADIUS + innerR * Math.cos(rad);
                    const y2 = RADIUS + innerR * Math.sin(rad);
                    return (
                        <line
                            key={`radial-${angle}`}
                            x1={x1}
                            y1={y1}
                            x2={x2}
                            y2={y2}
                            class="stroke-gray-300 stroke-[0.5]"
                        />
                    );
                });
            })()}
            <text x={RADIUS} y={COMPASS_PADDING} text-anchor="middle" dominant-baseline="hanging" class="fill-gray-500 text-[6px]">N</text>
            <text x={RADIUS} y={SIZE - COMPASS_PADDING} text-anchor="middle" class="fill-gray-500 text-[6px]">S</text>
            <text x={SIZE - COMPASS_PADDING} y={RADIUS + CAP_X_HEIGHT_DIFF} text-anchor="end" dominant-baseline="middle" class="fill-gray-500 text-[6px]">E</text>
            <text x={COMPASS_PADDING} y={RADIUS + CAP_X_HEIGHT_DIFF} text-anchor="start" dominant-baseline="middle" class="fill-gray-500 text-[6px]">W</text>
            
            {satellites.map((sv) => {
                const [x, y] = toXY(sv.azimuth, sv.elevation);
                // A satellite is known to be unused if some satellite is marked as used and sv.used is not true.
                // Mark unused satellites with "-" if known to be unused.
                // Balance the trailing "-" with an invisible leading "-", so that the svid is centered.
                const usedValid = satellites.some(s => s.used === true);
                const unused = usedValid && !sv.used; // known unused             
                return (
                    <text
                        key={sv.id}
                        x={x}
                        y={y}
                        text-anchor="middle"
                        dominant-baseline="middle"
                        class={`text-[3px] font-bold ${colorClassFor(sv.id)} ${opacityClassFor(sv.signals[0].cn0)}`}
                    >
                        {unused ? <tspan class="opacity-0">-</tspan> : ''}
                        {sv.id}
                        {unused ? '-' : ''}
                    </text>
                );
            })}
        </svg>
    );
}

export function simplifySignals(satellites: SVInfo[]): SVInfo[] {
    return satellites
        .map(sv => {
            return {
                ...sv,
                signals: [{ cn0: signalsCN0(sv.signals) }]
            };
        })
        .filter(sv => sv.signals[0].cn0 > 0);
}

function signalsCN0(signals: SignalInfo[]): number {
    if (!signals) {
        return 0;
    }
    
    // Look for anonymous signal (id == "" or undefined) that has non-zero CN0
    let anonSignal = signals.find(s => (s.id === "" || s.id === undefined));
    if (anonSignal && anonSignal.cn0 > 0) {
        return anonSignal.cn0;
    }
    
    // Otherwise, find the maximum CN0
    return signals.reduce((maxCno, signal) => Math.max(maxCno, signal.cn0), 0);
}

function opacityClassFor(cno: number): string {
    if (cno < 25) return 'opacity-20';
    if (cno < 30) return 'opacity-40';
    if (cno < 35) return 'opacity-60';
    if (cno < 42) return 'opacity-75';
    if (cno < 50) return 'opacity-90';
    return 'opacity-100';
}

function colorClassFor(svid: string): string {
    const prefix = svid[0];
    switch (prefix) {
        case 'G': // GPS
        case 'S': // SBAS
            return 'fill-blue-600 dark:fill-blue-400';
        case 'E': // Galileo
            return 'fill-green-600 dark:fill-green-400';
        case 'C': // BeiDou
            return 'fill-red-600 dark:fill-red-400';
        case 'R': // GLONASS
            return 'fill-fuchsia-600 dark:fill-fuchsia-400';
        case 'J': // QZSS - Japan
            return 'fill-amber-600 dark:fill-amber-400';
        case 'I': // NavIC
            return 'fill-yellow-600 dark:fill-yellow-300';
        default:
            return 'fill-gray-600 dark:fill-gray-400';
    }
}

// Generate the SVG element for the signal level bar graph.
// Assumes satellites has already been simplified with simplifySignals.
export function SignalGraph(satellites: SVInfo[], maxSatelliteCount: number, isDoubleRow: boolean) {
    const WIDTH = 300;
    const MARGIN = { top: 20, right: 10, bottom: 10, left: 35 };
    const VERTICAL_MARGINS = MARGIN.top + MARGIN.bottom;
    const CHART_WIDTH = WIDTH - MARGIN.left - MARGIN.right;
    
    // Height constants (including margins)
    const MIN_HEIGHT = 200;
    const SINGLE_ROW_HEIGHT = 240;
    const DOUBLE_ROW_HEIGHT = 480;
    var height = isDoubleRow ? DOUBLE_ROW_HEIGHT : SINGLE_ROW_HEIGHT;
    var chartHeight = height - VERTICAL_MARGINS;

    const MIN_SATELLITES = 4
    const MAX_CNO = 55; // accomodate CNO up to this
    
    // Bar styling
    const MIN_BAR_HEIGHT = 14;
    const MAX_BAR_HEIGHT = 24;
    const BAR_SPACING = 4;
    
    satellites.sort((a, b) => {
        // First sort by constellation
        if (a.id[0] !== b.id[0]) {
            return a.id[0].localeCompare(b.id[0]);
        }
        // Then sort by the numeric part of the SVID
        const aNum = parseInt(a.id.substring(1));
        const bNum = parseInt(b.id.substring(1));
        return aNum - bNum;
    });
        
    // Calculate bar height based on satellite count
    const barHeight = Math.min(
        MAX_BAR_HEIGHT, 
        Math.max(MIN_BAR_HEIGHT, chartHeight / Math.max(MIN_SATELLITES, maxSatelliteCount))
    );
    const spacedBarHeight = barHeight + BAR_SPACING;

    const gridLineHeight = (satellites.length * spacedBarHeight) - BAR_SPACING;
    
    // Recompute chart height using maxSatelliteCount not current number of satellites
    chartHeight = maxSatelliteCount * spacedBarHeight
    
    // Recompute height
    height = Math.max(MIN_HEIGHT, chartHeight + VERTICAL_MARGINS);
    
    return (
        <svg
            viewBox={`0 0 ${WIDTH} ${height}`}
            preserveAspectRatio="xMidYMid meet"
            class="w-full h-full"
            xmlns="http://www.w3.org/2000/svg"
        >
            {/* X-axis and grid lines */}
            <g transform={`translate(${MARGIN.left}, ${MARGIN.top})`}>
                {/* X-axis labels */}
                {[0, 10, 20, 30, 40, 50].map((value) => (
                    <text
                        key={`x-label-${value}`}
                        x={(value / MAX_CNO) * CHART_WIDTH}
                        y={-5}
                        text-anchor="middle"
                        dominant-baseline="auto"
                        class="fill-gray-600 dark:fill-gray-100 text-xs"
                    >
                        {value}
                    </text>
                ))}

                {/* Vertical grid lines */}
                {[0, 10, 20, 30, 40, 50].map((value) => (
                    <line
                        key={`grid-${value}`}
                        x1={(value / MAX_CNO) * CHART_WIDTH}
                        y1={0}
                        x2={(value / MAX_CNO) * CHART_WIDTH}
                        y2={gridLineHeight}
                        class="stroke-gray-200 dark:stroke-gray-700 stroke-[0.5]"
                    />
                ))}

                {/* Bars */}
                {satellites.map((sv, i) => {
                    const cno = Math.min(sv.signals[0].cn0, MAX_CNO);
                    const barWidth = (cno / MAX_CNO) * CHART_WIDTH;
                    return (
                        <g key={sv.id} transform={`translate(0, ${i * spacedBarHeight})`}>
                            {/* SVID label */}
                            <text
                                x={-8} // Increased space from -5 to -8 to prevent text from being cut off
                                y={barHeight / 2}
                                text-anchor="end"
                                dominant-baseline="central"
                                class="text-[10px] tabular-nums fill-gray-800 dark:fill-gray-200"
                            >
                                {sv.id}
                            </text>

                            {/* Bar */}
                            <rect
                                x={0}
                                y={0}
                                width={barWidth}
                                height={barHeight}
                                class={colorClassFor(sv.id)}
                            />
                        </g>
                    );
                })}
            </g>
        </svg>
    );
}

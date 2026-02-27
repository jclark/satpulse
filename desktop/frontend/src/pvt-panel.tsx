import {h, Fragment} from 'preact';
import {useState, useEffect} from 'preact/hooks';
import {ECEFtoLLH, LLHtoECEF, VelNEDtoECEF, VelECEFtoNED} from '../wailsjs/go/main/App';
import {formatDateTime, formatTAI, formatUTCLocal} from './timefmt';
import type {LeapSecondState} from './app';
import type {ComponentChildren} from 'preact';

const blank = '\u2014';

// parseTAITime parses a ptime.Time string ("seconds.nanoseconds") to seconds.
function parseTAITime(t: string): number {
    const dot = t.indexOf('.');
    if (dot < 0) return parseInt(t, 10);
    return parseInt(t.substring(0, dot), 10);
}

function fmtDeg(deg: number, digits: number): string {
    return deg.toFixed(digits);
}

function fmtM(m: number, digits: number): string {
    return m.toFixed(digits);
}

function fmtMs(ms: number, digits: number): string {
    return ms.toFixed(digits);
}

function fmtNs(ns: number): string {
    if (ns >= 1e9) return (ns / 1e9).toFixed(1) + ' s';
    if (ns >= 1e6) return (ns / 1e6).toFixed(1) + ' ms';
    if (ns >= 1e3) return (ns / 1e3).toFixed(1) + ' us';
    return ns + ' ns';
}

// --- Row types matching what we get from Go ---

export interface PosGeoRow {
    kind: 'posGeo';
    tag: string;
    nativeMsgID: string;
    latLon: [number, number];
    height?: number;
    heightMSL?: number;
}

export interface PosECEFRow {
    kind: 'posECEF';
    tag: string;
    nativeMsgID: string;
    pos: [number, number, number];
}

export type PosRow = PosGeoRow | PosECEFRow;

export interface VelGeoRow {
    kind: 'velGeo';
    tag: string;
    nativeMsgID: string;
    velNED?: [number, number, number];
    groundSpeed?: number;
    speed3D?: number;
    course?: number;
}

export interface VelECEFRow {
    kind: 'velECEF';
    tag: string;
    nativeMsgID: string;
    vel: [number, number, number];
}

export type VelRow = VelGeoRow | VelECEFRow;

export interface TimeRow {
    tag: string;
    nativeMsgID: string;
    ref: number;
    taiTime?: string;
    utcTime?: string;
    accuracy?: number;
    utcOffset?: number;
    gnss?: string;
}

// --- Conversion cache types ---

interface PosGeoConverted {
    ecefX: number; ecefY: number; ecefZ: number;
}

interface PosECEFConverted {
    lat: number; lon: number; height: number;
}

// --- Props ---

interface Props {
    posRows: Map<string, PosRow>;
    velRows: Map<string, VelRow>;
    timeRows: Map<string, TimeRow>;
    leapSecond: LeapSecondState | null;
}

// Bold wrapper for native fields
function N({children}: {children: preact.ComponentChildren}) {
    return <span class="font-bold">{children}</span>;
}

const cellPad = 'pr-3 py-0.5';

function PVTMsgTable({title, columns, empty, children, hasData}: {
    title: string;
    columns: string[];
    empty: string;
    children: ComponentChildren;
    hasData: boolean;
}) {
    return (
        <section class="space-y-2">
            <h3 class="mb-1 text-xs font-semibold uppercase tracking-wider text-text-secondary">{title}</h3>
            {hasData ? (
                <div class="overflow-x-auto">
                    <table class="w-full border-collapse text-xs">
                        <thead class="text-text-muted">
                            <tr>
                                {columns.map(col => <th key={col} class={`${cellPad} text-left`}>{col}</th>)}
                            </tr>
                        </thead>
                        <tbody class="font-mono tabular-nums text-text-primary">
                            {children}
                        </tbody>
                    </table>
                </div>
            ) : (
                <p class="text-xs text-text-muted">{empty}</p>
            )}
        </section>
    );
}

// --- Position table ---
// Columns: Tag | Message | Latitude,Longitude | Height | Height MSL | ECEF

function PositionTable({rows}: {rows: Map<string, PosRow>}) {
    const [geoConv, setGeoConv] = useState<Map<string, PosGeoConverted>>(new Map());
    const [ecefConv, setEcefConv] = useState<Map<string, PosECEFConverted>>(new Map());

    // Convert geo rows to ECEF
    useEffect(() => {
        const geoRows = [...rows.values()].filter((r): r is PosGeoRow => r.kind === 'posGeo' && r.height != null);
        if (geoRows.length === 0) { setGeoConv(new Map()); return; }
        let cancelled = false;
        Promise.all(geoRows.map(async r => {
            const lat = r.latLon[0];
            const lon = r.latLon[1];
            const h = r.height!;
            const ecef = await LLHtoECEF(lat, lon, h);
            return [r.nativeMsgID, {ecefX: ecef[0], ecefY: ecef[1], ecefZ: ecef[2]}] as const;
        })).then(pairs => {
            if (!cancelled) setGeoConv(new Map(pairs));
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [...[...rows.values()].filter(r => r.kind === 'posGeo').flatMap(r => {
        const g = r as PosGeoRow;
        return [g.latLon[0], g.latLon[1], g.height ?? 0];
    })]);

    // Convert ECEF rows to LLH
    useEffect(() => {
        const ecefRows = [...rows.values()].filter((r): r is PosECEFRow => r.kind === 'posECEF');
        if (ecefRows.length === 0) { setEcefConv(new Map()); return; }
        let cancelled = false;
        Promise.all(ecefRows.map(async r => {
            const x = r.pos[0];
            const y = r.pos[1];
            const z = r.pos[2];
            const llh = await ECEFtoLLH(x, y, z);
            if (!llh) return null;
            return [r.nativeMsgID, {lat: llh.lat, lon: llh.lon, height: llh.height}] as const;
        })).then(pairs => {
            if (!cancelled) setEcefConv(new Map(pairs.filter(Boolean) as [string, PosECEFConverted][]));
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [...[...rows.values()].filter(r => r.kind === 'posECEF').flatMap(r => {
        const e = r as PosECEFRow;
        return [e.pos[0], e.pos[1], e.pos[2]];
    })]);

    const sorted = rows.size > 0 ? [...rows.values()].sort((a, b) => a.nativeMsgID.localeCompare(b.nativeMsgID)) : [];

    return (
        <PVTMsgTable title="Position" columns={['Tag', 'Message', 'Latitude,Longitude', 'Height', 'Height MSL', 'ECEF']} empty="No position data" hasData={rows.size > 0}>
            {sorted.map(row => {
                if (row.kind === 'posGeo') {
                    const conv = geoConv.get(row.nativeMsgID);
                    const latLon = <N>{fmtDeg(row.latLon[0], 7)},{fmtDeg(row.latLon[1], 7)}</N>;
                    const ecef = conv
                        ? <>{fmtM(conv.ecefX, 4)},{fmtM(conv.ecefY, 4)},{fmtM(conv.ecefZ, 4)}</>
                        : blank;
                    return (
                        <tr key={row.nativeMsgID}>
                            <td class={cellPad}>{row.tag}</td>
                            <td class={cellPad}>{row.nativeMsgID}</td>
                            <td class={cellPad}>{latLon}</td>
                            <td class={cellPad}>{row.height != null ? <N>{fmtM(row.height, 4)}</N> : blank}</td>
                            <td class={cellPad}>{row.heightMSL != null ? <N>{fmtM(row.heightMSL, 4)}</N> : blank}</td>
                            <td class={cellPad}>{ecef}</td>
                        </tr>
                    );
                } else {
                    const conv = ecefConv.get(row.nativeMsgID);
                    const latLon = conv
                        ? <>{fmtDeg(conv.lat, 7)},{fmtDeg(conv.lon, 7)}</>
                        : blank;
                    const ecef = <N>{fmtM(row.pos[0], 4)},{fmtM(row.pos[1], 4)},{fmtM(row.pos[2], 4)}</N>;
                    return (
                        <tr key={row.nativeMsgID}>
                            <td class={cellPad}>{row.tag}</td>
                            <td class={cellPad}>{row.nativeMsgID}</td>
                            <td class={cellPad}>{latLon}</td>
                            <td class={cellPad}>{conv ? fmtM(conv.height, 4) : blank}</td>
                            <td class={cellPad}>{blank}</td>
                            <td class={cellPad}>{ecef}</td>
                        </tr>
                    );
                }
            })}
        </PVTMsgTable>
    );
}

// --- Velocity table ---
// Columns: Tag | Message | Ground speed | Speed | Course | NED | ECEF

function VelocityTable({rows}: {rows: Map<string, VelRow>}) {
    const [nedToEcef, setNedToEcef] = useState<Map<string, [number, number, number]>>(new Map());
    const [ecefToNed, setEcefToNed] = useState<Map<string, [number, number, number]>>(new Map());

    // Convert VelGeo NED -> ECEF
    useEffect(() => {
        const geoRows = [...rows.values()].filter((r): r is VelGeoRow => r.kind === 'velGeo' && r.velNED != null);
        if (geoRows.length === 0) { setNedToEcef(new Map()); return; }
        let cancelled = false;
        Promise.all(geoRows.map(async r => {
            const ecef = await VelNEDtoECEF(r.velNED![0], r.velNED![1], r.velNED![2]);
            if (!ecef) return null;
            return [r.nativeMsgID, ecef as [number, number, number]] as const;
        })).then(pairs => {
            if (!cancelled) setNedToEcef(new Map(pairs.filter(Boolean) as [string, [number, number, number]][]));
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [...[...rows.values()].filter(r => r.kind === 'velGeo').flatMap(r => {
        const g = r as VelGeoRow;
        return g.velNED ?? [];
    })]);

    // Convert VelECEF -> NED
    useEffect(() => {
        const ecefRows = [...rows.values()].filter((r): r is VelECEFRow => r.kind === 'velECEF');
        if (ecefRows.length === 0) { setEcefToNed(new Map()); return; }
        let cancelled = false;
        Promise.all(ecefRows.map(async r => {
            const ned = await VelECEFtoNED(r.vel[0], r.vel[1], r.vel[2]);
            if (!ned) return null;
            return [r.nativeMsgID, ned as [number, number, number]] as const;
        })).then(pairs => {
            if (!cancelled) setEcefToNed(new Map(pairs.filter(Boolean) as [string, [number, number, number]][]));
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [...[...rows.values()].filter(r => r.kind === 'velECEF').flatMap(r => {
        const e = r as VelECEFRow;
        return e.vel;
    })]);

    const sorted = rows.size > 0 ? [...rows.values()].sort((a, b) => a.nativeMsgID.localeCompare(b.nativeMsgID)) : [];

    return (
        <PVTMsgTable title="Velocity" columns={['Tag', 'Message', 'Speed (m/s)', 'Ground speed', 'Course (\u00B0)', 'North,East,Down', 'ECEF (m/s)']} empty="No velocity data" hasData={rows.size > 0}>
            {sorted.map(row => {
                if (row.kind === 'velGeo') {
                    const ned = row.velNED
                        ? <N>{fmtMs(row.velNED[0], 4)},{fmtMs(row.velNED[1], 4)},{fmtMs(row.velNED[2], 4)}</N>
                        : blank;
                    const conv = nedToEcef.get(row.nativeMsgID);
                    const ecef = conv
                        ? <>{fmtMs(conv[0], 4)},{fmtMs(conv[1], 4)},{fmtMs(conv[2], 4)}</>
                        : blank;
                    return (
                        <tr key={row.nativeMsgID}>
                            <td class={cellPad}>{row.tag}</td>
                            <td class={cellPad}>{row.nativeMsgID}</td>
                            <td class={cellPad}>{row.speed3D != null
                                ? <N>{fmtMs(row.speed3D, 4)}</N>
                                : row.velNED != null
                                    ? fmtMs(Math.sqrt(row.velNED[0]**2 + row.velNED[1]**2 + row.velNED[2]**2), 4)
                                    : blank}</td>
                            <td class={cellPad}>{row.groundSpeed != null ? <N>{fmtMs(row.groundSpeed, 4)}</N> : blank}</td>
                            <td class={cellPad}>{row.course != null ? <N>{fmtDeg(row.course, 2)}</N> : blank}</td>
                            <td class={cellPad}>{ned}</td>
                            <td class={cellPad}>{ecef}</td>
                        </tr>
                    );
                } else {
                    const ecefVel = <N>{fmtMs(row.vel[0], 4)},{fmtMs(row.vel[1], 4)},{fmtMs(row.vel[2], 4)}</N>;
                    const conv = ecefToNed.get(row.nativeMsgID);
                    const ned = conv
                        ? <>{fmtMs(conv[0], 4)},{fmtMs(conv[1], 4)},{fmtMs(conv[2], 4)}</>
                        : blank;
                    return (
                        <tr key={row.nativeMsgID}>
                            <td class={cellPad}>{row.tag}</td>
                            <td class={cellPad}>{row.nativeMsgID}</td>
                            <td class={cellPad}>{blank}</td>
                            <td class={cellPad}>{blank}</td>
                            <td class={cellPad}>{blank}</td>
                            <td class={cellPad}>{ned}</td>
                            <td class={cellPad}>{ecefVel}</td>
                        </tr>
                    );
                }
            })}
        </PVTMsgTable>
    );
}

// --- Time table ---
// Columns: Tag | Message | Local | UTC | TAI | Leap sec | TAcc | GNSS

function TimeTable({rows, leapSecond}: {rows: Map<string, TimeRow>; leapSecond: LeapSecondState | null}) {
    const sorted = rows.size > 0 ? [...rows.values()].sort((a, b) => a.nativeMsgID.localeCompare(b.nativeMsgID)) : [];

    return (
        <PVTMsgTable title="Time" columns={['Tag', 'Message', 'Local', 'UTC', 'TAI', 'Leap sec', 'TAcc', 'GNSS']} empty="No time data" hasData={rows.size > 0}>
            {sorted.map(row => {
                // Determine the UTC ISO string for computing local and display
                let utcISO = '';
                let utcBold = false;
                if (row.utcTime) {
                    utcISO = row.utcTime;
                    utcBold = true;
                } else if (row.taiTime) {
                    const ls = row.utcOffset || (leapSecond?.utcOff ?? 0);
                    if (ls > 0) {
                        const taiSecs = parseTAITime(row.taiTime);
                        if (taiSecs > 0) {
                            utcISO = new Date((taiSecs - ls) * 1000).toISOString();
                        }
                    }
                }
                const utc = utcISO ? formatDateTime(utcISO) : blank;

                // Local time of day (always computed, not bold)
                let local = blank;
                if (utcISO) {
                    const dt = formatUTCLocal(utcISO);
                    if (dt) local = dt.time;
                }

                // Determine TAI: native if taiTime present, else computed from UTC + leap seconds
                let tai = blank;
                let taiBold = false;
                if (row.taiTime) {
                    const taiSecs = parseTAITime(row.taiTime);
                    if (taiSecs > 0) {
                        tai = formatTAI(taiSecs);
                        taiBold = true;
                    }
                } else if (row.utcTime) {
                    const ls = row.utcOffset || (leapSecond?.utcOff ?? 0);
                    if (ls > 0) {
                        const utcDate = new Date(row.utcTime);
                        const utcSecs = Math.floor(utcDate.getTime() / 1000);
                        if (utcSecs > 0) {
                            tai = formatTAI(utcSecs + ls);
                        }
                    }
                }

                // Leap seconds: native from message if utcOffset non-zero, else from global
                let leapStr = blank;
                let leapBold = false;
                if (row.utcOffset) {
                    leapStr = String(row.utcOffset);
                    leapBold = true;
                } else if (leapSecond) {
                    leapStr = String(leapSecond.utcOff);
                }

                return (
                    <tr key={row.nativeMsgID}>
                        <td class={cellPad}>{row.tag}</td>
                        <td class={cellPad}>{row.nativeMsgID}</td>
                        <td class={cellPad}>{local}</td>
                        <td class={cellPad}>{utcBold ? <N>{utc}</N> : utc}</td>
                        <td class={cellPad}>{taiBold ? <N>{tai}</N> : tai}</td>
                        <td class={cellPad}>{leapBold ? <N>{leapStr}</N> : leapStr}</td>
                        <td class={cellPad}>{row.accuracy ? <N>{fmtNs(row.accuracy)}</N> : blank}</td>
                        <td class={cellPad}>{row.gnss ? <N>{row.gnss}</N> : blank}</td>
                    </tr>
                );
            })}
        </PVTMsgTable>
    );
}

// --- Main PVT panel ---

export function PVTPanel({posRows, velRows, timeRows, leapSecond}: Props) {
    return (
        <div class="space-y-4">
            <PositionTable rows={posRows} />
            <VelocityTable rows={velRows} />
            <TimeTable rows={timeRows} leapSecond={leapSecond} />
        </div>
    );
}

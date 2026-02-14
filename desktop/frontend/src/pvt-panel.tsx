import {h, Fragment} from 'preact';
import {useState, useEffect} from 'preact/hooks';
import {ECEFtoLLH, LLHtoECEF} from '../wailsjs/go/main/App';
import {formatDateTime, formatTAI, formatUTCLocal} from './timefmt';
import type {LeapSecondState} from './app';

// Unit conversions from JSON wire format
function ndegToDeg(nd: number): number { return nd / 1e9; }
function umToM(um: number): number { return um / 1e6; }
function umsToMs(ums: number): number { return ums / 1e6; }

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
    hAcc?: number;
    vAcc?: number;
}

export interface PosECEFRow {
    kind: 'posECEF';
    tag: string;
    nativeMsgID: string;
    pos: [number, number, number];
    pAcc?: number;
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
    sAcc?: number;
    cAcc?: number;
}

export interface VelECEFRow {
    kind: 'velECEF';
    tag: string;
    nativeMsgID: string;
    vel: [number, number, number];
    sAcc?: number;
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

const td = 'pr-3 py-0.5';
const th = 'pr-3 py-0.5 font-medium font-sans';

// --- Position table ---
// Columns: Tag | Message | Lat,Lon | H Acc | Height | Height MSL | V Acc | ECEF | P Acc

function PositionTable({rows}: {rows: Map<string, PosRow>}) {
    const [geoConv, setGeoConv] = useState<Map<string, PosGeoConverted>>(new Map());
    const [ecefConv, setEcefConv] = useState<Map<string, PosECEFConverted>>(new Map());

    // Convert geo rows to ECEF
    useEffect(() => {
        const geoRows = [...rows.values()].filter((r): r is PosGeoRow => r.kind === 'posGeo' && r.height != null);
        if (geoRows.length === 0) { setGeoConv(new Map()); return; }
        let cancelled = false;
        Promise.all(geoRows.map(async r => {
            const lat = ndegToDeg(r.latLon[0]);
            const lon = ndegToDeg(r.latLon[1]);
            const h = umToM(r.height!);
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
            const x = umToM(r.pos[0]);
            const y = umToM(r.pos[1]);
            const z = umToM(r.pos[2]);
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

    if (rows.size === 0) return <p class="text-xs text-gray-400">No position data</p>;

    const sorted = [...rows.values()].sort((a, b) => a.nativeMsgID.localeCompare(b.nativeMsgID));

    return (
        <table class="text-xs font-mono tabular-nums w-full border-collapse">
            <thead>
                <tr class="text-left text-gray-500 dark:text-gray-400">
                    <th class={`${th}`}>Tag</th>
                    <th class={`${th}`}>Message</th>
                    <th class={`${th}`}>Lat,Lon</th>
                    <th class={`${th}`}>H Acc</th>
                    <th class={`${th}`}>Height</th>
                    <th class={`${th}`}>Height MSL</th>
                    <th class={`${th}`}>V Acc</th>
                    <th class={`${th}`}>ECEF</th>
                    <th class={`${th}`}>P Acc</th>
                </tr>
            </thead>
            <tbody>
                {sorted.map(row => {
                    if (row.kind === 'posGeo') {
                        const conv = geoConv.get(row.nativeMsgID);
                        const latLon = <N>{fmtDeg(ndegToDeg(row.latLon[0]), 7)},{fmtDeg(ndegToDeg(row.latLon[1]), 7)}</N>;
                        const ecef = conv
                            ? <>{fmtM(conv.ecefX, 4)},{fmtM(conv.ecefY, 4)},{fmtM(conv.ecefZ, 4)}</>
                            : blank;
                        return (
                            <tr key={row.nativeMsgID}>
                                <td class={td}>{row.tag}</td>
                                <td class={td}>{row.nativeMsgID}</td>
                                <td class={td}>{latLon}</td>
                                <td class={td}>{row.hAcc != null ? <N>{fmtM(umToM(row.hAcc), 4)}</N> : blank}</td>
                                <td class={td}>{row.height != null ? <N>{fmtM(umToM(row.height), 4)}</N> : blank}</td>
                                <td class={td}>{row.heightMSL != null ? <N>{fmtM(umToM(row.heightMSL), 4)}</N> : blank}</td>
                                <td class={td}>{row.vAcc != null ? <N>{fmtM(umToM(row.vAcc), 4)}</N> : blank}</td>
                                <td class={td}>{ecef}</td>
                                <td class={td}>{blank}</td>
                            </tr>
                        );
                    } else {
                        const conv = ecefConv.get(row.nativeMsgID);
                        const latLon = conv
                            ? <>{fmtDeg(conv.lat, 7)},{fmtDeg(conv.lon, 7)}</>
                            : blank;
                        const ecef = <N>{fmtM(umToM(row.pos[0]), 4)},{fmtM(umToM(row.pos[1]), 4)},{fmtM(umToM(row.pos[2]), 4)}</N>;
                        return (
                            <tr key={row.nativeMsgID}>
                                <td class={td}>{row.tag}</td>
                                <td class={td}>{row.nativeMsgID}</td>
                                <td class={td}>{latLon}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{conv ? fmtM(conv.height, 4) : blank}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{ecef}</td>
                                <td class={td}>{row.pAcc != null ? <N>{fmtM(umToM(row.pAcc), 4)}</N> : blank}</td>
                            </tr>
                        );
                    }
                })}
            </tbody>
        </table>
    );
}

// --- Velocity table ---
// Columns: Tag | Message | Ground spd | 3D spd | Spd acc | Course | Course acc | NED | ECEF vel

function VelocityTable({rows}: {rows: Map<string, VelRow>}) {
    if (rows.size === 0) return <p class="text-xs text-gray-400">No velocity data</p>;
    const sorted = [...rows.values()].sort((a, b) => a.nativeMsgID.localeCompare(b.nativeMsgID));

    return (
        <table class="text-xs font-mono tabular-nums w-full border-collapse">
            <thead>
                <tr class="text-left text-gray-500 dark:text-gray-400">
                    <th class={`${th}`}>Tag</th>
                    <th class={`${th}`}>Message</th>
                    <th class={`${th}`}>Ground spd</th>
                    <th class={`${th}`}>3D spd</th>
                    <th class={`${th}`}>Spd acc</th>
                    <th class={`${th}`}>Course</th>
                    <th class={`${th}`}>Course acc</th>
                    <th class={`${th}`}>NED</th>
                    <th class={`${th}`}>ECEF vel</th>
                </tr>
            </thead>
            <tbody>
                {sorted.map(row => {
                    if (row.kind === 'velGeo') {
                        const ned = row.velNED
                            ? <N>{fmtMs(umsToMs(row.velNED[0]), 4)},{fmtMs(umsToMs(row.velNED[1]), 4)},{fmtMs(umsToMs(row.velNED[2]), 4)}</N>
                            : blank;
                        return (
                            <tr key={row.nativeMsgID}>
                                <td class={td}>{row.tag}</td>
                                <td class={td}>{row.nativeMsgID}</td>
                                <td class={td}>{row.groundSpeed != null ? <N>{fmtMs(umsToMs(row.groundSpeed), 4)}</N> : blank}</td>
                                <td class={td}>{row.speed3D != null ? <N>{fmtMs(umsToMs(row.speed3D), 4)}</N> : blank}</td>
                                <td class={td}>{row.sAcc != null ? <N>{fmtMs(umsToMs(row.sAcc), 4)}</N> : blank}</td>
                                <td class={td}>{row.course != null ? <N>{fmtDeg(ndegToDeg(row.course), 2)}</N> : blank}</td>
                                <td class={td}>{row.cAcc != null ? <N>{fmtDeg(ndegToDeg(row.cAcc), 2)}</N> : blank}</td>
                                <td class={td}>{ned}</td>
                                <td class={td}>{blank}</td>
                            </tr>
                        );
                    } else {
                        const ecefVel = <N>{fmtMs(umsToMs(row.vel[0]), 4)},{fmtMs(umsToMs(row.vel[1]), 4)},{fmtMs(umsToMs(row.vel[2]), 4)}</N>;
                        return (
                            <tr key={row.nativeMsgID}>
                                <td class={td}>{row.tag}</td>
                                <td class={td}>{row.nativeMsgID}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{row.sAcc != null ? <N>{fmtMs(umsToMs(row.sAcc), 4)}</N> : blank}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{blank}</td>
                                <td class={td}>{ecefVel}</td>
                            </tr>
                        );
                    }
                })}
            </tbody>
        </table>
    );
}

// --- Time table ---
// Columns: Tag | Message | Local | UTC | TAI | Leap sec | TAcc | GNSS

function TimeTable({rows, leapSecond}: {rows: Map<string, TimeRow>; leapSecond: LeapSecondState | null}) {
    if (rows.size === 0) return <p class="text-xs text-gray-400">No time data</p>;
    const sorted = [...rows.values()].sort((a, b) => a.nativeMsgID.localeCompare(b.nativeMsgID));

    return (
        <table class="text-xs font-mono tabular-nums w-full border-collapse">
            <thead>
                <tr class="text-left text-gray-500 dark:text-gray-400">
                    <th class={`${th}`}>Tag</th>
                    <th class={`${th}`}>Message</th>
                    <th class={`${th}`}>Local</th>
                    <th class={`${th}`}>UTC</th>
                    <th class={`${th}`}>TAI</th>
                    <th class={`${th}`}>Leap sec</th>
                    <th class={`${th}`}>TAcc</th>
                    <th class={`${th}`}>GNSS</th>
                </tr>
            </thead>
            <tbody>
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
                            <td class={td}>{row.tag}</td>
                            <td class={td}>{row.nativeMsgID}</td>
                            <td class={td}>{local}</td>
                            <td class={td}>{utcBold ? <N>{utc}</N> : utc}</td>
                            <td class={td}>{taiBold ? <N>{tai}</N> : tai}</td>
                            <td class={td}>{leapBold ? <N>{leapStr}</N> : leapStr}</td>
                            <td class={td}>{row.accuracy ? <N>{fmtNs(row.accuracy)}</N> : blank}</td>
                            <td class={td}>{row.gnss ? <N>{row.gnss}</N> : blank}</td>
                        </tr>
                    );
                })}
            </tbody>
        </table>
    );
}

// --- Main PVT panel ---

export function PVTPanel({posRows, velRows, timeRows, leapSecond}: Props) {
    return (
        <div class="space-y-4">
            <div>
                <h4 class="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Position</h4>
                <div class="overflow-x-auto">
                    <PositionTable rows={posRows} />
                </div>
            </div>
            <div>
                <h4 class="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Velocity</h4>
                <div class="overflow-x-auto">
                    <VelocityTable rows={velRows} />
                </div>
            </div>
            <div>
                <h4 class="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-1">Time</h4>
                <div class="overflow-x-auto">
                    <TimeTable rows={timeRows} leapSecond={leapSecond} />
                </div>
            </div>
        </div>
    );
}

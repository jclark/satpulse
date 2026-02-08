import {h, Fragment} from 'preact';
import {formatUTCLocal, formatTAI, formatDateTime} from './timefmt';
import type {TimeMsg, LeapSecondState} from './app';

interface Props {
    msg: TimeMsg | null;
    leapSecond: LeapSecondState | null;
}

// parseTAITime parses a ptime.Time string ("seconds.nanoseconds") to seconds.
function parseTAITime(t: string): number {
    const dot = t.indexOf('.');
    if (dot < 0) return parseInt(t, 10);
    return parseInt(t.substring(0, dot), 10);
}

// taiToUTC computes a UTC ISO string from TAI seconds and a leap second offset.
function taiToUTC(taiSecs: number, utcOff: number): string {
    const utcSecs = taiSecs - utcOff;
    return new Date(utcSecs * 1000).toISOString();
}

export function TimePanel({msg, leapSecond}: Props) {
    if (!msg) {
        return (
            <div class="p-4 overflow-y-auto h-full">
                <h2 class="text-base font-semibold mb-4">Time</h2>
                <p class="text-gray-500 dark:text-gray-400 italic text-sm">
                    Waiting for time data...
                </p>
            </div>
        );
    }

    const rows: [string, string | preact.ComponentChildren][] = [];

    // UTC from the message (direct from receiver)
    if (msg.utcTime) {
        const local = formatUTCLocal(msg.utcTime);
        if (local) {
            rows.push(['Local time', local.time]);
            rows.push(['Local date', local.date]);
        }
        rows.push(['UTC', formatDateTime(msg.utcTime)]);
    }

    // TAI-derived UTC (using leap second offset)
    if (msg.taiTime && leapSecond) {
        const taiSecs = parseTAITime(msg.taiTime);
        if (taiSecs > 0) {
            const utcStr = taiToUTC(taiSecs, leapSecond.utcOff);
            if (!msg.utcTime) {
                const local = formatUTCLocal(utcStr);
                if (local) {
                    rows.push(['Local time', local.time]);
                    rows.push(['Local date', local.date]);
                }
            }
            rows.push(['UTC (from TAI)', formatDateTime(utcStr)]);
        }
    }

    // TAI time
    if (msg.taiTime) {
        const taiSecs = parseTAITime(msg.taiTime);
        if (taiSecs > 0) {
            rows.push(['TAI', formatTAI(taiSecs)]);
        }
    }

    // Leap seconds
    if (leapSecond) {
        rows.push(['Leap seconds', String(leapSecond.utcOff)]);
    }

    // GNSS source
    if (msg.gnss) {
        rows.push(['Time source', msg.gnss]);
    }

    return (
        <div class="p-4 overflow-y-auto h-full">
            <h2 class="text-base font-semibold mb-4">Time</h2>
            {rows.length > 0 ? (
                <dl class="grid grid-cols-[120px_1fr] gap-x-4 gap-y-2 max-w-xl">
                    {rows.map(([label, value]) => (
                        <>
                            <dt class="text-gray-500 dark:text-gray-400 text-xs">{label}</dt>
                            <dd class="text-sm tabular-nums">{value}</dd>
                        </>
                    ))}
                </dl>
            ) : (
                <p class="text-gray-500 dark:text-gray-400 italic text-sm">
                    Waiting for time data...
                </p>
            )}
        </div>
    );
}

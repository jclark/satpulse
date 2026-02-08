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

const blank = '\u2014';

export function TimePanel({msg, leapSecond}: Props) {
    let localTime = blank, localDate = blank, utc = blank, utcFromTAI = '', tai = '', leapSecs = '', source = '';

    if (msg?.utcTime) {
        const local = formatUTCLocal(msg.utcTime);
        if (local) {
            localTime = local.time;
            localDate = local.date;
        }
        utc = formatDateTime(msg.utcTime);
    }

    if (msg?.taiTime && leapSecond) {
        const taiSecs = parseTAITime(msg.taiTime);
        if (taiSecs > 0) {
            const utcStr = taiToUTC(taiSecs, leapSecond.utcOff);
            if (!msg.utcTime) {
                const local = formatUTCLocal(utcStr);
                if (local) {
                    localTime = local.time;
                    localDate = local.date;
                }
            }
            utcFromTAI = formatDateTime(utcStr);
        }
    }

    if (msg?.taiTime) {
        const taiSecs = parseTAITime(msg.taiTime);
        if (taiSecs > 0) tai = formatTAI(taiSecs);
    }

    if (leapSecond) leapSecs = String(leapSecond.utcOff);
    if (msg?.gnss) source = msg.gnss;

    const rows: [string, string][] = [
        ['Local time', localTime],
        ['Local date', localDate],
        ['UTC', utc],
    ];
    if (utcFromTAI) rows.push(['UTC (from TAI)', utcFromTAI]);
    if (tai) rows.push(['TAI', tai]);
    rows.push(['Leap seconds', leapSecs || blank]);
    rows.push(['Time source', source || blank]);

    return (
        <dl class="grid grid-cols-[120px_1fr] gap-x-4 gap-y-2 max-w-xl">
            {rows.map(([label, value]) => (
                <>
                    <dt class="text-gray-500 dark:text-gray-400 text-xs">{label}</dt>
                    <dd class="text-sm tabular-nums">{value}</dd>
                </>
            ))}
        </dl>
    );
}

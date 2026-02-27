import {h} from 'preact';
import {formatUTCLocal, formatTAI, formatDateTime, parseTAITime, taiToUTC} from './timefmt';
import type {TimeMsg, LeapSecondState} from './app';
import {DefinitionList} from './ui';

interface Props {
    msg: TimeMsg | null;
    leapSecond: LeapSecondState | null;
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

    return <DefinitionList rows={rows.map(([label, value]) => ({label, value}))} class="max-w-xl grid-cols-[120px_1fr]" />;
}

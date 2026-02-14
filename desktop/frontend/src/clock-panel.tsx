import {h} from 'preact';
import {useMemo} from 'preact/hooks';
import {parseTAITime, taiToUTC} from './timefmt';
import type {TimeMsg, LeapSecondState} from './app';
import '@fontsource/dseg7-classic/700.css';

interface Props {
    msg: TimeMsg | null;
    leapSecond: LeapSecondState | null;
}

const W = 320;
const H = 256;
const FONT = "'DSEG7 Classic', monospace";

function formatUTCOffset(): string {
    const mins = new Date().getTimezoneOffset();
    const sign = mins <= 0 ? '+' : '-';
    const abs = Math.abs(mins);
    const h = String(Math.floor(abs / 60));
    const m = String(abs % 60).padStart(2, '0');
    return `${sign}${h}:${m}`;
}

export function ClockPanel({msg, leapSecond}: Props) {
    const time = useMemo(() => {
        if (!msg) return null;
        let d: Date | null = null;
        if (msg.taiTime && leapSecond) {
            const taiSecs = parseTAITime(msg.taiTime);
            if (taiSecs > 0) {
                const utcStr = taiToUTC(taiSecs, leapSecond.utcOff);
                d = new Date(utcStr);
            }
        }
        if (!d && msg.utcTime) {
            d = new Date(msg.utcTime);
        }
        if (!d) return null;
        const yyyy = String(d.getFullYear());
        const mm = String(d.getMonth() + 1).padStart(2, '0');
        const dd = String(d.getDate()).padStart(2, '0');
        return {
            date: `${yyyy}-${mm}-${dd}`,
            hm: String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0'),
            ss: String(d.getSeconds()).padStart(2, '0'),
        };
    }, [msg?.taiTime, msg?.utcTime, leapSecond?.utcOff]);

    const utcOffset = useMemo(() => formatUTCOffset(), []);

    if (!time) {
        return (
            <div
                class="relative bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-gray-400 dark:text-gray-500 text-sm select-none shrink-0"
                style={{width: W + 'px', height: H + 'px'}}
            >
                Waiting for time
            </div>
        );
    }

    // Ghost text: all-8s shows every segment dimly behind the real digits.
    const dateGhost = '8888-88-88';
    const hmGhost = '88:88';
    const ssGhost = '88';
    const tzGhost = utcOffset.replace(/[0-9]/g, '8');

    return (
        <div
            class="relative shrink-0 bg-gray-100 dark:bg-gray-800 flex items-center justify-center select-none"
            style={{width: W + 'px', height: H + 'px'}}
        >
            <div class="flex flex-col items-end" style={{gap: '10px'}}>
                {/* Date YYYY-MM-DD — scaled to match HH:MM width */}
                <div class="relative self-stretch text-center" style={{lineHeight: 1}}>
                    <span
                        class="text-gray-200 dark:text-gray-700/40"
                        style={{fontFamily: FONT, fontSize: '26px', fontWeight: 700, letterSpacing: '2px'}}
                    >
                        {dateGhost}
                    </span>
                    <span
                        class="absolute inset-0 text-gray-800 dark:text-green-400"
                        style={{fontFamily: FONT, fontSize: '26px', fontWeight: 700, letterSpacing: '2px'}}
                    >
                        {time.date}
                    </span>
                </div>
                {/* HH:MM large */}
                <div class="relative" style={{lineHeight: 1}}>
                    <span
                        class="text-gray-200 dark:text-gray-700/40"
                        style={{fontFamily: FONT, fontSize: '72px', fontWeight: 700}}
                    >
                        {hmGhost}
                    </span>
                    <span
                        class="absolute inset-0 text-gray-800 dark:text-green-400"
                        style={{fontFamily: FONT, fontSize: '72px', fontWeight: 700}}
                    >
                        {time.hm}
                    </span>
                </div>
                {/* SS + timezone offset on same line */}
                <div class="self-stretch flex justify-between items-baseline" style={{lineHeight: 1}}>
                    <div class="relative">
                        <span
                            class="text-gray-200 dark:text-gray-700/40"
                            style={{fontFamily: FONT, fontSize: '36px', fontWeight: 700}}
                        >
                            {tzGhost}
                        </span>
                        <span
                            class="absolute inset-0 text-gray-800 dark:text-green-400"
                            style={{fontFamily: FONT, fontSize: '36px', fontWeight: 700}}
                        >
                            {utcOffset}
                        </span>
                    </div>
                    <div class="relative">
                        <span
                            class="text-gray-200 dark:text-gray-700/40"
                            style={{fontFamily: FONT, fontSize: '36px', fontWeight: 700}}
                        >
                            {ssGhost}
                        </span>
                        <span
                            class="absolute inset-0 text-gray-800 dark:text-green-400"
                            style={{fontFamily: FONT, fontSize: '36px', fontWeight: 700}}
                        >
                            {time.ss}
                        </span>
                    </div>
                </div>
            </div>
        </div>
    );
}

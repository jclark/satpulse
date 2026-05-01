import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import type {TimeMsg} from './app';
import '@fontsource/dseg7-classic/700.css';

interface Props {
    msg: TimeMsg | null;
}

interface ClockTime {
    date: string;
    hm: string;
    ss: string;
}

const FONT = "'DSEG7 Classic', monospace";
// Sizes are expressed as a percentage of container height (cqh) so the
// clock scales fluidly. Proportions follow the reference design (HH:MM
// largest, then seconds/tz, then date), sized to fill ~95% of container.
const HM_SIZE = '44.5cqh';
const DATE_SIZE = '16.1cqh';
const SS_SIZE = '22.3cqh';
const GAP = '6.2cqh';
const HM_LETTER = '0';
const DATE_LETTER = '1.24cqh';

function formatUTCOffset(): string {
    const mins = new Date().getTimezoneOffset();
    const sign = mins <= 0 ? '+' : '-';
    const abs = Math.abs(mins);
    const h = String(Math.floor(abs / 60));
    const m = String(abs % 60).padStart(2, '0');
    return `${sign}${h}:${m}`;
}

export function ClockPanel({msg}: Props) {
    const [time, setTime] = useState<ClockTime | null>(null);
    useEffect(() => {
        if (!msg?.utcTime) return;
        const d = new Date(msg.utcTime);
        if (d.getMilliseconds() !== 0) return;
        const yyyy = String(d.getFullYear());
        const mm = String(d.getMonth() + 1).padStart(2, '0');
        const dd = String(d.getDate()).padStart(2, '0');
        setTime({
            date: `${yyyy}-${mm}-${dd}`,
            hm: String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0'),
            ss: String(d.getSeconds()).padStart(2, '0'),
        });
    }, [msg?.utcTime]);

    const utcOffset = useMemo(() => formatUTCOffset(), []);

    if (!time) {
        return (
            <div
                class="relative flex h-full w-full select-none items-center justify-start bg-surface-1 text-sm text-text-muted"
                style="container-type: size;"
            >
                Waiting for time
            </div>
        );
    }

    const dateGhost = '8888-88-88';
    const hmGhost = '88:88';
    const ssGhost = '88';
    const tzGhost = utcOffset.replace(/[0-9]/g, '8');

    return (
        <div
            class="relative flex h-full w-full select-none items-center justify-start bg-surface-1"
            style="container-type: size;"
        >
            <div class="flex flex-col items-start" style={{gap: GAP}}>
                {/* Date YYYY-MM-DD — scaled to match HH:MM width */}
                <div class="relative self-stretch text-center" style={{lineHeight: 1}}>
                    <span
                        class="text-clock-ghost"
                        style={{fontFamily: FONT, fontSize: DATE_SIZE, fontWeight: 700, letterSpacing: DATE_LETTER}}
                    >
                        {dateGhost}
                    </span>
                    <span
                        class="absolute inset-0 text-clock-digit"
                        style={{fontFamily: FONT, fontSize: DATE_SIZE, fontWeight: 700, letterSpacing: DATE_LETTER}}
                    >
                        {time.date}
                    </span>
                </div>
                {/* HH:MM large */}
                <div class="relative" style={{lineHeight: 1}}>
                    <span
                        class="text-clock-ghost"
                        style={{fontFamily: FONT, fontSize: HM_SIZE, fontWeight: 700, letterSpacing: HM_LETTER}}
                    >
                        {hmGhost}
                    </span>
                    <span
                        class="absolute inset-0 text-clock-digit"
                        style={{fontFamily: FONT, fontSize: HM_SIZE, fontWeight: 700, letterSpacing: HM_LETTER}}
                    >
                        {time.hm}
                    </span>
                </div>
                {/* SS + timezone offset on same line */}
                <div class="self-stretch flex justify-between items-baseline" style={{lineHeight: 1}}>
                    <div class="relative">
                        <span
                            class="text-clock-ghost"
                            style={{fontFamily: FONT, fontSize: SS_SIZE, fontWeight: 700}}
                        >
                            {tzGhost}
                        </span>
                        <span
                            class="absolute inset-0 text-clock-digit"
                            style={{fontFamily: FONT, fontSize: SS_SIZE, fontWeight: 700}}
                        >
                            {utcOffset}
                        </span>
                    </div>
                    <div class="relative">
                        <span
                            class="text-clock-ghost"
                            style={{fontFamily: FONT, fontSize: SS_SIZE, fontWeight: 700}}
                        >
                            {ssGhost}
                        </span>
                        <span
                            class="absolute inset-0 text-clock-digit"
                            style={{fontFamily: FONT, fontSize: SS_SIZE, fontWeight: 700}}
                        >
                            {time.ss}
                        </span>
                    </div>
                </div>
            </div>
        </div>
    );
}

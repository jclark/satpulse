import {h} from 'preact';
import {useState, useEffect, useRef, useMemo} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Button} from './ui';

interface MsgRow {
    msgType: string;
    count: number;
    firstTime: number;
    lastTime: number;
}

interface Props {
    connected: boolean;
}

const MSM_BASES: [number, string][] = [
    [1070, 'GPS'], [1080, 'GLONASS'], [1090, 'Galileo'], [1100, 'SBAS'],
    [1110, 'QZSS'], [1120, 'BeiDou'], [1130, 'NavIC'],
];

const NON_MSM: Record<string, string> = {
    '1005': 'Station ARP',
    '1006': 'Station ARP + height',
    '1007': 'Antenna descriptor',
    '1008': 'Antenna + serial',
    '1033': 'Receiver + antenna',
    '1230': 'GLONASS bias',
};

function rtcmDescription(msgType: string): string {
    const n = parseInt(msgType, 10);
    if (isNaN(n)) return '';
    for (const [base, name] of MSM_BASES) {
        const off = n - base;
        if (off >= 1 && off <= 7) return `${name} MSM${off}`;
    }
    return NON_MSM[msgType] || '';
}

function formatAge(ms: number): string {
    if (ms < 0) return '';
    return Math.floor(ms / 1000) + 's';
}

function formatPeriod(ms: number): string {
    if (ms < 0) return '';
    const s = Math.round(ms / 10) / 100; // round to 10ms
    return s + 's';
}

export function RtcmPanel({connected}: Props) {
    const rowsRef = useRef<Map<string, MsgRow>>(new Map());
    const [displayed, setDisplayed] = useState<Map<string, MsgRow>>(new Map());
    const [tick, setTick] = useState(0);

    // Listen for corrpacket events
    useEffect(() => {
        const off = EventsOn('gps:corrpacket', (evt: {msg: string}) => {
            const rows = rowsRef.current;
            const existing = rows.get(evt.msg);
            const now = Date.now();
            if (existing) {
                existing.count++;
                existing.lastTime = now;
            } else {
                rows.set(evt.msg, {msgType: evt.msg, count: 1, firstTime: now, lastTime: now});
            }
            setDisplayed(new Map(rows));
        });
        return () => {
            if (typeof off === 'function') off(); else EventsOff('gps:corrpacket');
        };
    }, []);

    // Clear on disconnect
    useEffect(() => {
        if (!connected) {
            rowsRef.current = new Map();
            setDisplayed(new Map());
        }
    }, [connected]);

    // 1-second tick for age/rate updates
    useEffect(() => {
        const id = setInterval(() => setTick(t => t + 1), 1000);
        return () => clearInterval(id);
    }, []);

    const handleClear = () => {
        rowsRef.current = new Map();
        setDisplayed(new Map());
    };

    const sortedRows = useMemo(() => {
        const arr = Array.from(displayed.values());
        arr.sort((a, b) => a.msgType.localeCompare(b.msgType, undefined, {numeric: true}));
        return arr;
    }, [displayed]);

    const now = Date.now();

    return (
        <div class="flex flex-1 flex-col text-xs">
            <div class="mx-3 mt-3 flex-2 overflow-y-auto rounded border border-border-subtle bg-surface-2">
                <table class="w-full border-collapse">
                    <thead class="sticky top-0 z-10 bg-surface-2">
                        <tr class="text-left text-text-secondary">
                            <th class="whitespace-nowrap px-2 py-1.5">Type</th>
                            <th class="whitespace-nowrap px-2 py-1.5">Description</th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Count</th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Age</th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Period</th>
                        </tr>
                    </thead>
                    <tbody class="font-mono">
                        {sortedRows.map(row => {
                            const age = now - row.lastTime;
                            const period = row.count >= 2 ? (row.lastTime - row.firstTime) / (row.count - 1) : -1;
                            const textClass = age < 10000 ? 'text-text-primary' : 'text-text-muted';
                            return (
                                <tr key={row.msgType} class={`hover:bg-surface-3 ${textClass}`}>
                                    <td class="whitespace-nowrap px-2 py-0.5">{row.msgType}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5">{rtcmDescription(row.msgType)}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{row.count}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{formatAge(age)}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{period >= 0 ? formatPeriod(period) : ''}</td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </div>
            <div class="mx-3 flex shrink-0 items-center gap-2 py-1.5">
                <div class="flex-1" />
                <Button onClick={handleClear}>Clear</Button>
            </div>
        </div>
    );
}

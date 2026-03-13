import {h} from 'preact';
import {useState, useEffect, useRef, useMemo} from 'preact/hooks';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Button} from './ui';
import {rtcmInfo} from './rtcm';

interface CorrPacketEvent {
    msg: string;
    epoch?: number;
    refstation?: number;
}

interface MsgRow {
    msgType: string;
    count: number;      // epochs for MSM, packets for non-MSM
    splits: number;     // extra packets beyond one-per-epoch (MSM only)
    lastEpoch: number;  // last epoch number seen (MSM only, -1 if none)
    station: number;    // last seen reference station ID, -1 if none
    lastTime: number;
}

interface Props {
    connected: boolean;
}

function formatAge(ms: number): string {
    if (ms < 0) return '';
    return Math.floor(ms / 1000) + 's';
}

export function RtcmPanel({connected}: Props) {
    const rowsRef = useRef<Map<string, MsgRow>>(new Map());
    const [displayed, setDisplayed] = useState<Map<string, MsgRow>>(new Map());
    const [tick, setTick] = useState(0);

    // Listen for corrpacket events
    useEffect(() => {
        const off = EventsOn('gps:corrpacket', (evt: CorrPacketEvent) => {
            const rows = rowsRef.current;
            const row = rows.get(evt.msg);
            const now = Date.now();
            if (evt.refstation != null && row) {
                row.station = evt.refstation;
            }
            if (evt.epoch != null) {
                // MSM packet: count by epoch
                if (row) {
                    if (evt.epoch !== row.lastEpoch) {
                        row.count++;
                        row.lastEpoch = evt.epoch;
                        row.lastTime = now;
                    } else {
                        row.splits++;
                    }
                } else {
                    rows.set(evt.msg, {
                        msgType: evt.msg, count: 1, splits: 0,
                        lastEpoch: evt.epoch, station: evt.refstation ?? -1,
                        lastTime: now,
                    });
                }
            } else {
                // Non-MSM: count every packet
                if (row) {
                    row.count++;
                    row.lastTime = now;
                } else {
                    rows.set(evt.msg, {
                        msgType: evt.msg, count: 1, splits: 0,
                        lastEpoch: -1, station: evt.refstation ?? -1,
                        lastTime: now,
                    });
                }
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

    // 1-second tick for age updates
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
                            <th class="whitespace-nowrap px-2 py-1.5">Station ID</th>
                            <th class="whitespace-nowrap px-2 py-1.5">Type</th>
                            <th class="whitespace-nowrap px-2 py-1.5">MSM</th>
                            <th class="w-full whitespace-nowrap px-2 py-1.5">Description</th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Count</th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Splits</th>
                            <th class="whitespace-nowrap px-2 py-1.5 text-right">Age</th>
                        </tr>
                    </thead>
                    <tbody class="font-mono">
                        {sortedRows.map(row => {
                            const age = now - row.lastTime;
                            const textClass = age < 10000 ? 'text-text-primary' : 'text-text-muted';
                            const info = rtcmInfo(row.msgType);
                            return (
                                <tr key={row.msgType} class={`hover:bg-surface-3 ${textClass}`}>
                                    <td class="whitespace-nowrap px-2 py-0.5 tabular-nums">{row.station >= 0 ? row.station : ''}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5">{row.msgType}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5">{info.msm}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5">{info.description}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{row.count}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{row.splits > 0 ? row.splits : ''}</td>
                                    <td class="whitespace-nowrap px-2 py-0.5 text-right tabular-nums">{formatAge(age)}</td>
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

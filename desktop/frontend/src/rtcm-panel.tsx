import {h} from 'preact';
import {useState, useEffect, useRef, useMemo} from 'preact/hooks';
import type {CorReportMsg} from '@satpulse/gps/gpsprot';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Button} from './ui';
import {rtcmInfo} from './rtcm';

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
    sessionSeq: number;
}

function formatAge(ms: number): string {
    if (ms < 0) return '';
    return Math.floor(ms / 1000) + 's';
}

export function RtcmPanel({connected, sessionSeq}: Props) {
    const rowsRef = useRef<Map<string, MsgRow>>(new Map());
    // Epoch counter reconstructed from finalFragment: incremented after
    // each packet that completes a logical message (finalFragment true).
    const epochRef = useRef(0);
    const [displayed, setDisplayed] = useState<Map<string, MsgRow>>(new Map());
    const [tick, setTick] = useState(0);

    // Listen for corrpacket events
    useEffect(() => {
        const off = EventsOn('gps:corrpacket', (evt: CorReportMsg) => {
            const rows = rowsRef.current;
            const row = rows.get(evt.msgID);
            const now = Date.now();
            if (evt.rtcmRefBaseID != null && row) {
                row.station = evt.rtcmRefBaseID;
            }
            if (evt.finalFragment != null) {
                // Fragmentable message (MSM): count by reconstructed epoch.
                const epoch = epochRef.current;
                if (evt.finalFragment) epochRef.current++;
                if (row) {
                    if (epoch !== row.lastEpoch) {
                        row.count++;
                        row.lastEpoch = epoch;
                        row.lastTime = now;
                    } else {
                        row.splits++;
                    }
                } else {
                    rows.set(evt.msgID, {
                        msgType: evt.msgID, count: 1, splits: 0,
                        lastEpoch: epoch, station: evt.rtcmRefBaseID ?? -1,
                        lastTime: now,
                    });
                }
            } else {
                // Non-fragmentable message: count every packet.
                if (row) {
                    row.count++;
                    row.lastTime = now;
                } else {
                    rows.set(evt.msgID, {
                        msgType: evt.msgID, count: 1, splits: 0,
                        lastEpoch: -1, station: evt.rtcmRefBaseID ?? -1,
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
            epochRef.current = 0;
            setDisplayed(new Map());
        }
    }, [connected]);

    // Clear when a new corrections session starts
    useEffect(() => {
        rowsRef.current = new Map();
        epochRef.current = 0;
        setDisplayed(new Map());
    }, [sessionSeq]);

    // 1-second tick for age updates
    useEffect(() => {
        const id = setInterval(() => setTick(t => t + 1), 1000);
        return () => clearInterval(id);
    }, []);

    const handleClear = () => {
        rowsRef.current = new Map();
        epochRef.current = 0;
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

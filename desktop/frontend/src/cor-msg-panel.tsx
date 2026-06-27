import {h} from 'preact';
import {useState, useEffect, useRef, useMemo} from 'preact/hooks';
import type {CorReportMsg} from '@satpulse/gps/gpsprot';
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import {Button, Badge} from './ui';
import {rtcmInfo} from './rtcm';
import {spartnInfo} from './spartn';

interface MsgRow {
    tag: string;        // protocol tag: "RTCM" or "SPARTN"
    msgType: string;    // msgID verbatim ("1074" for RTCM, "0.0" for SPARTN)
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

// rowInfo returns the description and MSM label for a row, dispatching on its
// protocol tag. SPARTN messages have no MSM concept, so msm is always empty.
function rowInfo(row: MsgRow): {description: string; msm: string} {
    if (row.tag === 'RTCM') return rtcmInfo(row.msgType);
    return {description: spartnInfo(row.msgType).description, msm: ''};
}

function formatAge(ms: number): string {
    if (ms < 0) return '';
    return Math.floor(ms / 1000) + 's';
}

export function CorMsgPanel({connected, sessionSeq}: Props) {
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
                        tag: evt.tag, msgType: evt.msgID, count: 1, splits: 0,
                        lastEpoch: epoch, station: evt.rtcmRefBaseID ?? -1,
                        lastTime: now,
                    });
                }
            } else {
                // Non-fragmentable message (non-MSM RTCM, all SPARTN): count
                // every packet.
                if (row) {
                    row.count++;
                    row.lastTime = now;
                } else {
                    rows.set(evt.msgID, {
                        tag: evt.tag, msgType: evt.msgID, count: 1, splits: 0,
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

    // Sort by protocol ("RTCM" before "SPARTN") then by message number.
    const sortedRows = useMemo(() => {
        const arr = Array.from(displayed.values());
        arr.sort((a, b) => {
            if (a.tag !== b.tag) return a.tag < b.tag ? -1 : 1;
            return a.msgType.localeCompare(b.msgType, undefined, {numeric: true});
        });
        return arr;
    }, [displayed]);

    const now = Date.now();

    return (
        <div class="flex flex-1 flex-col text-xs">
            <div class="mx-3 mt-3 flex-2 overflow-y-auto rounded border border-border-subtle bg-surface-2">
                <table class="w-full border-collapse">
                    <thead class="sticky top-0 z-10 bg-surface-2">
                        <tr class="text-left text-text-secondary">
                            <th class="px-2 py-1.5"></th>
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
                            const info = rowInfo(row);
                            return (
                                <tr key={row.msgType} class={`hover:bg-surface-3 ${textClass}`}>
                                    <td class="px-2 py-0.5"><Badge>{row.tag === 'RTCM' ? 'R' : 'S'}</Badge></td>
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

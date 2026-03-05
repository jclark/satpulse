import {h} from 'preact';
import {MonitorDataView, type MonitorDataRow} from './ui';
export type {NavEpochMsg} from '@satpulse/gps/gpsprot';
import type {NavEpochMsg} from '@satpulse/gps/gpsprot';

interface Props {
    msg: NavEpochMsg | null;
}

function fmtSignals(sigs: Record<string, string[]>): string {
    const parts: string[] = [];
    for (const [gnss, names] of Object.entries(sigs)) {
        if (names.length > 0) parts.push(`${gnss}: ${names.join(', ')}`);
    }
    return parts.join('; ');
}

export function StatusPanel({msg}: Props) {
    if (!msg) return null;
    const rows: MonitorDataRow[] = [];
    const add = (label: string, value: string | undefined) => {
        if (value != null) rows.push({label, value});
    };
    add('Fix level', msg.fixLevel);
    add('Solution dimensionality', msg.solutionDim);
    if (msg.correction) add('Corrections', msg.correction.length ? msg.correction.join(', ') : 'None');
    if (msg.auxSrc) add('Aux sources', msg.auxSrc.length ? msg.auxSrc.join(', ') : 'None');
    add('Horizontal accuracy', msg.acc?.hor != null ? `${msg.acc.hor.toFixed(3)} m` : undefined);
    add('Vertical accuracy', msg.acc?.vert != null ? `${msg.acc.vert.toFixed(3)} m` : undefined);
    add('Position accuracy', msg.acc?.pos != null ? `${msg.acc.pos.toFixed(3)} m` : undefined);
    add('Speed accuracy', msg.acc?.speed != null ? `${msg.acc.speed.toFixed(3)} m/s` : undefined);
    add('Ground speed accuracy', msg.acc?.groundSpeed != null ? `${msg.acc.groundSpeed.toFixed(3)} m/s` : undefined);
    add('Course accuracy', msg.acc?.course != null ? `${msg.acc.course.toFixed(1)}\u00b0` : undefined);
    add('Horizontal DOP', msg.dop?.hor != null ? msg.dop.hor.toFixed(1) : undefined);
    add('Vertical DOP', msg.dop?.vert != null ? msg.dop.vert.toFixed(1) : undefined);
    add('Position DOP', msg.dop?.pos != null ? msg.dop.pos.toFixed(1) : undefined);
    add('Geometric DOP', msg.dop?.geom != null ? msg.dop.geom.toFixed(1) : undefined);
    add('Time DOP', msg.dop?.time != null ? msg.dop.time.toFixed(1) : undefined);
    add('Satellites used', msg.numSVUsed != null ? String(msg.numSVUsed) : undefined);
    add('Satellites tracked', msg.numSVTracked != null ? String(msg.numSVTracked) : undefined);
    add('Satellites in view', msg.numSVInView != null ? String(msg.numSVInView) : undefined);
    add('Diff age', msg.diffAge != null ? `${msg.diffAge.toFixed(1)} s` : undefined);
    add('RTCM base ID', msg.rtcmRefBaseID != null ? String(msg.rtcmRefBaseID) : undefined);
    if (msg.signalsUsed) {
        const s = fmtSignals(msg.signalsUsed);
        if (s) add('Signals used', s);
    }
    return <MonitorDataView rows={rows} class="max-w-xl grid-cols-[140px_1fr]" />;
}

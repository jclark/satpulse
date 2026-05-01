import {h} from 'preact';
import type {NavEpochMsg} from '@satpulse/gps/gpsprot';

interface Props {
    msg: NavEpochMsg | null;
}

export function SummaryPanel({msg}: Props) {
    if (!msg) {
        return (
            <div class="flex-1 min-w-0 bg-surface-1 flex flex-col gap-1 tabular-nums text-text-muted">
                Waiting for fix
            </div>
        );
    }

    const fixParts = [msg.fixLevel, msg.solutionDim, ...(msg.auxSrc || []), ...(msg.correction || [])]
        .filter(Boolean);
    const fixLine = fixParts.length > 0 ? `Fix: ${fixParts.join(' ')}` : '';

    const svCount = msg.numSVUsed != null && msg.numSVTracked != null
        ? `${msg.numSVUsed}/${msg.numSVTracked}`
        : '';
    const gnssParts = [
        svCount,
        (msg.gnssUsed || []).join(' '),
        (msg.bandsUsed || []).join(' '),
    ].filter(s => s.length > 0);
    const gnssLine = gnssParts.length > 0 ? `Sats: ${gnssParts.join('; ')}` : '';

    const dop = msg.dop || {};
    const dopParts: string[] = [];
    if (dop.geom != null) dopParts.push(`G${dop.geom.toFixed(1)}`);
    if (dop.pos != null) dopParts.push(`P${dop.pos.toFixed(1)}`);
    if (dop.time != null) dopParts.push(`T${dop.time.toFixed(1)}`);
    if (dop.hor != null) dopParts.push(`H${dop.hor.toFixed(1)}`);
    if (dop.vert != null) dopParts.push(`V${dop.vert.toFixed(1)}`);
    if (dop.north != null) dopParts.push(`N${dop.north.toFixed(1)}`);
    if (dop.east != null) dopParts.push(`E${dop.east.toFixed(1)}`);
    const dopLine = dopParts.length > 0 ? `DOP: ${dopParts.join(' ')}` : '';

    const acc = msg.acc || {};
    const accParts: string[] = [];
    if (acc.hor != null) accParts.push(`H${acc.hor.toFixed(3)}`);
    if (acc.vert != null) accParts.push(`V${acc.vert.toFixed(3)}`);
    if (acc.speed != null) accParts.push(`S${acc.speed.toFixed(3)}`);
    const accLine = accParts.length > 0 ? `Acc: ${accParts.join(' ')}` : '';

    return (
        <div
            class="flex-1 min-w-0 bg-surface-1 flex flex-col gap-1 tabular-nums text-text-primary"
            style="padding-left: 1em; text-indent: -1em;"
        >
            {fixLine && <div>{fixLine}</div>}
            {gnssLine && <div>{gnssLine}</div>}
            {dopLine && <div>{dopLine}</div>}
            {accLine && <div>{accLine}</div>}
        </div>
    );
}

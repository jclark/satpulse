import {NMEAMsgNames, NMEAMsgOther} from '@satpulse/gps/configtarget';
import type {NMEAMsgFlag} from '@satpulse/gps/configtarget';

export * from '@satpulse/gps/configtarget';

// The UI offers a checkbox per named sentence; NMEAMsgOther is not one of them,
// it is appended to the wire value to retain whatever else the receiver sends.
export type NMEASelectableMsgFlag = Exclude<NMEAMsgFlag, typeof NMEAMsgOther>;
export const NMEASelectableMsgNames = NMEAMsgNames.filter(
    (name): name is NMEASelectableMsgFlag => name !== NMEAMsgOther);

export function toggleMsgFlag<T>(flags: ReadonlySet<T>, flag: T, on: boolean): ReadonlySet<T> {
    const next = new Set(flags);
    if (on) next.add(flag); else next.delete(flag);
    return next;
}

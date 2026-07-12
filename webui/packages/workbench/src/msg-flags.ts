import {NMEAMsgNames, NMEAMsgOther} from '@satpulse/gps/configtarget';

export * from '@satpulse/gps/configtarget';

export const NMEASelectableMsgNames = NMEAMsgNames.filter(name => name !== NMEAMsgOther);
export type NMEASelectableMsgFlag = typeof NMEASelectableMsgNames[number];

export function toggleMsgFlag<T>(flags: ReadonlySet<T>, flag: T, on: boolean): ReadonlySet<T> {
    const next = new Set(flags);
    if (on) next.add(flag); else next.delete(flag);
    return next;
}

import {NMEAMsgNames, NMEAMsgOther} from '@satpulse/gps/configtarget';

export * from '@satpulse/gps/configtarget';

export const NMEASelectableMsgNames = NMEAMsgNames.filter(name => name !== NMEAMsgOther);

export function msgFlag<T extends string>(name: NoInfer<T>, names: readonly T[]): number {
    const i = names.indexOf(name);
    if (i < 0) throw new Error(`unknown message flag name: ${name}`);
    return 1 << i;
}

export function msgFlagNames<T extends string>(flags: number, names: readonly T[]): T[] {
    return names.filter((name, i) => (flags & (1 << i)) !== 0);
}

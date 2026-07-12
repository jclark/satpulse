import {h} from 'preact';
import {NMEAMsgRMC, NMEAMsgGGA, NMEAMsgGSA, NMEAMsgGSV, NMEAMsgZDA, NMEAMsgVTG, NMEAMsgGLL, NMEAMsgOther, NMEASelectableMsgNames, toggleMsgFlag} from './msg-flags';
import type {NMEASelectableMsgFlag} from './msg-flags';
import type {NMEAMsgFlags} from '@satpulse/gps/configtarget';
import {ConfigSubGroup, labeledControlText} from './ui';

const nmeaMsgs: {name: typeof NMEASelectableMsgNames[number]; label: string}[] = [
    {name: NMEAMsgGGA, label: 'GGA'},
    {name: NMEAMsgGLL, label: 'GLL'},
    {name: NMEAMsgGSA, label: 'GSA'},
    {name: NMEAMsgGSV, label: 'GSV'},
    {name: NMEAMsgRMC, label: 'RMC'},
    {name: NMEAMsgVTG, label: 'VTG'},
    {name: NMEAMsgZDA, label: 'ZDA'},
];

interface Props {
    change: boolean;
    disableProtocol: boolean;
    flags: ReadonlySet<NMEASelectableMsgFlag>;
    onChangeChange: (v: boolean) => void;
    onDisableChange: (v: boolean) => void;
    onFlagsChange: (f: ReadonlySet<NMEASelectableMsgFlag>) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined if change is false (skip). */
export function nmeaWireValue(change: boolean, disableProtocol: boolean, flags: ReadonlySet<NMEASelectableMsgFlag>): NMEAMsgFlags | undefined {
    if (!change) return undefined;
    if (disableProtocol) return [];
    return [...NMEASelectableMsgNames.filter(name => flags.has(name)), NMEAMsgOther];
}

export function NMEAGroup({change, disableProtocol, flags, onChangeChange, onDisableChange, onFlagsChange, disabled}: Props) {
    const childDisabled = disabled || !change || disableProtocol;
    const disableDisabled = disabled || !change;
    return (
        <ConfigSubGroup title="NMEA">
            <div class="flex flex-col gap-1.5">
                <div class="flex gap-x-4">
                    <label class={`flex items-center gap-1.5 ${labeledControlText(!!disabled)}`}>
                        <input type="checkbox" class="accent-accent" checked={change} disabled={disabled}
                            onChange={e => onChangeChange((e.target as HTMLInputElement).checked)} />
                        Change
                    </label>
                    <label class={`flex items-center gap-1.5 ${labeledControlText(disableDisabled)}`}>
                        <input type="checkbox" class="accent-accent" checked={disableProtocol} disabled={disableDisabled}
                            onChange={e => onDisableChange((e.target as HTMLInputElement).checked)} />
                        Disable protocol
                    </label>
                </div>
                <div class="flex flex-wrap gap-x-4 gap-y-1">
                    {nmeaMsgs.map(m => {
                        return <label key={m.name} class={`flex items-center gap-1.5 ${labeledControlText(childDisabled)}`}>
                            <input
                                type="checkbox"
                                class="accent-accent"
                                checked={flags.has(m.name)}
                                disabled={childDisabled}
                                onChange={e => onFlagsChange(toggleMsgFlag(flags, m.name, (e.target as HTMLInputElement).checked))}
                            />
                            {m.label}
                        </label>;
                    })}
                </div>
            </div>
        </ConfigSubGroup>
    );
}

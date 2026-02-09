import {h} from 'preact';
import {NMEAMsgRMC, NMEAMsgGGA, NMEAMsgGSA, NMEAMsgGSV, NMEAMsgZDA, NMEAMsgVTG, NMEAMsgOther} from './msg-flags';

const nmeaMsgs: {flag: number; label: string}[] = [
    {flag: NMEAMsgRMC, label: 'RMC'},
    {flag: NMEAMsgGGA, label: 'GGA'},
    {flag: NMEAMsgGSA, label: 'GSA'},
    {flag: NMEAMsgGSV, label: 'GSV'},
    {flag: NMEAMsgZDA, label: 'ZDA'},
    {flag: NMEAMsgVTG, label: 'VTG'},
];

interface Props {
    change: boolean;
    disableProtocol: boolean;
    flags: number;
    onChangeChange: (v: boolean) => void;
    onDisableChange: (v: boolean) => void;
    onFlagsChange: (f: number) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined if change is false (skip). */
export function nmeaWireValue(change: boolean, disableProtocol: boolean, flags: number): number | undefined {
    if (!change) return undefined;
    if (disableProtocol) return 0;
    return flags | NMEAMsgOther;
}

export function NMEAGroup({change, disableProtocol, flags, onChangeChange, onDisableChange, onFlagsChange, disabled}: Props) {
    const childDisabled = disabled || !change || disableProtocol;
    const disableDisabled = disabled || !change;
    return (
        <div>
            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">NMEA</div>
            <div class="flex flex-col gap-1.5 ml-0.5">
                <div class="flex gap-x-4">
                    <label class={`flex items-center gap-1.5 text-xs ${disabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                        <input type="checkbox" class="accent-blue-600" checked={change} disabled={disabled}
                            onChange={e => onChangeChange((e.target as HTMLInputElement).checked)} />
                        Change
                    </label>
                    <label class={`flex items-center gap-1.5 text-xs ${disableDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                        <input type="checkbox" class="accent-blue-600" checked={disableProtocol} disabled={disableDisabled}
                            onChange={e => onDisableChange((e.target as HTMLInputElement).checked)} />
                        Disable protocol
                    </label>
                </div>
                <div class="flex flex-wrap gap-x-4 gap-y-1">
                    {nmeaMsgs.map(m => (
                        <label key={m.flag} class={`flex items-center gap-1.5 text-xs ${childDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                            <input
                                type="checkbox"
                                class="accent-blue-600"
                                checked={(flags & m.flag) !== 0}
                                disabled={childDisabled}
                                onChange={() => onFlagsChange(flags ^ m.flag)}
                            />
                            {m.label}
                        </label>
                    ))}
                </div>
            </div>
        </div>
    );
}

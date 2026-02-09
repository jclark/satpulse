import {h} from 'preact';
import {ThreeWaySelector, type ThreeWayState} from './three-way-selector';
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
    state: ThreeWayState;
    flags: number;
    onStateChange: (s: ThreeWayState) => void;
    onFlagsChange: (f: number) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined if state is 'skip'. */
export function nmeaWireValue(state: ThreeWayState, flags: number): number | undefined {
    if (state === 'skip') return undefined;
    if (state === 'disable') return 0;
    return flags | NMEAMsgOther;
}

export function NMEAGroup({state, flags, onStateChange, onFlagsChange, disabled}: Props) {
    const childDisabled = disabled || state !== 'configure';
    return (
        <div>
            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">NMEA</div>
            <ThreeWaySelector name="nmea-mode" state={state} onChange={onStateChange} disabled={disabled} />
            <div class="flex flex-wrap gap-x-4 gap-y-1 mt-1.5 ml-0.5">
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
    );
}

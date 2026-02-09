import {h} from 'preact';
import {RawMsgObs, RawMsgNavData} from './msg-flags';

interface Props {
    change: boolean;
    flags: number;
    onChangeChange: (v: boolean) => void;
    onFlagsChange: (f: number) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined when not configured (change=false). */
export function rawWireValue(change: boolean, flags: number): number | undefined {
    if (!change) return undefined;
    return flags;
}

function Checkbox({label, checked, disabled, onChange}: {
    label: string; checked: boolean; disabled: boolean;
    onChange: (v: boolean) => void;
}) {
    return (
        <label class={`flex items-center gap-1.5 text-xs ${disabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
            <input
                type="checkbox"
                class="accent-blue-600"
                checked={checked}
                disabled={disabled}
                onChange={e => onChange((e.target as HTMLInputElement).checked)}
            />
            {label}
        </label>
    );
}

export function RawGroup({change, flags, onChangeChange, onFlagsChange, disabled}: Props) {
    const childDisabled = disabled || !change;
    const toggle = (flag: number, on: boolean) => onFlagsChange(on ? flags | flag : flags & ~flag);
    const has = (flag: number) => (flags & flag) !== 0;

    return (
        <div>
            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">Raw</div>
            <div class="flex flex-col gap-1.5 ml-0.5">
                <Checkbox label="Change" checked={change} disabled={!!disabled}
                    onChange={onChangeChange} />
                <div class="flex flex-wrap gap-x-4 gap-y-1">
                    <Checkbox label="Observations (RINEX .obs)" checked={has(RawMsgObs)} disabled={childDisabled}
                        onChange={v => toggle(RawMsgObs, v)} />
                    <Checkbox label="Navigation data (RINEX .nav)" checked={has(RawMsgNavData)} disabled={childDisabled}
                        onChange={v => toggle(RawMsgNavData, v)} />
                </div>
            </div>
        </div>
    );
}

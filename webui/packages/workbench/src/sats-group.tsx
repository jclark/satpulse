import {h} from 'preact';
import {SatsMsgSat, SatsMsgSignal, SatsMsgNames, msgFlag, msgFlagNames} from './msg-flags';
import type {SatsMsgFlag, SatsMsgFlags} from '@satpulse/gps/configtarget';
import {ConfigSubGroup, labeledControlText} from './ui';

interface Props {
    change: boolean;
    flags: number;
    onChangeChange: (v: boolean) => void;
    onFlagsChange: (f: number) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined when not configured (change=false). */
export function satsWireValue(change: boolean, flags: number): SatsMsgFlags | undefined {
    if (!change) return undefined;
    return msgFlagNames(flags, SatsMsgNames);
}

export const satsFlag = (name: SatsMsgFlag) => msgFlag(name, SatsMsgNames);

function Checkbox({label, checked, disabled, onChange}: {
    label: string; checked: boolean; disabled: boolean;
    onChange: (v: boolean) => void;
}) {
    return (
        <label class={`flex items-center gap-1.5 ${labeledControlText(disabled)}`}>
            <input
                type="checkbox"
                class="accent-accent"
                checked={checked}
                disabled={disabled}
                onChange={e => onChange((e.target as HTMLInputElement).checked)}
            />
            {label}
        </label>
    );
}

export function SatsGroup({change, flags, onChangeChange, onFlagsChange, disabled}: Props) {
    const childDisabled = disabled || !change;
    const toggle = (flag: number, on: boolean) => onFlagsChange(on ? flags | flag : flags & ~flag);
    const has = (flag: number) => (flags & flag) !== 0;

    return (
        <ConfigSubGroup title="Satellites">
            <div class="flex flex-col gap-1.5">
                <Checkbox label="Change" checked={change} disabled={!!disabled}
                    onChange={onChangeChange} />
                <div class="flex flex-wrap gap-x-4 gap-y-1">
                    <Checkbox label="Satellite positions" checked={has(satsFlag(SatsMsgSat))} disabled={childDisabled}
                        onChange={v => toggle(satsFlag(SatsMsgSat), v)} />
                    <Checkbox label="Signals" checked={has(satsFlag(SatsMsgSignal))} disabled={childDisabled}
                        onChange={v => toggle(satsFlag(SatsMsgSignal), v)} />
                </div>
            </div>
        </ConfigSubGroup>
    );
}

import {h} from 'preact';
import {RawMsgObs, RawMsgNavData, RawMsgNames, toggleMsgFlag} from './msg-flags';
import type {RawMsgFlag, RawMsgFlags} from '@satpulse/gps/configtarget';
import {ConfigSubGroup, labeledControlText} from './ui';

interface Props {
    change: boolean;
    flags: ReadonlySet<RawMsgFlag>;
    onChangeChange: (v: boolean) => void;
    onFlagsChange: (f: ReadonlySet<RawMsgFlag>) => void;
    disabled?: boolean;
    // notSupported greys the whole subgroup (muted title) when the receiver has
    // no raw output capability.
    notSupported?: boolean;
}

/** Compute the wire value for Apply. Returns undefined when not configured (change=false). */
export function rawWireValue(change: boolean, flags: ReadonlySet<RawMsgFlag>): RawMsgFlags | undefined {
    if (!change) return undefined;
    return RawMsgNames.filter(name => flags.has(name));
}

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

export function RawGroup({change, flags, onChangeChange, onFlagsChange, disabled, notSupported}: Props) {
    const groupDisabled = disabled || notSupported;
    const childDisabled = groupDisabled || !change;
    const toggle = (flag: RawMsgFlag, on: boolean) => onFlagsChange(toggleMsgFlag(flags, flag, on));

    return (
        <ConfigSubGroup title="Raw" disabled={notSupported}>
            <div class="flex flex-col gap-1.5">
                <Checkbox label="Change" checked={change} disabled={!!groupDisabled}
                    onChange={onChangeChange} />
                <div class="flex flex-wrap gap-x-4 gap-y-1">
                    <Checkbox label="Observations (RINEX .obs)" checked={flags.has(RawMsgObs)} disabled={childDisabled}
                        onChange={v => toggle(RawMsgObs, v)} />
                    <Checkbox label="Navigation data (RINEX .nav)" checked={flags.has(RawMsgNavData)} disabled={childDisabled}
                        onChange={v => toggle(RawMsgNavData, v)} />
                </div>
            </div>
        </ConfigSubGroup>
    );
}

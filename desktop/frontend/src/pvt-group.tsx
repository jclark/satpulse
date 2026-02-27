import {h} from 'preact';
import {
    PVTMsgPos, PVTMsgVel, PVTMsgTime, PVTMsgTimePulse,
    PVTMsgLeapSecond, PVTMsgTAI, PVTMsgECEF,
    PVTMsgTimePulseAfter, PVTMsgQuality, PVTMsgEpoch, PVTMsgOff,
} from './msg-flags';
import {ConfigSubGroup, labeledControlText} from './ui';

interface Props {
    change: boolean;
    flags: number;
    onChangeChange: (v: boolean) => void;
    onFlagsChange: (f: number) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined when not configured (change=false). */
export function pvtWireValue(change: boolean, flags: number): number | undefined {
    if (!change) return undefined;
    return flags;
}

function Checkbox({label, checked, disabled, onChange, indent}: {
    label: string; checked: boolean; disabled: boolean;
    onChange: (v: boolean) => void; indent?: boolean;
}) {
    return (
        <label class={`flex items-center gap-1.5 ${indent ? 'ml-4' : ''} ${labeledControlText(disabled)}`}>
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

export function PVTGroup({change, flags, onChangeChange, onFlagsChange, disabled}: Props) {
    const childDisabled = disabled || !change;
    const toggle = (flag: number, on: boolean) => onFlagsChange(on ? flags | flag : flags & ~flag);
    const has = (flag: number) => (flags & flag) !== 0;

    const hasTime = has(PVTMsgTime) || has(PVTMsgTimePulse);
    const hasPosVel = has(PVTMsgPos) || has(PVTMsgVel);

    return (
        <ConfigSubGroup title="PVT">
            <div class="flex flex-col gap-1.5 ml-0.5">
                <div class="flex gap-x-4">
                    <Checkbox label="Change" checked={change} disabled={!!disabled}
                        onChange={onChangeChange} />
                    <Checkbox label="Turn off unselected" checked={has(PVTMsgOff)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgOff, v)} />
                </div>
                {/* Time subgroup */}
                <div class="grid grid-cols-[13rem_auto] gap-x-6 gap-y-1 items-start">
                    <div class={`col-span-2 text-[10px] font-medium uppercase tracking-wide ${childDisabled ? 'text-text-muted' : 'text-text-secondary'}`}>Time</div>
                    <Checkbox label="Navigation time" checked={has(PVTMsgTime)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgTime, v)} />
                    <Checkbox label="Prefer TAI" checked={has(PVTMsgTAI)}
                        disabled={childDisabled || !hasTime}
                        onChange={v => toggle(PVTMsgTAI, v)} />
                    <Checkbox label="Time-pulse time" checked={has(PVTMsgTimePulse)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgTimePulse, v)} />
                    <div />
                    <Checkbox label="Ensure message after pulse" checked={has(PVTMsgTimePulseAfter)}
                        disabled={childDisabled || !has(PVTMsgTimePulse)}
                        onChange={v => toggle(PVTMsgTimePulseAfter, v)} indent />
                    <div />
                    <Checkbox label="Leap second" checked={has(PVTMsgLeapSecond)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgLeapSecond, v)} />
                    <div />
                </div>
                {/* Position & velocity subgroup */}
                <div class="grid grid-cols-[13rem_auto] gap-x-6 gap-y-1 items-start">
                    <div class={`col-span-2 text-[10px] font-medium uppercase tracking-wide ${childDisabled ? 'text-text-muted' : 'text-text-secondary'}`}>Position & velocity</div>
                    <Checkbox label="Position" checked={has(PVTMsgPos)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgPos, v)} />
                    <Checkbox label="Prefer ECEF" checked={has(PVTMsgECEF)}
                        disabled={childDisabled || !hasPosVel}
                        onChange={v => toggle(PVTMsgECEF, v)} />
                    <Checkbox label="Velocity" checked={has(PVTMsgVel)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgVel, v)} />
                    <div />
                    <Checkbox label="Solution quality" checked={has(PVTMsgQuality)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgQuality, v)} />
                    <div />
                    <Checkbox label="End of navigation epoch" checked={has(PVTMsgEpoch)} disabled={childDisabled}
                        onChange={v => toggle(PVTMsgEpoch, v)} />
                    <div />
                </div>
            </div>
        </ConfigSubGroup>
    );
}

import {h} from 'preact';
import {
    PVTMsgPos, PVTMsgVel, PVTMsgTime, PVTMsgTimePulse,
    PVTMsgLeapSecond, PVTMsgTAI, PVTMsgECEF,
    PVTMsgTimePulseAfter, PVTMsgQuality, PVTMsgEpoch, PVTMsgOff,
    PVTMsgNames, msgFlag, msgFlagNames,
} from './msg-flags';
import type {PVTMsgFlag, PVTMsgFlags} from '@satpulse/gps/configtarget';
import {ConfigSubGroup, ConfigSubSubGroup, labeledControlText} from './ui';

interface Props {
    change: boolean;
    flags: number;
    onChangeChange: (v: boolean) => void;
    onFlagsChange: (f: number) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined when not configured (change=false). */
export function pvtWireValue(change: boolean, flags: number): PVTMsgFlags | undefined {
    if (!change) return undefined;
    return msgFlagNames(flags, PVTMsgNames);
}

export const pvtFlag = (name: PVTMsgFlag) => msgFlag(name, PVTMsgNames);

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

    const hasTime = has(pvtFlag(PVTMsgTime)) || has(pvtFlag(PVTMsgTimePulse));
    const hasPosVel = has(pvtFlag(PVTMsgPos)) || has(pvtFlag(PVTMsgVel));

    return (
        <ConfigSubGroup title="PVT">
            <div class="flex flex-col gap-1.5">
                <div class="flex gap-x-4">
                    <Checkbox label="Change" checked={change} disabled={!!disabled}
                        onChange={onChangeChange} />
                    <Checkbox label="Turn off unselected" checked={has(pvtFlag(PVTMsgOff))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgOff), v)} />
                </div>
                <ConfigSubSubGroup title="Time" disabled={childDisabled}>
                    <Checkbox label="Navigation time" checked={has(pvtFlag(PVTMsgTime))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgTime), v)} />
                    <Checkbox label="Prefer TAI" checked={has(pvtFlag(PVTMsgTAI))}
                        disabled={childDisabled || !hasTime}
                        onChange={v => toggle(pvtFlag(PVTMsgTAI), v)} />
                    <Checkbox label="Time-pulse time" checked={has(pvtFlag(PVTMsgTimePulse))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgTimePulse), v)} />
                    <div />
                    <Checkbox label="Ensure message after pulse" checked={has(pvtFlag(PVTMsgTimePulseAfter))}
                        disabled={childDisabled || !has(pvtFlag(PVTMsgTimePulse))}
                        onChange={v => toggle(pvtFlag(PVTMsgTimePulseAfter), v)} indent />
                    <div />
                    <Checkbox label="Leap second" checked={has(pvtFlag(PVTMsgLeapSecond))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgLeapSecond), v)} />
                    <div />
                </ConfigSubSubGroup>
                <ConfigSubSubGroup title="Position and velocity" disabled={childDisabled}>
                    <Checkbox label="Position" checked={has(pvtFlag(PVTMsgPos))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgPos), v)} />
                    <Checkbox label="Prefer ECEF" checked={has(pvtFlag(PVTMsgECEF))}
                        disabled={childDisabled || !hasPosVel}
                        onChange={v => toggle(pvtFlag(PVTMsgECEF), v)} />
                    <Checkbox label="Velocity" checked={has(pvtFlag(PVTMsgVel))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgVel), v)} />
                    <div />
                </ConfigSubSubGroup>
                <ConfigSubSubGroup title="Navigation epoch" disabled={childDisabled}>
                    <Checkbox label="Solution quality" checked={has(pvtFlag(PVTMsgQuality))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgQuality), v)} />
                    <div />
                    <Checkbox label="End of epoch" checked={has(pvtFlag(PVTMsgEpoch))} disabled={childDisabled}
                        onChange={v => toggle(pvtFlag(PVTMsgEpoch), v)} />
                    <div />
                </ConfigSubSubGroup>
            </div>
        </ConfigSubGroup>
    );
}

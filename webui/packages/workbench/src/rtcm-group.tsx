import {h} from 'preact';
import {RTCMMsgMSM4, RTCMMsgMSM7, RTCMMsgARP, RTCMMsgLax, RTCMMsgOther} from './msg-flags';
import type {RTCMMsgFlags} from '@satpulse/gps/configtarget';
import {ConfigSubGroup, labeledControlText} from './ui';

type MSMType = 'none' | 'msm4' | 'msm7';

interface Props {
    change: boolean;
    disableProtocol: boolean;
    msm: MSMType;
    fallback: boolean;
    arp: boolean;
    onChangeChange: (v: boolean) => void;
    onDisableChange: (v: boolean) => void;
    onMSMChange: (m: MSMType) => void;
    onFallbackChange: (f: boolean) => void;
    onARPChange: (a: boolean) => void;
    disabled?: boolean;
    // notSupported greys the whole subgroup (muted title) when the receiver has
    // no RTCM output capability (neither MSM version). msm4Supported/msm7Supported
    // grey the individual MSM radios when only one version is available.
    notSupported?: boolean;
    msm4Supported?: boolean;
    msm7Supported?: boolean;
}

/** Compute the wire value for Apply. Returns undefined if change is false (skip). */
export function rtcmWireValue(change: boolean, disableProtocol: boolean, msm: MSMType, fallback: boolean, arp: boolean): RTCMMsgFlags | undefined {
    if (!change) return undefined;
    if (disableProtocol) return [];
    const v: RTCMMsgFlags = [RTCMMsgOther];
    if (msm === 'msm4') v.push(RTCMMsgMSM4);
    if (msm === 'msm7') v.push(RTCMMsgMSM7);
    if (fallback && msm !== 'none') v.push(RTCMMsgLax);
    if (arp) v.push(RTCMMsgARP);
    return v;
}

const msmOptions: {value: MSMType; label: string}[] = [
    {value: 'none', label: 'No MSM'},
    {value: 'msm4', label: 'MSM4'},
    {value: 'msm7', label: 'MSM7'},
];

export function RTCMGroup({change, disableProtocol, msm, fallback, arp, onChangeChange, onDisableChange, onMSMChange, onFallbackChange, onARPChange, disabled, notSupported, msm4Supported = true, msm7Supported = true}: Props) {
    const groupDisabled = disabled || notSupported;
    const childDisabled = groupDisabled || !change || disableProtocol;
    const disableDisabled = groupDisabled || !change;
    const fallbackDisabled = childDisabled || msm === 'none';
    // Grey an MSM version's radio when only the other version is supported. "No
    // MSM" is always available. When neither is supported the whole group is
    // greyed via notSupported.
    const msmSupported: Record<MSMType, boolean> = {none: true, msm4: msm4Supported, msm7: msm7Supported};
    return (
        <ConfigSubGroup title="RTCM" disabled={notSupported}>
            <div class="flex flex-col gap-1.5">
                <div class="flex gap-x-4">
                    <label class={`flex items-center gap-1.5 ${labeledControlText(!!groupDisabled)}`}>
                        <input type="checkbox" class="accent-accent" checked={change} disabled={groupDisabled}
                            onChange={e => onChangeChange((e.target as HTMLInputElement).checked)} />
                        Change
                    </label>
                    <label class={`flex items-center gap-1.5 ${labeledControlText(disableDisabled)}`}>
                        <input type="checkbox" class="accent-accent" checked={disableProtocol} disabled={disableDisabled}
                            onChange={e => onDisableChange((e.target as HTMLInputElement).checked)} />
                        Disable protocol
                    </label>
                </div>
                <div class="grid grid-cols-[13rem_auto] gap-x-6 items-center">
                    <div class="flex gap-x-4 whitespace-nowrap">
                        {msmOptions.map(opt => {
                            const optDisabled = childDisabled || !msmSupported[opt.value];
                            return (
                                <label key={opt.value} class={`flex items-center gap-1.5 ${labeledControlText(optDisabled)}`}>
                                    <input
                                        type="radio"
                                        name="rtcm-msm"
                                        class="accent-accent"
                                        checked={msm === opt.value}
                                        disabled={optDisabled}
                                        onChange={() => onMSMChange(opt.value)}
                                    />
                                    {opt.label}
                                </label>
                            );
                        })}
                    </div>
                    <label class={`flex items-center gap-1.5 ${labeledControlText(fallbackDisabled)}`}>
                        <input
                            type="checkbox"
                            class="accent-accent"
                            checked={fallback}
                            disabled={fallbackDisabled}
                            onChange={e => onFallbackChange((e.target as HTMLInputElement).checked)}
                        />
                        Allow MSM fallback
                    </label>
                </div>
                {/* ARP checkbox */}
                <label class={`flex items-center gap-1.5 ${labeledControlText(childDisabled)}`}>
                    <input
                        type="checkbox"
                        class="accent-accent"
                        checked={arp}
                        disabled={childDisabled}
                        onChange={e => onARPChange((e.target as HTMLInputElement).checked)}
                    />
                    Antenna reference point (ARP)
                </label>
            </div>
        </ConfigSubGroup>
    );
}

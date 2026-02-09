import {h} from 'preact';
import {RTCMMsgMSM4, RTCMMsgMSM7, RTCMMsgARP, RTCMMsgLax, RTCMMsgOther} from './msg-flags';

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
}

/** Compute the wire value for Apply. Returns undefined if change is false (skip). */
export function rtcmWireValue(change: boolean, disableProtocol: boolean, msm: MSMType, fallback: boolean, arp: boolean): number | undefined {
    if (!change) return undefined;
    if (disableProtocol) return 0;
    let v = RTCMMsgOther;
    if (msm === 'msm4') v |= RTCMMsgMSM4;
    if (msm === 'msm7') v |= RTCMMsgMSM7;
    if (fallback && msm !== 'none') v |= RTCMMsgLax;
    if (arp) v |= RTCMMsgARP;
    return v;
}

const msmOptions: {value: MSMType; label: string}[] = [
    {value: 'none', label: 'No MSM'},
    {value: 'msm4', label: 'MSM4'},
    {value: 'msm7', label: 'MSM7'},
];

export function RTCMGroup({change, disableProtocol, msm, fallback, arp, onChangeChange, onDisableChange, onMSMChange, onFallbackChange, onARPChange, disabled}: Props) {
    const childDisabled = disabled || !change || disableProtocol;
    const disableDisabled = disabled || !change;
    const fallbackDisabled = childDisabled || msm === 'none';
    return (
        <div>
            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">RTCM</div>
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
                {/* MSM type radio + fallback */}
                <div class="grid grid-cols-[13rem_auto] gap-x-6 gap-y-1 items-center">
                    <div class="flex gap-x-4 flex-wrap">
                        {msmOptions.map(opt => (
                            <label key={opt.value} class={`flex items-center gap-1.5 text-xs ${childDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                                <input
                                    type="radio"
                                    name="rtcm-msm"
                                    class="accent-blue-600"
                                    checked={msm === opt.value}
                                    disabled={childDisabled}
                                    onChange={() => onMSMChange(opt.value)}
                                />
                                {opt.label}
                            </label>
                        ))}
                    </div>
                    <label class={`flex items-center gap-1.5 text-xs ${fallbackDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                        <input
                            type="checkbox"
                            class="accent-blue-600"
                            checked={fallback}
                            disabled={fallbackDisabled}
                            onChange={e => onFallbackChange((e.target as HTMLInputElement).checked)}
                        />
                        Allow MSM fallback
                    </label>
                </div>
                {/* ARP checkbox */}
                <label class={`flex items-center gap-1.5 text-xs ${childDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                    <input
                        type="checkbox"
                        class="accent-blue-600"
                        checked={arp}
                        disabled={childDisabled}
                        onChange={e => onARPChange((e.target as HTMLInputElement).checked)}
                    />
                    Antenna reference point (ARP)
                </label>
            </div>
        </div>
    );
}

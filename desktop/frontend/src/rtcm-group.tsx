import {h} from 'preact';
import {ThreeWaySelector, type ThreeWayState} from './three-way-selector';
import {RTCMMsgMSM4, RTCMMsgMSM7, RTCMMsgARP, RTCMMsgLax, RTCMMsgOther} from './msg-flags';

type MSMType = 'none' | 'msm4' | 'msm7';

interface Props {
    state: ThreeWayState;
    msm: MSMType;
    fallback: boolean;
    arp: boolean;
    onStateChange: (s: ThreeWayState) => void;
    onMSMChange: (m: MSMType) => void;
    onFallbackChange: (f: boolean) => void;
    onARPChange: (a: boolean) => void;
    disabled?: boolean;
}

/** Compute the wire value for Apply. Returns undefined if state is 'skip'. */
export function rtcmWireValue(state: ThreeWayState, msm: MSMType, fallback: boolean, arp: boolean): number | undefined {
    if (state === 'skip') return undefined;
    if (state === 'disable') return 0;
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

export function RTCMGroup({state, msm, fallback, arp, onStateChange, onMSMChange, onFallbackChange, onARPChange, disabled}: Props) {
    const childDisabled = disabled || state !== 'configure';
    const fallbackDisabled = childDisabled || msm === 'none';
    return (
        <div>
            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">RTCM</div>
            <ThreeWaySelector name="rtcm-mode" state={state} onChange={onStateChange} disabled={disabled} />
            <div class="mt-1.5 ml-0.5 flex flex-col gap-1.5">
                {/* MSM type radio + fallback */}
                <div class="flex gap-x-4 gap-y-0.5 flex-wrap items-center">
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
                    <label class={`flex items-center gap-1.5 text-xs ml-2 ${fallbackDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
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

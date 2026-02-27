import {h} from 'preact';

export type ThreeWayState = 'skip' | 'configure' | 'disable';

interface Props {
    name: string;
    state: ThreeWayState;
    onChange: (state: ThreeWayState) => void;
    disabled?: boolean;
}

const options: {value: ThreeWayState; label: string}[] = [
    {value: 'skip', label: 'Leave as-is'},
    {value: 'disable', label: 'Disable'},
    {value: 'configure', label: 'Select'},
];

export function ThreeWaySelector({name, state, onChange, disabled}: Props) {
    return (
        <div class="flex gap-x-4 gap-y-0.5 flex-wrap">
            {options.map(opt => (
                <label key={opt.value} class={`flex items-center gap-1.5 text-xs ${disabled ? 'text-text-muted' : 'cursor-pointer text-text-primary'}`}>
                    <input
                        type="radio"
                        name={name}
                        class="accent-accent"
                        checked={state === opt.value}
                        disabled={disabled}
                        onChange={() => { if (!disabled) onChange(opt.value); }}
                    />
                    {opt.label}
                </label>
            ))}
        </div>
    );
}

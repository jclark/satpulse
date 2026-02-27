import {h} from 'preact';
import {useState} from 'preact/hooks';
import type {ComponentChildren} from 'preact';

interface Props {
    title: string;
    defaultOpen?: boolean;
    open?: boolean;
    onToggle?: (open: boolean) => void;
    variant?: 'config' | 'panel';
    children: ComponentChildren;
}

export function CollapsibleSection({title, defaultOpen = true, open: controlledOpen, onToggle, variant = 'config', children}: Props) {
    const [internalOpen, setInternalOpen] = useState(defaultOpen);
    const isOpen = controlledOpen !== undefined ? controlledOpen : internalOpen;
    const toggle = () => {
        const next = !isOpen;
        if (onToggle) onToggle(next);
        if (controlledOpen === undefined) setInternalOpen(next);
    };

    const panelStyle = variant === 'panel';
    return (
        <section class={panelStyle ? '' : 'mb-1'}>
            <button
                type="button"
                class={panelStyle
                    ? 'flex w-full cursor-pointer items-center gap-2 border-none bg-surface-2 px-3 py-2 text-left text-sm font-semibold text-text-primary hover:bg-surface-3'
                    : 'flex w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 py-1.5 text-left text-xs font-semibold uppercase tracking-wider text-text-secondary hover:text-text-primary'
                }
                onClick={toggle}
            >
                <svg
                    class={`w-3 h-3 shrink-0 transition-transform duration-150 ${isOpen ? 'rotate-90' : ''}`}
                    viewBox="0 0 12 12" fill="currentColor"
                >
                    <path d="M4 2l4 4-4 4z" />
                </svg>
                {title}
            </button>
            {isOpen && <div class={panelStyle ? 'px-4 py-3' : 'pl-1 pb-3'}>{children}</div>}
        </section>
    );
}

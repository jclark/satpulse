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
                    ? 'flex items-center gap-2 w-full text-left py-2 px-3 text-sm font-semibold cursor-pointer bg-gray-200/50 dark:bg-gray-700/50 hover:bg-gray-200 dark:hover:bg-gray-700 border-none'
                    : 'flex items-center gap-1.5 w-full text-left py-1.5 px-1 text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider font-semibold hover:text-gray-700 dark:hover:text-gray-200 cursor-pointer bg-transparent border-none'
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

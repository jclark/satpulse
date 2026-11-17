import {h} from 'preact';
import {useState} from 'preact/hooks';
import type {ComponentChildren} from 'preact';

interface Props {
    title: string;
    defaultOpen?: boolean;
    variant?: 'config' | 'panel';
    children: ComponentChildren;
}

export function CollapsibleSection({title, defaultOpen = true, variant = 'config', children}: Props) {
    const [isOpen, setIsOpen] = useState(defaultOpen);
    const toggle = () => setIsOpen(!isOpen);

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
            {isOpen && <div class={panelStyle ? 'pl-8 pr-4 py-3' : 'pl-1 pb-3'}>{children}</div>}
        </section>
    );
}

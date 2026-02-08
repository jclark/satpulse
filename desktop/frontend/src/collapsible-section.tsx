import {h} from 'preact';
import {useState} from 'preact/hooks';
import type {ComponentChildren} from 'preact';

interface Props {
    title: string;
    defaultOpen?: boolean;
    children: ComponentChildren;
}

export function CollapsibleSection({title, defaultOpen = true, children}: Props) {
    const [open, setOpen] = useState(defaultOpen);
    return (
        <section class="mb-1">
            <button
                type="button"
                class="flex items-center gap-1.5 w-full text-left py-1.5 px-1 text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider font-semibold hover:text-gray-700 dark:hover:text-gray-200 cursor-pointer bg-transparent border-none"
                onClick={() => setOpen(v => !v)}
            >
                <svg
                    class={`w-3 h-3 shrink-0 transition-transform duration-150 ${open ? 'rotate-90' : ''}`}
                    viewBox="0 0 12 12" fill="currentColor"
                >
                    <path d="M4 2l4 4-4 4z" />
                </svg>
                {title}
            </button>
            {open && <div class="pl-1 pb-3">{children}</div>}
        </section>
    );
}

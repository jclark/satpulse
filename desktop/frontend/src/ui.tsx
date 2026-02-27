import {h, Fragment} from 'preact';
import {useState} from 'preact/hooks';
import type {ComponentChildren, JSX} from 'preact';

type ClassValue = unknown;

function toClassString(value: ClassValue): string {
    if (typeof value === 'string') return value;
    if (typeof value === 'number') return String(value);
    if (!value) return '';
    if (typeof value === 'object' && 'value' in value) {
        const signalValue = (value as {value?: unknown}).value;
        if (typeof signalValue === 'string') return signalValue;
    }
    return '';
}

export function cx(...values: ClassValue[]): string {
    return values.map(toClassString).filter(Boolean).join(' ');
}

export type ButtonVariant = 'primary' | 'secondary' | 'danger' | 'ghost';
export type ButtonSize = 'sm' | 'md';

export interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
    variant?: ButtonVariant;
    size?: ButtonSize;
}

const buttonBase =
    'inline-flex items-center justify-center rounded border text-xs font-medium transition-colors disabled:cursor-default disabled:opacity-50';

const buttonSizes: Record<ButtonSize, string> = {
    sm: 'px-2.5 py-0.5',
    md: 'px-3.5 py-1',
};

const buttonVariants: Record<ButtonVariant, string> = {
    primary: 'border-accent bg-accent text-surface-2 enabled:hover:border-accent-hover enabled:hover:bg-accent-hover',
    secondary: 'border-border-strong bg-control text-text-primary enabled:hover:border-border-strong enabled:hover:bg-control-hover',
    danger: 'border-danger bg-danger text-surface-2 enabled:hover:border-danger enabled:hover:bg-danger enabled:hover:opacity-90',
    ghost: 'border-transparent bg-transparent text-text-secondary enabled:hover:bg-surface-3 enabled:hover:text-text-primary',
};

export function Button({variant = 'secondary', size = 'md', class: className, children, ...props}: ButtonProps) {
    return (
        <button class={cx(buttonBase, buttonSizes[size], buttonVariants[variant], className)} {...props}>
            {children}
        </button>
    );
}

const inputBase =
    'rounded border bg-surface-2 px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-1 disabled:cursor-default disabled:opacity-60';

export interface InputProps extends JSX.InputHTMLAttributes<HTMLInputElement> {
    invalid?: boolean;
}

export function Input({invalid = false, class: className, ...props}: InputProps) {
    return (
        <input
            class={cx(
                inputBase,
                invalid ? 'border-danger focus:ring-danger' : 'border-border-strong focus:ring-accent',
                className,
            )}
            {...props}
        />
    );
}

export interface SelectProps extends JSX.SelectHTMLAttributes<HTMLSelectElement> {
    invalid?: boolean;
}

export function Select({invalid = false, class: className, ...props}: SelectProps) {
    return (
        <select
            class={cx(
                inputBase,
                invalid ? 'border-danger focus:ring-danger' : 'border-border-strong focus:ring-accent',
                className,
            )}
            {...props}
        />
    );
}

export function Card({class: className, children, ...props}: JSX.HTMLAttributes<HTMLDivElement>) {
    return (
        <div class={cx('rounded border border-border-subtle bg-surface-2', className)} {...props}>
            {children}
        </div>
    );
}


export type BadgeTone = 'default' | 'info' | 'success' | 'warning' | 'error';

export interface BadgeProps extends JSX.HTMLAttributes<HTMLSpanElement> {
    tone?: BadgeTone;
}

const badgeTones: Record<BadgeTone, string> = {
    default: 'bg-surface-3 text-text-primary',
    info: 'bg-info text-surface-2',
    success: 'bg-success text-surface-2',
    warning: 'bg-warning text-surface-1',
    error: 'bg-danger text-surface-2',
};

export function Badge({tone = 'default', class: className, children, ...props}: BadgeProps) {
    return (
        <span
            class={cx(
                'inline-flex items-center rounded px-1 text-[10px] font-semibold leading-4',
                badgeTones[tone],
                className,
            )}
            {...props}
        >
            {children}
        </span>
    );
}


export interface ConfigSubGroupProps {
    title: string;
    disabled?: boolean;
    children: ComponentChildren;
}

export function ConfigSubGroup({title, disabled, children}: ConfigSubGroupProps) {
    return (
        <div>
            <div class={`mb-1 text-xs font-semibold ${disabled ? 'text-text-muted' : 'text-text-secondary'}`}>{title}</div>
            {children}
        </div>
    );
}

export interface ConfigGroupProps {
    title: string;
    defaultOpen?: boolean;
    children: ComponentChildren;
}

export function ConfigGroup({title, defaultOpen = true, children}: ConfigGroupProps) {
    const [open, setOpen] = useState(defaultOpen);
    return (
        <section class="mt-3 first:mt-0">
            <button
                type="button"
                class="flex w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 py-1.5 text-left text-sm font-semibold text-text-secondary hover:text-text-primary"
                onClick={() => setOpen(!open)}
            >
                <svg
                    class={`w-3 h-3 shrink-0 transition-transform duration-150 ${open ? 'rotate-90' : ''}`}
                    viewBox="0 0 12 12" fill="currentColor"
                >
                    <path d="M4 2l4 4-4 4z" />
                </svg>
                {title}
            </button>
            {open && <div class="space-y-4 pl-5 pb-2">{children}</div>}
        </section>
    );
}

export function labeledControlText(disabled: boolean): string {
    return disabled ? 'text-xs text-text-muted' : 'text-xs text-text-primary cursor-pointer';
}

export function fieldLabelText(disabled = false): string {
    return disabled ? 'text-xs text-text-muted' : 'text-xs text-text-secondary';
}

export interface DefinitionRowProps {
    label: string;
    value: string | ComponentChildren;
}

export function DefinitionList({rows, class: className}: {rows: DefinitionRowProps[]; class?: string}) {
    return (
        <dl class={cx('grid gap-x-4 gap-y-2', className)}>
            {rows.map(({label, value}) => (
                <Fragment key={label}>
                    <dt class="text-xs text-text-secondary">{label}</dt>
                    <dd class="text-sm tabular-nums text-text-primary">{value}</dd>
                </Fragment>
            ))}
        </dl>
    );
}

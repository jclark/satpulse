import {h, Fragment} from 'preact';
import {useState, useEffect, useRef, useMemo, useCallback} from 'preact/hooks';
import type {LogEntry} from './app';
import {Badge, Button, Input, Select} from './ui';

interface Props {
    logEntries: LogEntry[];
    setLogEntries: (fn: (prev: LogEntry[]) => LogEntry[]) => void;
}

const levels = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
type Level = (typeof levels)[number];

const levelOrder: Record<string, number> = {DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3};

const badgeTone: Record<string, 'default' | 'info' | 'success' | 'warning' | 'error'> = {
    DEBUG: 'default',
    INFO: 'info',
    WARN: 'warning',
    ERROR: 'error',
};

const levelTextClass: Record<string, string> = {
    DEBUG: 'text-text-muted',
    INFO: 'text-info',
    WARN: 'text-warning',
    ERROR: 'text-danger',
};

function formatAttrValue(v: any): string {
    if (v === null || v === undefined) return '';
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
}

function formatAttrs(attrs: Record<string, any>): string {
    return Object.entries(attrs)
        .map(([k, v]) => k + '=' + formatAttrValue(v))
        .join(' ');
}

function matchesSearch(entry: LogEntry, q: string): boolean {
    if (entry.message.toLowerCase().includes(q)) return true;
    if (entry.component && entry.component.toLowerCase().includes(q)) return true;
    if (entry.attrs) {
        for (const [k, v] of Object.entries(entry.attrs)) {
            if (k.toLowerCase().includes(q) || String(v).toLowerCase().includes(q)) return true;
        }
    }
    return false;
}

export function LoggingPanel({logEntries, setLogEntries}: Props) {
    const logRef = useRef<HTMLDivElement>(null);
    const autoScrollRef = useRef(true);
    const [minLevel, setMinLevel] = useState<Level>('INFO');
    const [component, setComponent] = useState('');
    const [search, setSearch] = useState('');

    const components = useMemo(() => {
        const s = new Set<string>();
        for (const e of logEntries) {
            if (e.component) s.add(e.component);
        }
        return Array.from(s).sort();
    }, [logEntries]);

    const filtered = useMemo(() => {
        const minOrd = levelOrder[minLevel] ?? 0;
        const q = search.toLowerCase();
        return logEntries.filter(e => {
            if ((levelOrder[e.level] ?? 0) < minOrd) return false;
            if (component && e.component !== component) return false;
            if (q && !matchesSearch(e, q)) return false;
            return true;
        });
    }, [logEntries, minLevel, component, search]);

    const handleScroll = useCallback(() => {
        const el = logRef.current;
        if (!el) return;
        autoScrollRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    }, []);

    useEffect(() => {
        const el = logRef.current;
        if (el && autoScrollRef.current) {
            el.scrollTop = el.scrollHeight;
        }
    }, [filtered]);

    return (
        <div class="flex h-full flex-col">
            <div class="flex shrink-0 flex-wrap items-center gap-2 px-3 py-1.5">
                <h3 class="mr-2 text-xs uppercase tracking-wider text-text-secondary">Log</h3>
                <label class="text-xs text-text-secondary">Level</label>
                <Select value={minLevel} class="px-1.5 py-0.5" onChange={e => setMinLevel((e.target as HTMLSelectElement).value as Level)}>
                    {levels.map(l => (
                        <option key={l} value={l}>
                            {l}
                        </option>
                    ))}
                </Select>
                {components.length > 0 && (
                    <Fragment>
                        <label class="text-xs text-text-secondary">Component</label>
                        <Select value={component} class="px-1.5 py-0.5" onChange={e => setComponent((e.target as HTMLSelectElement).value)}>
                            <option value="">All</option>
                            {components.map(c => (
                                <option key={c} value={c}>
                                    {c}
                                </option>
                            ))}
                        </Select>
                    </Fragment>
                )}
                <Input
                    type="text"
                    class="w-36 px-1.5 py-0.5"
                    placeholder="Search..."
                    value={search}
                    onInput={e => setSearch((e.target as HTMLInputElement).value)}
                />
                <Button size="sm" onClick={() => setLogEntries(() => [])}>
                    Clear
                </Button>
            </div>
            <div
                ref={logRef}
                onScroll={handleScroll}
                class="flex-1 overflow-x-auto overflow-y-auto border-t border-border-subtle bg-surface-1 px-2.5 py-1.5 font-mono text-xs leading-relaxed"
            >
                {filtered.map((entry, i) => (
                    <div key={i} class="whitespace-nowrap py-px">
                        <span class="text-text-muted">{entry.time}</span>{' '}
                        <Badge tone={badgeTone[entry.level] || 'default'} class="w-12 justify-center">
                            {entry.level}
                        </Badge>{' '}
                        {entry.component && (
                            <Fragment>
                                <span class="text-text-secondary">[{entry.component}]</span>{' '}
                            </Fragment>
                        )}
                        <span class={levelTextClass[entry.level] || 'text-text-primary'}>{entry.message}</span>
                        {entry.attrs && Object.keys(entry.attrs).length > 0 && (
                            <Fragment>
                                {' '}
                                <span class="text-text-muted">{formatAttrs(entry.attrs)}</span>
                            </Fragment>
                        )}
                    </div>
                ))}
            </div>
        </div>
    );
}

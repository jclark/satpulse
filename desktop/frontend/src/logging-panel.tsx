import {h, Fragment} from 'preact';
import {useState, useEffect, useRef, useMemo, useCallback} from 'preact/hooks';
import type {LogEntry} from './app';

interface Props {
    logEntries: LogEntry[];
    setLogEntries: (fn: (prev: LogEntry[]) => LogEntry[]) => void;
}

const levels = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
type Level = typeof levels[number];

const levelOrder: Record<string, number> = {DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3};

const badgeClass: Record<string, string> = {
    DEBUG: 'bg-gray-500 text-white',
    INFO: 'bg-blue-500 text-white',
    WARN: 'bg-amber-500 text-black',
    ERROR: 'bg-red-500 text-white',
};

const levelTextClass: Record<string, string> = {
    DEBUG: 'text-gray-400',
    INFO: 'text-blue-400',
    WARN: 'text-amber-400',
    ERROR: 'text-red-400',
};

function formatAttrValue(v: any): string {
    if (v === null || v === undefined) return '';
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
}

function formatAttrs(attrs: Record<string, any>): string {
    return Object.entries(attrs).map(([k, v]) => k + '=' + formatAttrValue(v)).join(' ');
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

    // Collect unique components from entries
    const components = useMemo(() => {
        const s = new Set<string>();
        for (const e of logEntries) {
            if (e.component) s.add(e.component);
        }
        return Array.from(s).sort();
    }, [logEntries]);

    // Filter entries
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

    // Track scroll position to decide auto-scroll
    const handleScroll = useCallback(() => {
        const el = logRef.current;
        if (!el) return;
        autoScrollRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    }, []);

    // Auto-scroll when new entries arrive
    useEffect(() => {
        const el = logRef.current;
        if (el && autoScrollRef.current) {
            el.scrollTop = el.scrollHeight;
        }
    }, [filtered]);

    const btnClass = 'px-2.5 py-0.5 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white';
    const selectClass = 'bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-1.5 py-0.5 rounded text-xs';

    return (
        <div class="flex flex-col h-full">
            {/* Toolbar */}
            <div class="flex items-center gap-2 px-3 py-1.5 shrink-0 flex-wrap">
                <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mr-2">Log</h3>
                <label class="text-xs text-gray-500 dark:text-gray-400">Level</label>
                <select
                    class={selectClass}
                    value={minLevel}
                    onChange={e => setMinLevel((e.target as HTMLSelectElement).value as Level)}
                >
                    {levels.map(l => <option key={l} value={l}>{l}</option>)}
                </select>
                {components.length > 0 && (
                    <>
                        <label class="text-xs text-gray-500 dark:text-gray-400">Component</label>
                        <select
                            class={selectClass}
                            value={component}
                            onChange={e => setComponent((e.target as HTMLSelectElement).value)}
                        >
                            <option value="">All</option>
                            {components.map(c => <option key={c} value={c}>{c}</option>)}
                        </select>
                    </>
                )}
                <input
                    type="text"
                    class={selectClass + ' w-36'}
                    placeholder="Search..."
                    value={search}
                    onInput={e => setSearch((e.target as HTMLInputElement).value)}
                />
                <button class={btnClass} onClick={() => setLogEntries(() => [])}>
                    Clear
                </button>
            </div>
            {/* Log entries */}
            <div
                ref={logRef}
                onScroll={handleScroll}
                class="flex-1 font-mono text-xs leading-relaxed bg-gray-900 dark:bg-black border-t border-gray-200 dark:border-gray-700 px-2.5 py-1.5 overflow-y-auto overflow-x-auto"
            >
                {filtered.map((entry, i) => (
                    <div key={i} class="whitespace-nowrap py-px">
                        <span class="text-gray-500">{entry.time}</span>
                        {' '}
                        <span class={`inline-block text-center rounded px-1 w-12 text-[10px] leading-4 font-semibold ${badgeClass[entry.level] || badgeClass.INFO}`}>
                            {entry.level}
                        </span>
                        {' '}
                        {entry.component && (
                            <><span class="text-purple-400">[{entry.component}]</span>{' '}</>
                        )}
                        <span class={levelTextClass[entry.level] || 'text-green-400'}>
                            {entry.message}
                        </span>
                        {entry.attrs && Object.keys(entry.attrs).length > 0 && (
                            <>{' '}<span class="text-gray-500">{formatAttrs(entry.attrs)}</span></>
                        )}
                    </div>
                ))}
            </div>
        </div>
    );
}

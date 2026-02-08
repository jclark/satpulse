import {h} from 'preact';
import {useEffect, useRef} from 'preact/hooks';
import type {LogEntry} from './app';

interface Props {
    logEntries: LogEntry[];
    setLogEntries: (fn: (prev: LogEntry[]) => LogEntry[]) => void;
}

export function LoggingPanel({logEntries, setLogEntries}: Props) {
    const logRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        const el = logRef.current;
        if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 100) {
            el.scrollTop = el.scrollHeight;
        }
    }, [logEntries]);

    return (
        <div class="flex flex-col h-full">
            <div class="flex items-center gap-2 px-3 py-1.5 shrink-0">
                <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider">Activity log</h3>
                <button
                    class="px-2.5 py-0.5 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white"
                    onClick={() => setLogEntries(() => [])}
                >
                    Clear
                </button>
            </div>
            <div
                ref={logRef}
                class="flex-1 font-mono text-xs leading-relaxed bg-gray-900 dark:bg-black border-t border-gray-200 dark:border-gray-700 px-2.5 py-1.5 overflow-y-auto whitespace-pre-wrap break-all"
            >
                {logEntries.map((entry, i) => (
                    <div key={i}>
                        <span class="text-gray-500 dark:text-gray-400">{entry.time}</span>{' '}
                        <span class={
                            entry.level === 'WARN' ? 'text-amber-400' :
                            entry.level === 'ERROR' ? 'text-red-400' :
                            'text-green-400'
                        }>{entry.message}</span>
                    </div>
                ))}
            </div>
        </div>
    );
}

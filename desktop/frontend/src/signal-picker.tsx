import {h} from 'preact';
import {useState} from 'preact/hooks';
import {main} from '../wailsjs/go/models';

interface Props {
    signalCatalog: main.GNSSInfo[];
    selectedSignals: Set<number>;
    onConfirm: (signals: Set<number>) => void;
    onCancel: () => void;
}

export function SignalPicker({signalCatalog, selectedSignals, onConfirm, onCancel}: Props) {
    const [working, setWorking] = useState<Set<number>>(() => new Set(selectedSignals));

    const toggle = (index: number) => {
        setWorking(prev => {
            const next = new Set(prev);
            if (next.has(index)) next.delete(index);
            else next.add(index);
            return next;
        });
    };

    const toggleGNSS = (gnss: main.GNSSInfo, checked: boolean) => {
        setWorking(prev => {
            const next = new Set(prev);
            for (const sig of gnss.signals) {
                if (checked) next.add(sig.index);
                else next.delete(sig.index);
            }
            return next;
        });
    };

    return (
        <div class="fixed inset-0 bg-black/60 z-50 flex items-center justify-center" onClick={onCancel}>
            <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg w-[700px] max-w-[90vw] max-h-[80vh] flex flex-col" onClick={e => e.stopPropagation()}>
                {/* Header */}
                <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200 dark:border-gray-700">
                    <h2 class="text-sm font-semibold">Select GNSS signals</h2>
                    <button class="bg-transparent border-none text-gray-900 dark:text-gray-100 text-lg cursor-pointer" onClick={onCancel}>
                        &#x2715;
                    </button>
                </div>

                {/* Body */}
                <div class="p-4 overflow-y-auto flex-1">
                    {signalCatalog.map((gnss, gi) => {
                        const allSelected = gnss.signals.every(s => working.has(s.index));
                        const someSelected = gnss.signals.some(s => working.has(s.index));
                        return (
                            <div key={gi} class="mb-4">
                                <h4 class="text-sm mb-1.5 flex items-center gap-2">
                                    <label class="cursor-pointer flex items-center gap-1.5">
                                        <input
                                            type="checkbox"
                                            class="accent-blue-600"
                                            checked={allSelected}
                                            ref={(el) => { if (el) el.indeterminate = someSelected && !allSelected; }}
                                            onChange={(e) => toggleGNSS(gnss, (e.target as HTMLInputElement).checked)}
                                        />
                                        {gnss.name}
                                    </label>
                                </h4>
                                <div class="flex gap-2 flex-wrap pl-6">
                                    {gnss.signals.map(sig => {
                                        const checked = working.has(sig.index);
                                        return (
                                            <label
                                                key={sig.index}
                                                class={`flex items-center gap-1 text-xs cursor-pointer px-2 py-0.5 rounded border ${
                                                    checked
                                                        ? 'border-blue-600 dark:border-blue-500 bg-blue-600/15'
                                                        : 'border-gray-200 dark:border-gray-700 bg-gray-100 dark:bg-gray-900'
                                                }`}
                                            >
                                                <input
                                                    type="checkbox"
                                                    class="accent-blue-600"
                                                    checked={checked}
                                                    onChange={() => toggle(sig.index)}
                                                />
                                                {sig.name}
                                            </label>
                                        );
                                    })}
                                </div>
                            </div>
                        );
                    })}
                </div>

                {/* Footer */}
                <div class="flex justify-end gap-2 px-4 py-3 border-t border-gray-200 dark:border-gray-700">
                    <button
                        class="px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-gray-300 dark:hover:bg-gray-600"
                        onClick={onCancel}
                    >
                        Cancel
                    </button>
                    <button
                        class="px-3.5 py-1 rounded text-xs border border-blue-600 bg-blue-600 text-white cursor-pointer hover:bg-blue-700"
                        onClick={() => onConfirm(working)}
                    >
                        OK
                    </button>
                </div>
            </div>
        </div>
    );
}

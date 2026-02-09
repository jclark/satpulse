import {h} from 'preact';
import {useState} from 'preact/hooks';

interface Props {
    signalCatalog: Record<string, string[]>;
    selectedSignals: Set<string>;
    onConfirm: (signals: Set<string>) => void;
    onCancel: () => void;
}

export function SignalPicker({signalCatalog, selectedSignals, onConfirm, onCancel}: Props) {
    const [working, setWorking] = useState<Set<string>>(() => new Set(selectedSignals));

    const toggle = (key: string) => {
        setWorking(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    };

    const toggleGNSS = (gnssName: string, sigs: string[], checked: boolean) => {
        setWorking(prev => {
            const next = new Set(prev);
            for (const sig of sigs) {
                const key = `${gnssName}:${sig}`;
                if (checked) next.add(key);
                else next.delete(key);
            }
            return next;
        });
    };

    const gnssNames = Object.keys(signalCatalog);

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
                    {gnssNames.map(gnssName => {
                        const sigs = signalCatalog[gnssName];
                        const allSelected = sigs.every(sig => working.has(`${gnssName}:${sig}`));
                        const someSelected = sigs.some(sig => working.has(`${gnssName}:${sig}`));
                        return (
                            <div key={gnssName} class="mb-4">
                                <h4 class="text-sm mb-1.5 flex items-center gap-2">
                                    <label class="cursor-pointer flex items-center gap-1.5">
                                        <input
                                            type="checkbox"
                                            class="accent-blue-600"
                                            checked={allSelected}
                                            ref={(el) => { if (el) el.indeterminate = someSelected && !allSelected; }}
                                            onChange={(e) => toggleGNSS(gnssName, sigs, (e.target as HTMLInputElement).checked)}
                                        />
                                        {gnssName}
                                    </label>
                                </h4>
                                <div class="flex gap-2 flex-wrap pl-6">
                                    {sigs.map(sig => {
                                        const key = `${gnssName}:${sig}`;
                                        const checked = working.has(key);
                                        return (
                                            <label
                                                key={key}
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
                                                    onChange={() => toggle(key)}
                                                />
                                                {sig}
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

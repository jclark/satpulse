import {h} from 'preact';
import {useState} from 'preact/hooks';
import {Button, Card} from './ui';

interface Props {
    signalCatalog: Record<string, string[]>;
    selectedSignals: Set<string>;
    // isSupported reports whether a constellation is supported by the receiver.
    // Unsupported constellations render greyed and cannot be toggled.
    isSupported: (gnssName: string) => boolean;
    onConfirm: (signals: Set<string>) => void;
    onCancel: () => void;
}

export function SignalPicker({signalCatalog, selectedSignals, isSupported, onConfirm, onCancel}: Props) {
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
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-surface-1/70" onClick={onCancel}>
            <Card class="flex max-h-[80vh] w-[700px] max-w-[90vw] flex-col rounded-lg" onClick={e => e.stopPropagation()}>
                <div class="flex items-center justify-between border-b border-border-subtle px-4 py-3">
                    <h2 class="text-sm font-semibold text-text-primary">Select GNSS signals</h2>
                    <button class="cursor-pointer border-none bg-transparent text-lg text-text-primary" onClick={onCancel}>
                        &#x2715;
                    </button>
                </div>

                <div class="flex-1 overflow-y-auto p-4">
                    {gnssNames.map(gnssName => {
                        const sigs = signalCatalog[gnssName];
                        const supported = isSupported(gnssName);
                        const allSelected = sigs.every(sig => working.has(`${gnssName}:${sig}`));
                        const someSelected = sigs.some(sig => working.has(`${gnssName}:${sig}`));
                        return (
                            <div key={gnssName} class="mb-4">
                                <h4 class={`mb-1.5 flex items-center gap-2 text-sm ${supported ? 'text-text-primary' : 'text-text-muted'}`}>
                                    <label class={`flex items-center gap-1.5 ${supported ? 'cursor-pointer text-text-primary' : 'text-text-muted'}`}>
                                        <input
                                            type="checkbox"
                                            class="accent-accent"
                                            checked={allSelected}
                                            disabled={!supported}
                                            ref={el => {
                                                if (el) el.indeterminate = someSelected && !allSelected;
                                            }}
                                            onChange={e => toggleGNSS(gnssName, sigs, (e.target as HTMLInputElement).checked)}
                                        />
                                        {gnssName}
                                    </label>
                                </h4>
                                <div class="flex flex-wrap gap-2 pl-6">
                                    {sigs.map(sig => {
                                        const key = `${gnssName}:${sig}`;
                                        const checked = working.has(key);
                                        return (
                                            <label
                                                key={key}
                                                class={`flex items-center gap-1 rounded border px-2 py-0.5 text-xs ${
                                                    !supported
                                                        ? 'border-border-subtle bg-surface-1 text-text-muted'
                                                        : checked
                                                            ? 'cursor-pointer border-accent bg-accent/15 text-text-primary'
                                                            : 'cursor-pointer border-border-subtle bg-surface-1 text-text-secondary'
                                                }`}
                                            >
                                                <input type="checkbox" class="accent-accent" checked={checked} disabled={!supported} onChange={() => toggle(key)} />
                                                {sig}
                                            </label>
                                        );
                                    })}
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div class="flex justify-end gap-2 border-t border-border-subtle px-4 py-3">
                    <Button onClick={onCancel}>Cancel</Button>
                    <Button variant="primary" onClick={() => onConfirm(working)}>
                        OK
                    </Button>
                </div>
            </Card>
        </div>
    );
}

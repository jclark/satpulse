import {h, Fragment} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {ApplyConfig, SaveConfig, ResetConfig, ReadConfig} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {SignalPicker} from './signal-picker';
import {CollapsibleSection} from './collapsible-section';
import type {OperationState} from './app';

interface Props {
    connected: boolean;
    receiverInfo: main.ReceiverInfo | null;
    signalCatalog: main.GNSSInfo[];
    selectedSignals: Set<number>;
    setSelectedSignals: (fn: (prev: Set<number>) => Set<number>) => void;
    setStatusText: (s: string) => void;
    setOperation: (op: OperationState) => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
    onConfigReadback: (info: main.ReceiverInfo) => void;
}

function setsEqual(a: Set<number>, b: Set<number>): boolean {
    if (a.size !== b.size) return false;
    for (const v of a) if (!b.has(v)) return false;
    return true;
}

function countPendingChanges(
    cfg: Record<string, any> | undefined,
    mode: string, ppsPeriod: string, ppsWidth: string, ppsAlign: boolean, ppsLocked: boolean, ppsRising: boolean,
    timeGNSS: string, cableDelay: string, minElev: string,
    selectedSignals: Set<number>, originalSignals: Set<number>,
): number {
    let n = 0;
    if (!setsEqual(selectedSignals, originalSignals)) n++;
    const tp = cfg?.timePulse as Record<string, any> | undefined;
    if (mode && cfg?.mode && mode !== cfg.mode) n++;
    if (ppsPeriod !== '' && tp?.period !== undefined && parseFloat(ppsPeriod) !== tp.period) n++;
    if (ppsWidth !== '' && tp?.width !== undefined && parseFloat(ppsWidth) !== tp.width) n++;
    if (tp && ppsAlign !== tp.alignToGNSS) n++;
    if (tp && ppsLocked !== tp.onlyWhenLocked) n++;
    if (tp && ppsRising !== tp.polarityRising) n++;
    if (timeGNSS && cfg?.timeGNSS && timeGNSS !== String(cfg.timeGNSS)) n++;
    if (cableDelay !== '' && cfg?.antennaCableDelay !== undefined && parseFloat(cableDelay) !== cfg.antennaCableDelay) n++;
    if (minElev !== '' && cfg?.minElevation !== undefined && parseFloat(minElev) !== cfg.minElevation) n++;
    return n;
}

interface FieldError {
    field: string;
    message: string;
}

function validateFields(ppsPeriod: string, ppsWidth: string, cableDelay: string, minElev: string): FieldError[] {
    const errors: FieldError[] = [];
    if (ppsPeriod !== '') {
        const v = parseFloat(ppsPeriod);
        if (isNaN(v) || v < 0) errors.push({field: 'ppsPeriod', message: 'Must be >= 0 s'});
    }
    if (ppsWidth !== '') {
        const v = parseFloat(ppsWidth);
        if (isNaN(v) || v < 0 || v > 1) errors.push({field: 'ppsWidth', message: 'Must be 0-1 s'});
    }
    if (ppsWidth !== '' && ppsPeriod !== '') {
        const w = parseFloat(ppsWidth), p = parseFloat(ppsPeriod);
        if (!isNaN(w) && !isNaN(p) && w > 0 && p > 0 && w >= p) {
            errors.push({field: 'ppsWidth', message: 'Width must be less than period'});
        }
    }
    if (cableDelay !== '') {
        const v = parseFloat(cableDelay);
        if (isNaN(v)) errors.push({field: 'cableDelay', message: 'Must be a number'});
    }
    if (minElev !== '') {
        const v = parseFloat(minElev);
        if (isNaN(v) || v < 0 || v > 90) errors.push({field: 'minElev', message: 'Must be 0-90 deg'});
    }
    return errors;
}

const inputClass = 'bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs';
const inputErrorClass = 'bg-gray-100 dark:bg-gray-900 border border-red-500 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs';
const btnClass = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100';
const btnPrimary = 'px-3.5 py-1 rounded text-xs border border-blue-600 bg-blue-600 text-white cursor-pointer hover:bg-blue-700 disabled:opacity-50 disabled:cursor-default';
const btnDanger = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-red-400 cursor-pointer hover:bg-red-400 hover:border-red-400 hover:text-black disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-red-400';

export function ConfigPanel({connected, receiverInfo, signalCatalog, selectedSignals, setSelectedSignals, setStatusText, setOperation, addToast, onConfigReadback}: Props) {
    const [mode, setMode] = useState('');
    const [ppsPeriod, setPpsPeriod] = useState('');
    const [ppsWidth, setPpsWidth] = useState('');
    const [ppsAlign, setPpsAlign] = useState(true);
    const [ppsLocked, setPpsLocked] = useState(true);
    const [ppsRising, setPpsRising] = useState(true);
    const [timeGNSS, setTimeGNSS] = useState('');
    const [cableDelay, setCableDelay] = useState('');
    const [minElev, setMinElev] = useState('');
    const [showPicker, setShowPicker] = useState(false);

    // Readback state
    const [readbackTime, setReadbackTime] = useState<string | null>(null);
    const [reading, setReading] = useState(false);
    const [originalSignals, setOriginalSignals] = useState<Set<number>>(new Set());
    const hasReadback = useRef(false);

    // Confirmation dialog
    const [confirmAction, setConfirmAction] = useState<string | null>(null);

    // Applying state
    const [applying, setApplying] = useState(false);

    const populateFromConfig = useCallback((cfg: Record<string, any> | undefined, signalIndices?: number[]) => {
        if (!cfg) return;
        if (cfg.mode) setMode(cfg.mode);
        const tp = cfg.timePulse as Record<string, any> | undefined;
        if (tp) {
            if (tp.period !== undefined) setPpsPeriod(String(tp.period));
            if (tp.width !== undefined) setPpsWidth(String(tp.width));
            if (tp.alignToGNSS !== undefined) setPpsAlign(tp.alignToGNSS);
            if (tp.onlyWhenLocked !== undefined) setPpsLocked(tp.onlyWhenLocked);
            if (tp.polarityRising !== undefined) setPpsRising(tp.polarityRising);
        }
        if (cfg.timeGNSS) setTimeGNSS(String(cfg.timeGNSS));
        if (cfg.antennaCableDelay !== undefined) setCableDelay(String(cfg.antennaCableDelay));
        if (cfg.minElevation !== undefined) setMinElev(String(cfg.minElevation));
        if (signalIndices && signalIndices.length > 0) {
            const s = new Set(signalIndices);
            setSelectedSignals(() => s);
            setOriginalSignals(s);
        }
    }, [setSelectedSignals]);

    useEffect(() => {
        if (!receiverInfo?.config) return;
        populateFromConfig(receiverInfo.config, receiverInfo.signalIndices);
    }, [receiverInfo, populateFromConfig]);

    // Trigger readback when panel first becomes visible and connected
    useEffect(() => {
        if (connected && !hasReadback.current) {
            hasReadback.current = true;
            doReadback();
        }
        if (!connected) hasReadback.current = false;
    }, [connected]);

    const doReadback = async () => {
        if (!connected || reading) return;
        setReading(true);
        setStatusText('Reading configuration...');
        setOperation({status: 'running', label: 'Reading configuration'});
        try {
            const info = await ReadConfig();
            if (info.ok) {
                populateFromConfig(info.config, info.signalIndices);
                onConfigReadback(info);
                setReadbackTime(new Date().toLocaleTimeString());
                setStatusText('Configuration read');
                setOperation({status: 'success', label: 'Reading configuration'});
            } else {
                addToast(info.error || 'Readback failed', 'error');
                setStatusText('Readback failed');
                setOperation({status: 'failed', label: 'Reading configuration', error: info.error || 'Readback failed'});
            }
        } catch (e: any) {
            addToast('Readback error: ' + e.message, 'error');
            setOperation({status: 'failed', label: 'Reading configuration', error: e.message});
        } finally {
            setReading(false);
        }
    };

    const handleApply = async () => {
        const cfg: Record<string, any> = {};
        if (selectedSignals.size > 0) {
            cfg.setSignals = true;
            cfg.signalIndices = Array.from(selectedSignals);
        }
        if (mode) { cfg.setMode = true; cfg.mode = mode; }
        if (ppsPeriod !== '' || ppsWidth !== '') {
            cfg.setPPS = true;
            cfg.ppsPeriod = parseFloat(ppsPeriod) || 0;
            cfg.ppsWidth = parseFloat(ppsWidth) || 0;
            cfg.ppsAlignToGNSS = ppsAlign;
            cfg.ppsOnlyLocked = ppsLocked;
            cfg.ppsRising = ppsRising;
        }
        if (timeGNSS) { cfg.setTimeGNSS = true; cfg.timeGNSS = timeGNSS; }
        if (cableDelay !== '') { cfg.setCableDelay = true; cfg.cableDelay = parseFloat(cableDelay) || 0; }
        if (minElev !== '') { cfg.setMinElevation = true; cfg.minElevation = parseFloat(minElev) || 0; }
        setApplying(true);
        setStatusText('Applying configuration...');
        setOperation({status: 'running', label: 'Applying configuration'});
        const r = await ApplyConfig(new main.ConfigUpdate(cfg));
        setApplying(false);
        if (r.ok) {
            addToast('Configuration applied', 'success');
            setStatusText('Configuration applied');
            setOperation({status: 'success', label: 'Applying configuration'});
            setOriginalSignals(new Set(selectedSignals));
        } else {
            addToast(r.error || 'Apply failed', 'error');
            setStatusText('Configuration failed');
            setOperation({status: 'failed', label: 'Applying configuration', error: r.error || 'Apply failed'});
        }
    };

    const handleSave = async () => {
        setStatusText('Saving to NVM...');
        setOperation({status: 'running', label: 'Saving to NVM'});
        const r = await SaveConfig();
        if (r.ok) {
            addToast('Configuration saved to NVM', 'success');
            setStatusText('Saved to NVM');
            setOperation({status: 'success', label: 'Saving to NVM'});
        } else {
            addToast(r.error || 'Save failed', 'error');
            setStatusText('Save failed');
            setOperation({status: 'failed', label: 'Saving to NVM', error: r.error || 'Save failed'});
        }
    };

    const handleReset = async (type: string) => {
        setConfirmAction(null);
        const label = 'Resetting receiver (' + type + ')';
        setStatusText(label + '...');
        setOperation({status: 'running', label});
        const r = await ResetConfig(type);
        if (r.ok) {
            addToast('Receiver reset: ' + type, 'success');
            setStatusText('Reset complete: ' + type);
            setOperation({status: 'success', label});
        } else {
            addToast(r.error || 'Reset failed', 'error');
            setStatusText('Reset failed');
            setOperation({status: 'failed', label, error: r.error || 'Reset failed'});
        }
    };

    const toggleConstellation = (gnss: main.GNSSInfo, enable: boolean) => {
        setSelectedSignals(prev => {
            const next = new Set(prev);
            for (const sig of gnss.signals) {
                if (enable) next.add(sig.index);
                else next.delete(sig.index);
            }
            return next;
        });
    };

    const cfg = receiverInfo?.config;
    const errors = validateFields(ppsPeriod, ppsWidth, cableDelay, minElev);
    const errorMap = new Map(errors.map(e => [e.field, e.message]));
    const hasErrors = errors.length > 0;
    const pendingCount = countPendingChanges(cfg, mode, ppsPeriod, ppsWidth, ppsAlign, ppsLocked, ppsRising, timeGNSS, cableDelay, minElev, selectedSignals, originalSignals);

    return (
        <div class="flex flex-col h-full">
            {/* Sticky action bar */}
            <div class="shrink-0 flex items-center gap-2 px-4 py-2 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <h2 class="text-sm font-semibold mr-auto">Configuration</h2>
                {readbackTime && (
                    <span class="text-[10px] text-gray-400">Read {readbackTime}</span>
                )}
                {pendingCount > 0 && (
                    <span class="text-[10px] text-amber-500 font-medium">{pendingCount} pending</span>
                )}
                <button class={btnClass} disabled={!connected || reading} onClick={doReadback}>
                    {reading ? 'Reading...' : 'Refresh'}
                </button>
                <button class={btnPrimary} disabled={!connected || hasErrors || applying} onClick={handleApply}>
                    {applying ? 'Applying...' : 'Apply'}
                </button>
            </div>

            {/* Scrollable content */}
            <div class="flex-1 overflow-y-auto p-4">

                    <CollapsibleSection title="Time pulse" defaultOpen={true}>
                        <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 items-center">
                            <label class="text-xs text-gray-500 dark:text-gray-400">Period (s)</label>
                            <div class="flex items-center gap-2">
                                <input type="number" class={(errorMap.has('ppsPeriod') ? inputErrorClass : inputClass) + ' w-25'} value={ppsPeriod} step="0.1" min="0" placeholder="e.g. 1.0"
                                    disabled={!connected} onInput={e => setPpsPeriod((e.target as HTMLInputElement).value)} />
                                {errorMap.has('ppsPeriod') && <span class="text-[10px] text-red-500">{errorMap.get('ppsPeriod')}</span>}
                            </div>
                            <label class="text-xs text-gray-500 dark:text-gray-400">Pulse width (s)</label>
                            <div class="flex items-center gap-2">
                                <input type="number" class={(errorMap.has('ppsWidth') ? inputErrorClass : inputClass) + ' w-25'} value={ppsWidth} step="0.01" min="0" max="1" placeholder="e.g. 0.1"
                                    disabled={!connected} onInput={e => setPpsWidth((e.target as HTMLInputElement).value)} />
                                {errorMap.has('ppsWidth') && <span class="text-[10px] text-red-500">{errorMap.get('ppsWidth')}</span>}
                            </div>
                            <label class="text-xs text-gray-500 dark:text-gray-400">Align to GNSS</label>
                            <input type="checkbox" class="accent-blue-600 justify-self-start" checked={ppsAlign} disabled={!connected}
                                onChange={e => setPpsAlign((e.target as HTMLInputElement).checked)} />
                            <label class="text-xs text-gray-500 dark:text-gray-400">Only when locked</label>
                            <input type="checkbox" class="accent-blue-600 justify-self-start" checked={ppsLocked} disabled={!connected}
                                onChange={e => setPpsLocked((e.target as HTMLInputElement).checked)} />
                            <label class="text-xs text-gray-500 dark:text-gray-400">Rising edge</label>
                            <input type="checkbox" class="accent-blue-600 justify-self-start" checked={ppsRising} disabled={!connected}
                                onChange={e => setPpsRising((e.target as HTMLInputElement).checked)} />
                            <label class="text-xs text-gray-500 dark:text-gray-400">Time GNSS</label>
                            <select class={inputClass + ' w-30'} value={timeGNSS} disabled={!connected}
                                onChange={e => setTimeGNSS((e.target as HTMLSelectElement).value)}>
                                <option value="">--</option>
                                <option value="GPS">GPS</option>
                                <option value="GAL">Galileo</option>
                                <option value="BDS">BeiDou</option>
                                <option value="GLO">GLONASS</option>
                            </select>
                            <label class="text-xs text-gray-500 dark:text-gray-400">Cable delay (ns)</label>
                            <div class="flex items-center gap-2">
                                <input type="number" class={(errorMap.has('cableDelay') ? inputErrorClass : inputClass) + ' w-25'} value={cableDelay} step="1" placeholder="e.g. 50"
                                    disabled={!connected} onInput={e => setCableDelay((e.target as HTMLInputElement).value)} />
                                {errorMap.has('cableDelay') && <span class="text-[10px] text-red-500">{errorMap.get('cableDelay')}</span>}
                            </div>
                        </div>
                    </CollapsibleSection>

                    {/* Time mode subgroup */}
                    <CollapsibleSection title="Time mode" defaultOpen={true}>
                        <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 items-center">
                            <label class="text-xs text-gray-500 dark:text-gray-400">Mode</label>
                            <select class={inputClass + ' w-30'} value={mode} disabled={!connected}
                                onChange={e => setMode((e.target as HTMLSelectElement).value)}>
                                <option value="">--</option>
                                <option value="static">Static</option>
                                <option value="mobile">Mobile</option>
                            </select>
                        </div>
                    </CollapsibleSection>

                    {/* Constellations subgroup */}
                    <CollapsibleSection title="Constellations" defaultOpen={true}>
                        <div class="flex flex-wrap gap-x-4 gap-y-1">
                            {signalCatalog.map(gnss => {
                                const anySelected = gnss.signals.some(s => selectedSignals.has(s.index));
                                return (
                                    <label key={gnss.name} class="flex items-center gap-1.5 text-xs cursor-pointer">
                                        <input
                                            type="checkbox"
                                            class="accent-blue-600"
                                            checked={anySelected}
                                            disabled={!connected}
                                            onChange={e => toggleConstellation(gnss, (e.target as HTMLInputElement).checked)}
                                        />
                                        {gnss.name}
                                    </label>
                                );
                            })}
                        </div>
                        {signalCatalog.length > 0 && (
                            <button class={btnClass + ' mt-2'} disabled={!connected} onClick={() => setShowPicker(true)}>
                                Edit signals...
                            </button>
                        )}
                    </CollapsibleSection>

                    {/* Other properties */}
                    <CollapsibleSection title="Other" defaultOpen={true}>
                        <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 items-center">
                            <label class="text-xs text-gray-500 dark:text-gray-400">Min elevation (deg)</label>
                            <div class="flex items-center gap-2">
                                <input type="number" class={(errorMap.has('minElev') ? inputErrorClass : inputClass) + ' w-25'} value={minElev} step="1" min="0" max="90" placeholder="e.g. 10"
                                    disabled={!connected} onInput={e => setMinElev((e.target as HTMLInputElement).value)} />
                                {errorMap.has('minElev') && <span class="text-[10px] text-red-500">{errorMap.get('minElev')}</span>}
                            </div>
                        </div>
                    </CollapsibleSection>

                {/* Persistent operations */}
                <CollapsibleSection title="Persistent operations" defaultOpen={false}>
                    <div class="flex gap-2 flex-wrap">
                        <button class={btnClass} disabled={!connected} onClick={handleSave}>Save to NVM</button>
                        <button class={btnClass} disabled={!connected} onClick={() => handleReset('reload')}>Reload</button>
                        <button class={btnDanger} disabled={!connected} onClick={() => handleReset('cold')}>Cold restart</button>
                        <button class={btnDanger} disabled={!connected} onClick={() => setConfirmAction('factory')}>Factory reset</button>
                    </div>
                </CollapsibleSection>

            </div>

            {/* Signal picker dialog */}
            {showPicker && (
                <SignalPicker
                    signalCatalog={signalCatalog}
                    selectedSignals={selectedSignals}
                    onConfirm={(signals) => { setSelectedSignals(() => signals); setShowPicker(false); }}
                    onCancel={() => setShowPicker(false)}
                />
            )}

            {/* Confirmation dialog */}
            {confirmAction && (
                <div class="fixed inset-0 bg-black/60 z-50 flex items-center justify-center" onClick={() => setConfirmAction(null)}>
                    <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 max-w-sm" onClick={e => e.stopPropagation()}>
                        <h3 class="text-sm font-semibold mb-3">Confirm factory reset</h3>
                        <p class="text-xs text-gray-500 dark:text-gray-400 mb-4">
                            This will erase all saved configuration and restore factory defaults. This cannot be undone.
                        </p>
                        <div class="flex justify-end gap-2">
                            <button class={btnClass} onClick={() => setConfirmAction(null)}>Cancel</button>
                            <button class={btnDanger} onClick={() => handleReset('factory')}>Factory reset</button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

import {h, Fragment} from 'preact';
import {useState, useEffect} from 'preact/hooks';
import {ApplyConfig, SaveConfig, ResetConfig} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import {SignalPicker} from './signal-picker';
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
}

function signalSummaryText(catalog: main.GNSSInfo[], selected: Set<number>): string {
    if (selected.size === 0) return 'None selected';
    const parts: string[] = [];
    for (const gnss of catalog) {
        const names = gnss.signals.filter(s => selected.has(s.index)).map(s => s.name);
        if (names.length > 0) parts.push(gnss.name + '[' + names.join(', ') + ']');
    }
    return parts.join('  ') || 'None selected';
}

const inputClass = 'bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs';
const btnClass = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100';
const btnPrimary = 'px-3.5 py-1 rounded text-xs border border-blue-600 bg-blue-600 text-white cursor-pointer hover:bg-blue-700 disabled:opacity-50 disabled:cursor-default';
const btnDanger = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-red-400 cursor-pointer hover:bg-red-400 hover:border-red-400 hover:text-black disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-red-400';

export function ConfigPanel({connected, receiverInfo, signalCatalog, selectedSignals, setSelectedSignals, setStatusText, setOperation, addToast}: Props) {
    const [mode, setMode] = useState('');
    const [ppsWidth, setPpsWidth] = useState('');
    const [ppsPeriod, setPpsPeriod] = useState('');
    const [ppsAlign, setPpsAlign] = useState(true);
    const [ppsLocked, setPpsLocked] = useState(true);
    const [ppsRising, setPpsRising] = useState(true);
    const [timeGNSS, setTimeGNSS] = useState('');
    const [cableDelay, setCableDelay] = useState('');
    const [minElev, setMinElev] = useState('');
    const [showPicker, setShowPicker] = useState(false);

    // Pre-fill form from detected config
    useEffect(() => {
        if (!receiverInfo?.config) return;
        const cfg = receiverInfo.config;
        if (cfg.mode) setMode(cfg.mode);
        const tp = cfg.timePulse as Record<string, any> | undefined;
        if (tp) {
            if (tp.width !== undefined) setPpsWidth(String(tp.width));
            if (tp.period !== undefined) setPpsPeriod(String(tp.period));
            if (tp.alignToGNSS !== undefined) setPpsAlign(tp.alignToGNSS);
            if (tp.onlyWhenLocked !== undefined) setPpsLocked(tp.onlyWhenLocked);
            if (tp.polarityRising !== undefined) setPpsRising(tp.polarityRising);
        }
        if (cfg.timeGNSS) setTimeGNSS(String(cfg.timeGNSS));
        if (cfg.antennaCableDelay !== undefined) setCableDelay(String(cfg.antennaCableDelay));
        if (cfg.minElevation !== undefined) setMinElev(String(cfg.minElevation));
    }, [receiverInfo]);

    const handleApply = async () => {
        const cfg: Record<string, any> = {};
        if (selectedSignals.size > 0) {
            cfg.setSignals = true;
            cfg.signalIndices = Array.from(selectedSignals);
        }
        if (mode) { cfg.setMode = true; cfg.mode = mode; }
        if (ppsWidth !== '' || ppsPeriod !== '') {
            cfg.setPPS = true;
            cfg.ppsWidth = parseFloat(ppsWidth) || 0;
            cfg.ppsPeriod = parseFloat(ppsPeriod) || 0;
            cfg.ppsAlignToGNSS = ppsAlign;
            cfg.ppsOnlyLocked = ppsLocked;
            cfg.ppsRising = ppsRising;
        }
        if (timeGNSS) { cfg.setTimeGNSS = true; cfg.timeGNSS = timeGNSS; }
        if (cableDelay !== '') { cfg.setCableDelay = true; cfg.cableDelay = parseFloat(cableDelay) || 0; }
        if (minElev !== '') { cfg.setMinElevation = true; cfg.minElevation = parseFloat(minElev) || 0; }
        setStatusText('Applying configuration...');
        setOperation({status: 'running', label: 'Applying configuration'});
        const r = await ApplyConfig(new main.ConfigUpdate(cfg));
        if (r.ok) {
            addToast('Configuration applied', 'success');
            setStatusText('Configuration applied');
            setOperation({status: 'success', label: 'Applying configuration'});
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

    const cfg = receiverInfo?.config;
    const tp = cfg?.timePulse as Record<string, any> | undefined;

    return (
        <div class="p-4 overflow-y-auto h-full">
            <h2 class="text-base font-semibold mb-4">GPS configuration</h2>

            {/* Current config (read-only, shown after detection) */}
            {cfg && Object.keys(cfg).length > 0 && (
                <div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded p-3 mb-5 max-w-2xl">
                    <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">Current configuration</h3>
                    <dl class="grid grid-cols-[160px_1fr] gap-x-4 gap-y-2">
                        {cfg.signalsEnabled && <><dt class="text-gray-500 dark:text-gray-400 text-xs">Signals</dt><dd class="text-sm">{cfg.signalsEnabled}</dd></>}
                        {cfg.mode && <><dt class="text-gray-500 dark:text-gray-400 text-xs">Mode</dt><dd class="text-sm">{cfg.mode}</dd></>}
                        {cfg.timeGNSS && <><dt class="text-gray-500 dark:text-gray-400 text-xs">Time GNSS</dt><dd class="text-sm">{String(cfg.timeGNSS)}</dd></>}
                        {cfg.ppsEnabled !== undefined && <><dt class="text-gray-500 dark:text-gray-400 text-xs">PPS</dt><dd class="text-sm">{cfg.ppsEnabled ? 'Enabled' : 'Disabled'}</dd></>}
                        {tp?.width !== undefined && <><dt class="text-gray-500 dark:text-gray-400 text-xs">PPS width</dt><dd class="text-sm">{tp.width} s</dd></>}
                        {tp?.period !== undefined && <><dt class="text-gray-500 dark:text-gray-400 text-xs">PPS period</dt><dd class="text-sm">{tp.period} s</dd></>}
                        {cfg.antennaCableDelay !== undefined && <><dt class="text-gray-500 dark:text-gray-400 text-xs">Cable delay</dt><dd class="text-sm">{cfg.antennaCableDelay} ns</dd></>}
                        {cfg.minElevation !== undefined && <><dt class="text-gray-500 dark:text-gray-400 text-xs">Min elevation</dt><dd class="text-sm">{cfg.minElevation} deg</dd></>}
                        {cfg.navMsgAuth && <><dt class="text-gray-500 dark:text-gray-400 text-xs">Nav msg auth</dt><dd class="text-sm">{String(cfg.navMsgAuth)}</dd></>}
                    </dl>
                </div>
            )}

            {/* Signals */}
            <section class="mb-5 max-w-2xl">
                <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">Signals</h3>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Enabled signals</label>
                    <span class="text-xs">
                        {signalCatalog.length > 0
                            ? signalSummaryText(signalCatalog, selectedSignals).split('  ').map((part, i) => {
                                const m = part.match(/^(.+?)\[(.+)\]$/);
                                if (!m) return <span key={i}>{part}</span>;
                                return (
                                    <span key={i} class="mr-3">
                                        <span class="text-blue-600 dark:text-blue-500 font-semibold">{m[1]}</span>
                                        [{m[2]}]
                                    </span>
                                );
                            })
                            : 'Not configured'
                        }
                    </span>
                    <button class={btnClass} disabled={!connected} onClick={() => setShowPicker(true)}>
                        Edit signals...
                    </button>
                </div>
            </section>

            {/* Mode */}
            <section class="mb-5 max-w-2xl">
                <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">Positioning mode</h3>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Mode</label>
                    <select class={inputClass + ' w-30'} value={mode} onChange={e => setMode((e.target as HTMLSelectElement).value)}>
                        <option value="">--</option>
                        <option value="static">Static</option>
                        <option value="mobile">Mobile</option>
                    </select>
                </div>
            </section>

            {/* PPS */}
            <section class="mb-5 max-w-2xl">
                <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">Time pulse (PPS)</h3>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Pulse width (s)</label>
                    <input type="number" class={inputClass + ' w-25'} value={ppsWidth} step="0.01" min="0" max="1" placeholder="e.g. 0.1"
                        onInput={e => setPpsWidth((e.target as HTMLInputElement).value)} />
                </div>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Period (s)</label>
                    <input type="number" class={inputClass + ' w-25'} value={ppsPeriod} step="0.1" min="0" placeholder="e.g. 1.0"
                        onInput={e => setPpsPeriod((e.target as HTMLInputElement).value)} />
                </div>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Align to GNSS</label>
                    <input type="checkbox" class="accent-blue-600" checked={ppsAlign} onChange={e => setPpsAlign((e.target as HTMLInputElement).checked)} />
                </div>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Only when locked</label>
                    <input type="checkbox" class="accent-blue-600" checked={ppsLocked} onChange={e => setPpsLocked((e.target as HTMLInputElement).checked)} />
                </div>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Rising edge</label>
                    <input type="checkbox" class="accent-blue-600" checked={ppsRising} onChange={e => setPpsRising((e.target as HTMLInputElement).checked)} />
                </div>
            </section>

            {/* Other */}
            <section class="mb-5 max-w-2xl">
                <h3 class="text-xs text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">Other</h3>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Time GNSS</label>
                    <select class={inputClass + ' w-30'} value={timeGNSS} onChange={e => setTimeGNSS((e.target as HTMLSelectElement).value)}>
                        <option value="">--</option>
                        <option value="GPS">GPS</option>
                        <option value="GAL">Galileo</option>
                        <option value="BDS">BeiDou</option>
                        <option value="GLO">GLONASS</option>
                    </select>
                </div>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Cable delay (ns)</label>
                    <input type="number" class={inputClass + ' w-25'} value={cableDelay} step="1" placeholder="e.g. 50"
                        onInput={e => setCableDelay((e.target as HTMLInputElement).value)} />
                </div>
                <div class="flex items-center gap-2.5 mb-2">
                    <label class="text-xs text-gray-500 dark:text-gray-400 min-w-30">Min elevation (deg)</label>
                    <input type="number" class={inputClass + ' w-25'} value={minElev} step="1" min="0" max="90" placeholder="e.g. 10"
                        onInput={e => setMinElev((e.target as HTMLInputElement).value)} />
                </div>
            </section>

            {/* Action buttons */}
            <div class="flex gap-2 pt-4 border-t border-gray-200 dark:border-gray-700 max-w-2xl flex-wrap">
                <button class={btnPrimary} disabled={!connected} onClick={handleApply}>Apply configuration</button>
                <button class={btnClass} disabled={!connected} onClick={handleSave}>Save to NVM</button>
                <button class={btnClass} disabled={!connected} onClick={() => handleReset('reload')}>Reload</button>
                <button class={btnDanger} disabled={!connected} onClick={() => handleReset('cold')}>Cold restart</button>
                <button class={btnDanger} disabled={!connected} onClick={() => handleReset('factory')}>Factory reset</button>
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
        </div>
    );
}

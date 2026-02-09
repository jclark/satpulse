import {h} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {ApplyConfig, SaveConfig, ResetConfig, ReadConfig} from '../wailsjs/go/main/App';
import {SignalPicker} from './signal-picker';
import {CollapsibleSection} from './collapsible-section';
import {NMEAGroup, nmeaWireValue} from './nmea-group';
import {RTCMGroup, rtcmWireValue} from './rtcm-group';
import {PVTGroup, pvtWireValue} from './pvt-group';
import type {OperationState} from './app';

interface Props {
    connected: boolean;
    visible: boolean;
    configProps: Record<string, any> | null;
    signalCatalog: Record<string, string[]>;
    selectedSignals: Set<string>;
    setSelectedSignals: (fn: (prev: Set<string>) => Set<string>) => void;
    setStatusText: (s: string) => void;
    setOperation: (op: OperationState) => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
    onConfigReadback: (props: Record<string, any>) => void;
}

function setsEqual(a: Set<string>, b: Set<string>): boolean {
    if (a.size !== b.size) return false;
    for (const v of a) if (!b.has(v)) return false;
    return true;
}

// Convert a signalsEnabled map {GPS: ["L1","L5"]} to Set<string> {"GPS:L1","GPS:L5"}
function signalMapToSet(m: Record<string, string[]> | undefined): Set<string> {
    const s = new Set<string>();
    if (!m) return s;
    for (const [gnss, sigs] of Object.entries(m)) {
        for (const sig of sigs) s.add(`${gnss}:${sig}`);
    }
    return s;
}

// Convert Set<string> {"GPS:L1","GPS:L5"} back to map {GPS: ["L1","L5"]}
function signalSetToMap(s: Set<string>): Record<string, string[]> {
    const m: Record<string, string[]> = {};
    for (const entry of s) {
        const i = entry.indexOf(':');
        if (i < 0) continue;
        const gnss = entry.slice(0, i), sig = entry.slice(i + 1);
        (m[gnss] ??= []).push(sig);
    }
    return m;
}

function countPendingChanges(
    cfg: Record<string, any> | null,
    mode: string, ppsPeriod: string, ppsWidth: string, ppsAlign: boolean, ppsLocked: boolean, ppsRising: boolean,
    timeGNSS: string, cableDelay: string, minElev: string,
    selectedSignals: Set<string>, originalSignals: Set<string>,
): number {
    let n = 0;
    if (!setsEqual(selectedSignals, originalSignals)) n++;
    const tp = cfg?.timePulse as Record<string, any> | undefined;
    const cfgMode = cfg?.mode as Record<string, any> | undefined;
    if (mode && cfgMode) {
        const curMode = cfgMode.static ? 'static' : 'mobile';
        if (mode !== curMode) n++;
    }
    if (ppsPeriod !== '' && tp?.period !== undefined && parseFloat(ppsPeriod) !== tp.period) n++;
    if (ppsWidth !== '' && tp?.width !== undefined && parseFloat(ppsWidth) !== tp.width) n++;
    if (tp && ppsAlign !== tp.alignToGNSS) n++;
    if (tp && ppsLocked !== tp.onlyWhenLocked) n++;
    if (tp && ppsRising !== tp.polarityRising) n++;
    if (timeGNSS && cfg?.timeGNSS && timeGNSS !== String(cfg.timeGNSS)) n++;
    if (cableDelay !== '' && cfg?.antennaCableDelay !== undefined && parseFloat(cableDelay) !== cfg.antennaCableDelay * 1e9) n++;
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

export function ConfigPanel({connected, visible, configProps, signalCatalog, selectedSignals, setSelectedSignals, setStatusText, setOperation, addToast, onConfigReadback}: Props) {
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

    // Message state
    const [nmeaChange, setNmeaChange] = useState(false);
    const [nmeaDisable, setNmeaDisable] = useState(false);
    const [nmeaFlags, setNmeaFlags] = useState(0);
    const [rtcmChange, setRtcmChange] = useState(false);
    const [rtcmDisable, setRtcmDisable] = useState(false);
    const [rtcmMSM, setRtcmMSM] = useState<'none' | 'msm4' | 'msm7'>('none');
    const [rtcmFallback, setRtcmFallback] = useState(true);
    const [rtcmARP, setRtcmARP] = useState(false);
    const [pvtChange, setPvtChange] = useState(false);
    const [pvtFlags, setPvtFlags] = useState(0);

    // Readback state
    const [reading, setReading] = useState(false);
    const [originalSignals, setOriginalSignals] = useState<Set<string>>(new Set());
    const hasReadback = useRef(false);

    // Confirmation dialog
    const [confirmAction, setConfirmAction] = useState<string | null>(null);

    // Applying state
    const [applying, setApplying] = useState(false);

    // Populate form fields from ConfigProps JSON
    const populateFromConfig = useCallback((cfg: Record<string, any>) => {
        const m = cfg.mode as Record<string, any> | undefined;
        if (m) setMode(m.static ? 'static' : 'mobile');
        const tp = cfg.timePulse as Record<string, any> | undefined;
        if (tp) {
            if (tp.period !== undefined) setPpsPeriod(String(tp.period));
            if (tp.width !== undefined) setPpsWidth(String(tp.width));
            if (tp.alignToGNSS !== undefined) setPpsAlign(tp.alignToGNSS);
            if (tp.onlyWhenLocked !== undefined) setPpsLocked(tp.onlyWhenLocked);
            if (tp.polarityRising !== undefined) setPpsRising(tp.polarityRising);
        }
        if (cfg.timeGNSS) setTimeGNSS(String(cfg.timeGNSS));
        if (cfg.antennaCableDelay !== undefined) setCableDelay(String(cfg.antennaCableDelay * 1e9));
        if (cfg.minElevation !== undefined) setMinElev(String(cfg.minElevation));
        if (cfg.signalsEnabled) {
            const s = signalMapToSet(cfg.signalsEnabled);
            setSelectedSignals(() => s);
            setOriginalSignals(s);
        }
    }, [setSelectedSignals]);

    useEffect(() => {
        if (configProps) populateFromConfig(configProps);
    }, [configProps, populateFromConfig]);

    // Trigger readback on first switch to the config tab while connected
    useEffect(() => {
        if (visible && connected && !hasReadback.current) {
            hasReadback.current = true;
            doReadback();
        }
        if (!connected) hasReadback.current = false;
    }, [visible, connected]);

    const doReadback = async () => {
        if (!connected || reading) return;
        setReading(true);
        setStatusText('Reading configuration...');
        setOperation({status: 'running', label: 'Reading configuration'});
        try {
            const props = await ReadConfig();
            populateFromConfig(props as any);
            onConfigReadback(props as any);
            setStatusText('Configuration read');
            setOperation({status: 'success', label: 'Reading configuration'});
        } catch (e: any) {
            addToast('Readback error: ' + e.message, 'error');
            setStatusText('Readback failed');
            setOperation({status: 'failed', label: 'Reading configuration', error: e.message});
        } finally {
            setReading(false);
        }
    };

    const handleApply = async () => {
        const props: Record<string, any> = {};
        if (selectedSignals.size > 0) {
            props.signalsEnabled = signalSetToMap(selectedSignals);
        }
        if (mode) {
            props.mode = {static: mode === 'static'};
        }
        if (ppsPeriod !== '' || ppsWidth !== '') {
            props.timePulse = {
                period: parseFloat(ppsPeriod) || 0,
                width: parseFloat(ppsWidth) || 0,
                alignToGNSS: ppsAlign,
                onlyWhenLocked: ppsLocked,
                polarityRising: ppsRising,
            };
        }
        if (timeGNSS) props.timeGNSS = timeGNSS;
        if (cableDelay !== '') props.antennaCableDelay = (parseFloat(cableDelay) || 0) * 1e-9;
        if (minElev !== '') props.minElevation = parseFloat(minElev) || 0;
        const opts: Record<string, any> = {};
        const nmeaWire = nmeaWireValue(nmeaChange, nmeaDisable, nmeaFlags);
        if (nmeaWire !== undefined) opts.NMEAMsg = nmeaWire;
        const rtcmWire = rtcmWireValue(rtcmChange, rtcmDisable, rtcmMSM, rtcmFallback, rtcmARP);
        if (rtcmWire !== undefined) opts.RTCMMsg = rtcmWire;
        const pvtWire = pvtWireValue(pvtChange, pvtFlags);
        if (pvtWire !== undefined) opts.PVTMsg = pvtWire;
        const cfg: Record<string, any> = {Props: props, Opts: opts};
        setApplying(true);
        setStatusText('Applying configuration...');
        setOperation({status: 'running', label: 'Applying configuration'});
        const r = await ApplyConfig(cfg as any);
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

    const toggleConstellation = (gnssName: string, sigs: string[], enable: boolean) => {
        setSelectedSignals(prev => {
            const next = new Set(prev);
            for (const sig of sigs) {
                const key = `${gnssName}:${sig}`;
                if (enable) next.add(key);
                else next.delete(key);
            }
            return next;
        });
    };

    const gnssNames = Object.keys(signalCatalog);
    const errors = validateFields(ppsPeriod, ppsWidth, cableDelay, minElev);
    const errorMap = new Map(errors.map(e => [e.field, e.message]));
    const hasErrors = errors.length > 0;
    const pendingCount = countPendingChanges(configProps, mode, ppsPeriod, ppsWidth, ppsAlign, ppsLocked, ppsRising, timeGNSS, cableDelay, minElev, selectedSignals, originalSignals);

    return (
        <div class="flex flex-col h-full">
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
                            {gnssNames.map(gnssName => {
                                const sigs = signalCatalog[gnssName];
                                const anySelected = sigs.some(sig => selectedSignals.has(`${gnssName}:${sig}`));
                                return (
                                    <label key={gnssName} class="flex items-center gap-1.5 text-xs cursor-pointer">
                                        <input
                                            type="checkbox"
                                            class="accent-blue-600"
                                            checked={anySelected}
                                            disabled={!connected}
                                            onChange={e => toggleConstellation(gnssName, sigs, (e.target as HTMLInputElement).checked)}
                                        />
                                        {gnssName}
                                    </label>
                                );
                            })}
                        </div>
                        {gnssNames.length > 0 && (
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

                {/* Messages */}
                <CollapsibleSection title="Messages" defaultOpen={true}>
                    <div class="flex flex-col gap-3">
                        <NMEAGroup
                            change={nmeaChange}
                            disableProtocol={nmeaDisable}
                            flags={nmeaFlags}
                            onChangeChange={setNmeaChange}
                            onDisableChange={setNmeaDisable}
                            onFlagsChange={setNmeaFlags}
                            disabled={!connected}
                        />
                        <RTCMGroup
                            change={rtcmChange}
                            disableProtocol={rtcmDisable}
                            msm={rtcmMSM}
                            fallback={rtcmFallback}
                            arp={rtcmARP}
                            onChangeChange={setRtcmChange}
                            onDisableChange={setRtcmDisable}
                            onMSMChange={setRtcmMSM}
                            onFallbackChange={setRtcmFallback}
                            onARPChange={setRtcmARP}
                            disabled={!connected}
                        />
                        <PVTGroup
                            change={pvtChange}
                            flags={pvtFlags}
                            onChangeChange={setPvtChange}
                            onFlagsChange={setPvtFlags}
                            disabled={!connected}
                        />
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

            {/* Bottom action bar */}
            <div class="shrink-0 flex items-center gap-2 px-4 py-2 border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                {pendingCount > 0 && (
                    <span class="text-[10px] text-amber-500 font-medium">{pendingCount} pending</span>
                )}
                <span class="ml-auto" />
                <button class={btnClass} disabled={!connected || reading} onClick={doReadback}>
                    {reading ? 'Reading...' : 'Refresh'}
                </button>
                <button class={btnPrimary} disabled={!connected || hasErrors || applying} onClick={handleApply}>
                    {applying ? 'Applying...' : 'Apply'}
                </button>
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

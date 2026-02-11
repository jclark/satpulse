import {h} from 'preact';
import {useState, useEffect, useCallback, useRef} from 'preact/hooks';
import {ApplyConfig, CheckOnEarth, ReadConfig} from '../wailsjs/go/main/App';
import {SignalPicker} from './signal-picker';
import {CollapsibleSection} from './collapsible-section';
import {NMEAGroup, nmeaWireValue} from './nmea-group';
import {RTCMGroup, rtcmWireValue} from './rtcm-group';
import {PVTGroup, pvtWireValue} from './pvt-group';
import {SatsGroup, satsWireValue} from './sats-group';
import {RawGroup, rawWireValue} from './raw-group';
import {
    NMEAMsgRMC,
    PVTMsgTimePulse, PVTMsgTimePulseAfter, PVTMsgTAI, PVTMsgLeapSecond, PVTMsgOff, PVTMsgSurvey,
    SatsMsgSat, SatsMsgSignal,
} from './msg-flags';
import type {ConnState, OperationState} from './app';

interface Props {
    connState: ConnState;
    visible: boolean;
    configProps: Record<string, any> | null;
    signalCatalog: Record<string, string[]>;
    selectedSignals: Set<string>;
    setSelectedSignals: (fn: (prev: Set<string>) => Set<string>) => void;
    setOperation: (op: OperationState) => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
    onConfigReadback: (props: Record<string, any>) => void;
    speed: number;
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

function validateFields(
    timeMode: string, coordSystem: string,
    surveyTime: string, surveyAcc: string,
    fixedECEF: [string, string, string], fixedLLH: [string, string, string], fixedPosAcc: string,
    ppsPeriod: string, ppsWidth: string, cableDelay: string, minElev: string,
): Set<string> {
    const bad = new Set<string>();
    if (timeMode === 'survey') {
        if (surveyTime !== '') { const v = Number(surveyTime); if (isNaN(v) || v <= 0) bad.add('surveyTime'); }
        if (surveyAcc !== '') { const v = Number(surveyAcc); if (isNaN(v) || v < 0.001) bad.add('surveyAcc'); }
    }
    if (timeMode === 'fixed') {
        if (coordSystem === 'ecef') {
            for (const [i, f] of (['ecefX', 'ecefY', 'ecefZ'] as const).entries()) {
                if (fixedECEF[i] === '' || isNaN(Number(fixedECEF[i]))) bad.add(f);
            }
        } else {
            if (fixedLLH[0] === '') bad.add('llhLat'); else { const v = Number(fixedLLH[0]); if (isNaN(v) || v < -90 || v > 90) bad.add('llhLat'); }
            if (fixedLLH[1] === '') bad.add('llhLon'); else { const v = Number(fixedLLH[1]); if (isNaN(v) || v < -180 || v > 180) bad.add('llhLon'); }
            if (fixedLLH[2] === '' || isNaN(Number(fixedLLH[2]))) bad.add('llhHeight');
        }
        if (fixedPosAcc !== '') { const v = Number(fixedPosAcc); if (isNaN(v) || v < 0.001) bad.add('fixedPosAcc'); }
    }
    if (ppsPeriod !== '') { const v = Number(ppsPeriod); if (isNaN(v) || v < 0) bad.add('ppsPeriod'); }
    if (ppsWidth !== '') { const v = Number(ppsWidth); if (isNaN(v) || v < 0 || v > 1) bad.add('ppsWidth'); }
    if (ppsWidth !== '' && ppsPeriod !== '') {
        const w = Number(ppsWidth), p = Number(ppsPeriod);
        if (!isNaN(w) && !isNaN(p) && w > 0 && p > 0 && w >= p) bad.add('ppsWidth');
    }
    if (cableDelay !== '' && isNaN(Number(cableDelay))) bad.add('cableDelay');
    if (minElev !== '') { const v = Number(minElev); if (isNaN(v) || v < 0 || v > 90) bad.add('minElev'); }
    return bad;
}

const inputClass = 'bg-gray-100 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs';
const inputErrorClass = 'bg-red-100 dark:bg-red-950 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 px-2 py-1 rounded text-xs';
const btnClass = 'px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100';
const btnPrimary = 'px-3.5 py-1 rounded text-xs border border-blue-600 bg-blue-600 text-white cursor-pointer hover:bg-blue-700 disabled:opacity-50 disabled:cursor-default';

export function ConfigPanel({connState, visible, configProps, signalCatalog, selectedSignals, setSelectedSignals, setOperation, addToast, onConfigReadback, speed}: Props) {
    const connected = connState === 'connected';
    const [timeMode, setTimeMode] = useState<'' | 'mobile' | 'survey' | 'fixed'>('');
    const [surveyTime, setSurveyTime] = useState('');
    const [surveyAcc, setSurveyAcc] = useState('');
    const [surveyAgain, setSurveyAgain] = useState(false);
    const [surveyReport, setSurveyReport] = useState(true);
    const [coordSystem, setCoordSystem] = useState<'ecef' | 'llh'>('ecef');
    const [fixedECEF, setFixedECEF] = useState<[string, string, string]>(['', '', '']);
    const [fixedLLH, setFixedLLH] = useState<[string, string, string]>(['', '', '']);
    const [fixedPosAcc, setFixedPosAcc] = useState('');
    // Track whether readback showed static:true with no position (stationary info)
    const [readbackStationary, setReadbackStationary] = useState(false);
    const [ppsPeriod, setPpsPeriod] = useState('');
    const [ppsWidth, setPpsWidth] = useState('');
    const [ppsAlign, setPpsAlign] = useState(true);
    const [ppsLocked, setPpsLocked] = useState(true);
    const [ppsRising, setPpsRising] = useState(true);
    const [timeGNSS, setTimeGNSS] = useState('');
    const [cableDelay, setCableDelay] = useState('');
    const [minElev, setMinElev] = useState('');
    const [showPicker, setShowPicker] = useState(false);
    const [ecefOnEarth, setEcefOnEarth] = useState(true);

    // Check ECEF coordinates against Earth surface when all three are valid numbers
    useEffect(() => {
        const nums = fixedECEF.map(Number);
        if (fixedECEF.some(s => s === '' || isNaN(Number(s)))) { setEcefOnEarth(true); return; }
        let cancelled = false;
        CheckOnEarth(nums[0], nums[1], nums[2]).then(ok => { if (!cancelled) setEcefOnEarth(ok); });
        return () => { cancelled = true; };
    }, [fixedECEF]);

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
    const [satsChange, setSatsChange] = useState(false);
    const [satsFlags, setSatsFlags] = useState(0);
    const [rawChange, setRawChange] = useState(false);
    const [rawFlags, setRawFlags] = useState(0);

    // Persistent operations state
    const [saveType, setSaveType] = useState(0); // 0=none, 1=minimal, 2=all
    const [resetType, setResetType] = useState(0); // 0=none, 1=reload, 2=cold, 3=factory

    // Readback state
    const [reading, setReading] = useState(false);
    const hasReadback = useRef(false);

    // Per-section touched flags for dirty tracking
    const [timePulseTouched, setTimePulseTouched] = useState(false);
    const [timeModeTouched, setTimeModeTouched] = useState(false);
    const [signalsTouched, setSignalsTouched] = useState(false);
    const [otherTouched, setOtherTouched] = useState(false);

    // Applying state
    const [applying, setApplying] = useState(false);

    // Populate form fields from ConfigProps JSON
    const populateFromConfig = useCallback((cfg: Record<string, any>) => {
        const m = cfg.mode as Record<string, any> | undefined;
        if (m) {
            if (!m.static) {
                setTimeMode('mobile');
                setReadbackStationary(false);
            } else if (m.fixedPosECEF) {
                setTimeMode('fixed');
                setCoordSystem('ecef');
                const ecef = m.fixedPosECEF as number[];
                setFixedECEF([String(ecef[0]), String(ecef[1]), String(ecef[2])]);
                setReadbackStationary(false);
            } else if (m.fixedPosLLH) {
                setTimeMode('fixed');
                setCoordSystem('llh');
                const llh = m.fixedPosLLH as number[];
                setFixedLLH([String(llh[0]), String(llh[1]), String(m.height ?? 0)]);
                setReadbackStationary(false);
            } else {
                // static with no position
                setTimeMode('');
                setReadbackStationary(true);
            }
            if (m.fixedPosAcc !== undefined) setFixedPosAcc(String(m.fixedPosAcc));
        }
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
        setOperation({status: 'running', label: 'Reading configuration'});
        try {
            const props = await ReadConfig();
            populateFromConfig(props as any);
            onConfigReadback(props as any);
            setTimePulseTouched(false);
            setTimeModeTouched(false);
            setSignalsTouched(false);
            setOtherTouched(false);
            setNmeaChange(false);
            setRtcmChange(false);
            setPvtChange(false);
            setSatsChange(false);
            setRawChange(false);
            setSaveType(0);
            setResetType(0);
            setOperation({status: 'success', label: 'Reading configuration'});
        } catch (e: any) {
            addToast('Readback error: ' + e.message, 'error');
            setOperation({status: 'failed', label: 'Reading configuration', error: e.message});
        } finally {
            setReading(false);
        }
    };

    const handleApply = async () => {
        const props: Record<string, any> = {};
        const opts: Record<string, any> = {};
        if (signalsTouched && selectedSignals.size > 0) {
            props.signalsEnabled = signalSetToMap(selectedSignals);
        }
        if (timeModeTouched) {
            if (timeMode === 'mobile') {
                props.mode = {static: false};
            } else if (timeMode === 'survey') {
                props.mode = {static: true};
                const dur = surveyTime !== '' ? parseFloat(surveyTime) : 2000;
                const acc = surveyAcc !== '' ? parseFloat(surveyAcc) : 20;
                opts.Survey = {
                    Flags: surveyAgain ? 1 : 0,
                    MinDur: dur * 1e9,       // seconds -> nanoseconds
                    AccLimit: acc * 1e6,     // meters -> micrometers
                };
                if (surveyReport) opts.PVTMsg = (opts.PVTMsg || 0) | PVTMsgSurvey;
            } else if (timeMode === 'fixed') {
                if (coordSystem === 'ecef') {
                    props.mode = {
                        static: true,
                        fixedPosECEF: fixedECEF.map(v => parseFloat(v) || 0),
                        fixedPosAcc: fixedPosAcc !== '' ? parseFloat(fixedPosAcc) : 20,
                    };
                } else {
                    props.mode = {
                        static: true,
                        fixedPosLLH: [parseFloat(fixedLLH[0]) || 0, parseFloat(fixedLLH[1]) || 0],
                        height: parseFloat(fixedLLH[2]) || 0,
                        fixedPosAcc: fixedPosAcc !== '' ? parseFloat(fixedPosAcc) : 20,
                    };
                }
            }
        }
        if (timePulseTouched) {
            props.timePulse = {
                period: parseFloat(ppsPeriod) || 0,
                width: parseFloat(ppsWidth) || 0,
                alignToGNSS: ppsAlign,
                onlyWhenLocked: ppsLocked,
                polarityRising: ppsRising,
            };
            if (timeGNSS) props.timeGNSS = timeGNSS;
            if (cableDelay !== '') props.antennaCableDelay = (parseFloat(cableDelay) || 0) * 1e-9;
        }
        if (otherTouched) {
            if (minElev !== '') props.minElevation = parseFloat(minElev) || 0;
        }
        const nmeaWire = nmeaWireValue(nmeaChange, nmeaDisable, nmeaFlags);
        if (nmeaWire !== undefined) opts.NMEAMsg = nmeaWire;
        const rtcmWire = rtcmWireValue(rtcmChange, rtcmDisable, rtcmMSM, rtcmFallback, rtcmARP);
        if (rtcmWire !== undefined) opts.RTCMMsg = rtcmWire;
        const pvtWire = pvtWireValue(pvtChange, pvtFlags);
        if (pvtWire !== undefined) opts.PVTMsg = (opts.PVTMsg || 0) | pvtWire;
        const satsWire = satsWireValue(satsChange, satsFlags);
        if (satsWire !== undefined) opts.SatsMsg = satsWire;
        const rawWire = rawWireValue(rawChange, rawFlags);
        if (rawWire !== undefined) opts.RawMsg = rawWire;
        if (saveType) opts.Save = saveType;
        if (resetType) opts.Reset = resetType;
        const cfg: Record<string, any> = {Props: props, Opts: opts};
        setApplying(true);
        setOperation({status: 'running', label: 'Applying configuration'});
        const r = await ApplyConfig(cfg as any);
        setApplying(false);
        setSaveType(0);
        setResetType(0);
        // Reset survey one-shot fields
        setSurveyTime('');
        setSurveyAcc('');
        setSurveyAgain(false);
        setSurveyReport(true);
        if (r.ok) {
            addToast('Configuration applied', 'success');
            setOperation({status: 'success', label: 'Applying configuration'});
            setTimePulseTouched(false);
            setTimeModeTouched(false);
            setSignalsTouched(false);
            setOtherTouched(false);
        } else {
            addToast(r.error || 'Apply failed', 'error');
            setOperation({status: 'failed', label: 'Applying configuration', error: r.error || 'Apply failed'});
        }
    };

    const handleDiscard = () => {
        if (configProps) populateFromConfig(configProps);
        setTimePulseTouched(false);
        setTimeModeTouched(false);
        setSignalsTouched(false);
        setOtherTouched(false);
        setNmeaChange(false);
        setRtcmChange(false);
        setPvtChange(false);
        setSatsChange(false);
        setRawChange(false);
        setSaveType(0);
        setResetType(0);
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
    const errorSet = validateFields(timeMode, coordSystem, surveyTime, surveyAcc, fixedECEF, fixedLLH, fixedPosAcc, ppsPeriod, ppsWidth, cableDelay, minElev);
    const ecefBad = timeMode === 'fixed' && coordSystem === 'ecef' && !ecefOnEarth;
    const hasErrors = errorSet.size > 0 || ecefBad;
    const pendingSections: string[] = [];
    if (timePulseTouched) pendingSections.push('time pulse');
    if (timeModeTouched) pendingSections.push('time mode');
    if (signalsTouched) pendingSections.push('signals');
    if (otherTouched) pendingSections.push('other');
    if (nmeaChange || rtcmChange || pvtChange || satsChange || rawChange) pendingSections.push('messages');
    if (saveType) pendingSections.push('save');
    if (resetType) pendingSections.push('reset');
    const pendingLabel = pendingSections.length > 0
        ? 'Changes pending to ' + pendingSections.join(', ')
        : '';
    const surveyDisabled = !connected || timeMode !== 'survey';
    const fixedDisabled = !connected || timeMode !== 'fixed';
    const ecefDisabled = fixedDisabled || coordSystem !== 'ecef';
    const llhDisabled = fixedDisabled || coordSystem !== 'llh';
    const disabledText = 'text-gray-400 dark:text-gray-500';
    const enabledText = 'text-gray-700 dark:text-gray-200 cursor-pointer';

    return (
        <div class="flex flex-col h-full">
            {/* Scrollable content */}
            <div class="flex-1 overflow-y-auto p-4">

                    <CollapsibleSection title="Time pulse" defaultOpen={true}>
                        <div class="flex gap-x-6 items-start">
                            <div class="grid grid-cols-[auto_auto] gap-x-4 gap-y-1.5 items-center">
                                <label class="text-xs text-gray-500 dark:text-gray-400">Period (s)</label>
                                <input type="text" inputMode="decimal" class={(errorSet.has('ppsPeriod') ? inputErrorClass : inputClass) + ' w-20'} value={ppsPeriod} placeholder="e.g. 1.0"
                                    disabled={!connected} onInput={e => { setTimePulseTouched(true); setPpsPeriod((e.target as HTMLInputElement).value); }} />
                                <label class="text-xs text-gray-500 dark:text-gray-400">Pulse width (s)</label>
                                <input type="text" inputMode="decimal" class={(errorSet.has('ppsWidth') ? inputErrorClass : inputClass) + ' w-20'} value={ppsWidth} placeholder="e.g. 0.1"
                                    disabled={!connected} onInput={e => { setTimePulseTouched(true); setPpsWidth((e.target as HTMLInputElement).value); }} />
                                <label class="text-xs text-gray-500 dark:text-gray-400">Time GNSS</label>
                                <select class={inputClass + ' w-24'} value={timeGNSS} disabled={!connected}
                                    onChange={e => { setTimePulseTouched(true); setTimeGNSS((e.target as HTMLSelectElement).value); }}>
                                    <option value="">--</option>
                                    <option value="GPS">GPS</option>
                                    <option value="GAL">Galileo</option>
                                    <option value="BDS">BeiDou</option>
                                    <option value="GLO">GLONASS</option>
                                </select>
                                <label class="text-xs text-gray-500 dark:text-gray-400">Cable delay (ns)</label>
                                <input type="text" inputMode="decimal" class={(errorSet.has('cableDelay') ? inputErrorClass : inputClass) + ' w-20'} value={cableDelay} placeholder="e.g. 50"
                                    disabled={!connected} onInput={e => { setTimePulseTouched(true); setCableDelay((e.target as HTMLInputElement).value); }} />
                            </div>
                            <div class="flex flex-col gap-1.5 pt-0.5">
                                <label class={`flex items-center gap-1.5 text-xs ${!connected ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                                    <input type="checkbox" class="accent-blue-600" checked={ppsAlign} disabled={!connected}
                                        onChange={e => { setTimePulseTouched(true); setPpsAlign((e.target as HTMLInputElement).checked); }} />
                                    Align to GNSS
                                </label>
                                <label class={`flex items-center gap-1.5 text-xs ${!connected ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                                    <input type="checkbox" class="accent-blue-600" checked={ppsLocked} disabled={!connected}
                                        onChange={e => { setTimePulseTouched(true); setPpsLocked((e.target as HTMLInputElement).checked); }} />
                                    Only when locked
                                </label>
                                <label class={`flex items-center gap-1.5 text-xs ${!connected ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                                    <input type="checkbox" class="accent-blue-600" checked={ppsRising} disabled={!connected}
                                        onChange={e => { setTimePulseTouched(true); setPpsRising((e.target as HTMLInputElement).checked); }} />
                                    Rising edge
                                </label>
                            </div>
                        </div>
                    </CollapsibleSection>

                    {/* Time mode subgroup */}
                    <CollapsibleSection title="Time mode" defaultOpen={true}>
                        {/* Mode radio group */}
                        <div class="flex flex-wrap gap-x-4 gap-y-1 mb-3">
                            {([['mobile', 'Mobile'], ['survey', 'Survey-in'], ['fixed', 'Fixed position']] as const).map(([val, label]) => (
                                <label key={val} class={`flex items-center gap-1.5 text-xs ${!connected ? disabledText : enabledText}`}>
                                    <input type="radio" name="timeMode" class="accent-blue-600" checked={timeMode === val}
                                        disabled={!connected} onChange={() => { setTimeModeTouched(true); setTimeMode(val); }} />
                                    {label}
                                </label>
                            ))}
                        </div>
                        {readbackStationary && timeMode === '' && (
                            <div class="text-[10px] text-blue-500 mb-2">Receiver is in stationary mode.</div>
                        )}

                        {/* Survey-in group */}
                        <div class="mb-3">
                            <div class={`text-xs font-semibold mb-1 ${surveyDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-600 dark:text-gray-300'}`}>Survey-in</div>
                            <div class="flex gap-x-6 items-start">
                                <div class="grid grid-cols-[auto_auto] gap-x-4 gap-y-1.5 items-center">
                                    <label class={`text-xs ${surveyDisabled ? disabledText : 'text-gray-500 dark:text-gray-400'}`}>Survey time (s)</label>
                                    <input type="text" inputMode="decimal" class={(errorSet.has('surveyTime') ? inputErrorClass : inputClass) + ' w-24'} value={surveyTime} placeholder="2000"
                                        disabled={surveyDisabled} onInput={e => { setTimeModeTouched(true); setSurveyTime((e.target as HTMLInputElement).value); }} />
                                    <label class={`text-xs ${surveyDisabled ? disabledText : 'text-gray-500 dark:text-gray-400'}`}>Survey accuracy (m)</label>
                                    <input type="text" inputMode="decimal" class={(errorSet.has('surveyAcc') ? inputErrorClass : inputClass) + ' w-24'} value={surveyAcc} placeholder="20"
                                        disabled={surveyDisabled} onInput={e => { setTimeModeTouched(true); setSurveyAcc((e.target as HTMLInputElement).value); }} />
                                </div>
                                <div class="flex flex-col gap-1.5 pt-0.5">
                                    <label class={`flex items-center gap-1.5 text-xs ${surveyDisabled ? disabledText : enabledText}`}>
                                        <input type="checkbox" class="accent-blue-600" checked={surveyAgain}
                                            disabled={surveyDisabled} onChange={e => { setTimeModeTouched(true); setSurveyAgain((e.target as HTMLInputElement).checked); }} />
                                        Do a new survey
                                    </label>
                                </div>
                            </div>
                            <label class={`flex items-center gap-1.5 text-xs mt-1.5 ${surveyDisabled ? disabledText : enabledText}`}>
                                <input type="checkbox" class="accent-blue-600" checked={surveyReport}
                                    disabled={surveyDisabled} onChange={e => { setTimeModeTouched(true); setSurveyReport((e.target as HTMLInputElement).checked); }} />
                                Report survey progress
                            </label>
                        </div>

                        {/* Fixed position group */}
                        <div>
                            <div class={`text-xs font-semibold mb-1 ${fixedDisabled ? 'text-gray-400 dark:text-gray-500' : 'text-gray-600 dark:text-gray-300'}`}>Fixed position</div>
                            <div class="flex flex-wrap gap-x-4 gap-y-1 mb-2">
                                {([['ecef', 'ECEF'], ['llh', 'Lat/Lon/Height']] as const).map(([val, label]) => (
                                    <label key={val} class={`flex items-center gap-1.5 text-xs ${fixedDisabled ? disabledText : enabledText}`}>
                                        <input type="radio" name="coordSystem" class="accent-blue-600" checked={coordSystem === val}
                                            disabled={fixedDisabled} onChange={() => { setTimeModeTouched(true); setCoordSystem(val); }} />
                                        {label}
                                    </label>
                                ))}
                            </div>
                            <div class="flex gap-x-8 items-start">
                                {/* ECEF column + on-Earth indicator */}
                                <div class="flex items-stretch gap-1.5">
                                    <div class="grid grid-cols-[auto_auto] gap-x-3 gap-y-1.5 items-center">
                                        {(['X (m)', 'Y (m)', 'Z (m)'] as const).map((label, i) => {
                                            const field = (['ecefX', 'ecefY', 'ecefZ'] as const)[i];
                                            return [
                                                <label key={`l${i}`} class={`text-xs ${ecefDisabled ? disabledText : 'text-gray-500 dark:text-gray-400'}`}>{label}</label>,
                                                <input key={`v${i}`} type="text" inputMode="decimal" class={(errorSet.has(field) ? inputErrorClass : inputClass) + ' w-28'}
                                                    value={fixedECEF[i]} disabled={ecefDisabled}
                                                    title={ecefBad ? 'ECEF coordinates not on Earth' : undefined}
                                                    onInput={e => { setTimeModeTouched(true); const v = [...fixedECEF] as [string, string, string]; v[i] = (e.target as HTMLInputElement).value; setFixedECEF(v); }} />,
                                            ];
                                        })}
                                    </div>
                                    {ecefBad && <div class="w-0.5 bg-red-400 rounded-full" title="ECEF coordinates not on Earth" />}
                                </div>
                                {/* LLH column */}
                                <div class="grid grid-cols-[auto_auto] gap-x-3 gap-y-1.5 items-center">
                                    {(['Latitude (deg)', 'Longitude (deg)', 'Height (m)'] as const).map((label, i) => {
                                        const field = (['llhLat', 'llhLon', 'llhHeight'] as const)[i];
                                        return [
                                            <label key={`l${i}`} class={`text-xs ${llhDisabled ? disabledText : 'text-gray-500 dark:text-gray-400'}`}>{label}</label>,
                                            <input key={`v${i}`} type="text" inputMode="decimal" class={(errorSet.has(field) ? inputErrorClass : inputClass) + ' w-28'}
                                                value={fixedLLH[i]} disabled={llhDisabled}
                                                onInput={e => { setTimeModeTouched(true); const v = [...fixedLLH] as [string, string, string]; v[i] = (e.target as HTMLInputElement).value; setFixedLLH(v); }} />,
                                        ];
                                    })}
                                </div>
                            </div>
                            <div class="grid grid-cols-[auto_auto] gap-x-3 gap-y-1.5 items-center mt-2 w-fit">
                                <label class={`text-xs ${fixedDisabled ? disabledText : 'text-gray-500 dark:text-gray-400'}`}>Position accuracy (m)</label>
                                <input type="text" inputMode="decimal" class={(errorSet.has('fixedPosAcc') ? inputErrorClass : inputClass) + ' w-24'} value={fixedPosAcc} placeholder="20"
                                    disabled={fixedDisabled} onInput={e => { setTimeModeTouched(true); setFixedPosAcc((e.target as HTMLInputElement).value); }} />
                            </div>
                        </div>
                    </CollapsibleSection>

                    {/* Signals subgroup */}
                    <CollapsibleSection title="Signals" defaultOpen={true}>
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
                                            onChange={e => { setSignalsTouched(true); toggleConstellation(gnssName, sigs, (e.target as HTMLInputElement).checked); }}
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
                        <div class="grid grid-cols-[auto_auto] gap-x-4 gap-y-1.5 items-center w-fit">
                            <label class="text-xs text-gray-500 dark:text-gray-400">Min elevation (deg)</label>
                            <input type="text" inputMode="decimal" class={(errorSet.has('minElev') ? inputErrorClass : inputClass) + ' w-20'} value={minElev} placeholder="e.g. 10"
                                disabled={!connected} onInput={e => { setOtherTouched(true); setMinElev((e.target as HTMLInputElement).value); }} />
                        </div>
                    </CollapsibleSection>

                {/* Messages */}
                <CollapsibleSection title="Messages" defaultOpen={true}>
                    <div class="flex flex-col gap-3">
                        <div class="flex gap-2">
                            <button class={btnClass} disabled={!connected} onClick={() => {
                                setNmeaChange(true); setNmeaDisable(false); setNmeaFlags(NMEAMsgRMC);
                                setRtcmChange(true); setRtcmDisable(true);
                                setPvtChange(true); setPvtFlags(PVTMsgOff);
                                setSatsChange(true); setSatsFlags(0);
                                setRawChange(true); setRawFlags(0);
                            }}>Minimum</button>
                            <button class={btnClass} disabled={!connected} onClick={() => {
                                setNmeaChange(true); setNmeaDisable(true);
                                setPvtChange(true); setPvtFlags(PVTMsgTimePulse | PVTMsgTimePulseAfter | PVTMsgTAI | PVTMsgLeapSecond | PVTMsgOff);
                                if (speed >= 19200) { setSatsChange(true); setSatsFlags(SatsMsgSat | SatsMsgSignal); }
                            }}>Daemon</button>
                        </div>
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
                        <SatsGroup
                            change={satsChange}
                            flags={satsFlags}
                            onChangeChange={setSatsChange}
                            onFlagsChange={setSatsFlags}
                            disabled={!connected}
                        />
                        <RawGroup
                            change={rawChange}
                            flags={rawFlags}
                            onChangeChange={setRawChange}
                            onFlagsChange={setRawFlags}
                            disabled={!connected}
                        />
                    </div>
                </CollapsibleSection>

                {/* Persistent operations */}
                <CollapsibleSection title="Persistent operations" defaultOpen={true}>
                    <div class="flex flex-col gap-3">
                        <div>
                            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">Save</div>
                            <div class="flex flex-wrap gap-x-4 gap-y-1 ml-0.5">
                                {([
                                    [0, 'Nothing'],
                                    [1, 'Changes'],
                                    [2, 'All'],
                                ] as [number, string][]).map(([v, label]) => (
                                    <label key={v} class={`flex items-center gap-1.5 text-xs ${!connected ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                                        <input type="radio" name="saveType" class="accent-blue-600" value={v} checked={saveType === v}
                                            disabled={!connected} onChange={() => setSaveType(v)} />
                                        {label}
                                    </label>
                                ))}
                            </div>
                        </div>
                        <div>
                            <div class="text-xs font-semibold text-gray-600 dark:text-gray-300 mb-1">Reset</div>
                            <div class="flex flex-wrap gap-x-4 gap-y-1 ml-0.5">
                                {([
                                    [0, 'None'],
                                    [1, 'Reload'],
                                    [2, 'Cold start'],
                                    [3, 'Factory reset'],
                                ] as [number, string][]).map(([v, label]) => (
                                    <label key={v} class={`flex items-center gap-1.5 text-xs ${!connected ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-200 cursor-pointer'}`}>
                                        <input type="radio" name="resetType" class="accent-blue-600" value={v} checked={resetType === v}
                                            disabled={!connected} onChange={() => setResetType(v)} />
                                        {label}
                                    </label>
                                ))}
                            </div>
                        </div>
                    </div>
                </CollapsibleSection>

            </div>

            {/* Bottom action bar */}
            <div class="shrink-0 flex items-center gap-2 px-4 py-2 border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                {pendingLabel && (
                    <span class="text-[10px] text-amber-500 font-medium">{pendingLabel}</span>
                )}
                <span class="ml-auto" />
                <button class={btnClass} disabled={!connected || !pendingLabel} onClick={handleDiscard}>
                    Discard
                </button>
                <button class={btnPrimary} disabled={!connected || hasErrors || applying || !pendingLabel} onClick={handleApply}>
                    {applying ? 'Applying...' : 'Apply'}
                </button>
            </div>

            {/* Signal picker dialog */}
            {showPicker && (
                <SignalPicker
                    signalCatalog={signalCatalog}
                    selectedSignals={selectedSignals}
                    onConfirm={(signals) => { setSignalsTouched(true); setSelectedSignals(() => signals); setShowPicker(false); }}
                    onCancel={() => setShowPicker(false)}
                />
            )}

        </div>
    );
}

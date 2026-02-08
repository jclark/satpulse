import {h, Fragment} from 'preact';
import {DetectReceiver} from '../wailsjs/go/main/App';
import {main} from '../wailsjs/go/models';
import type {OperationState} from './app';

interface Props {
    connected: boolean;
    receiverInfo: main.ReceiverInfo | null;
    setReceiverInfo: (info: main.ReceiverInfo | null) => void;
    setSelectedSignals: (fn: (prev: Set<number>) => Set<number>) => void;
    setStatusText: (s: string) => void;
    setLogEntries: (fn: (prev: any[]) => any[]) => void;
    setOperation: (op: OperationState) => void;
    addToast: (msg: string, type: 'success' | 'error') => void;
}

function formatSignalGroups(signals: string[][]): string {
    return signals.map(g => g[0] + '[' + g.slice(1).join(', ') + ']').join(', ');
}

export function ReceiverPanel({connected, receiverInfo, setReceiverInfo, setSelectedSignals, setStatusText, setLogEntries, setOperation, addToast}: Props) {
    const handleDetect = async () => {
        setLogEntries(() => []);
        setStatusText('Probing receiver...');
        setOperation({status: 'running', label: 'Detecting receiver'});
        const r = await DetectReceiver();
        if (!r.ok) {
            setReceiverInfo(null);
            setStatusText('Detection failed');
            setOperation({status: 'failed', label: 'Detecting receiver', error: r.error || 'Detection failed'});
            addToast(r.error || 'Detection failed', 'error');
            return;
        }
        setReceiverInfo(r);
        if (r.signalIndices?.length) {
            setSelectedSignals(() => new Set(r.signalIndices));
        }
        setStatusText('Receiver detected: ' + (r.vendor || 'unknown'));
        setOperation({status: 'success', label: 'Detecting receiver'});
    };

    const info = receiverInfo;
    const cfg = info?.config;
    const tp = cfg?.timePulse as Record<string, any> | undefined;

    const infoRows: [string, string][] = [];
    if (info) {
        if (info.vendor) infoRows.push(['Vendor', info.vendor]);
        if (info.hardware) infoRows.push(['Hardware', info.hardware]);
        if (info.firmware) infoRows.push(['Firmware', info.firmware]);
        if (info.supportedGNSS?.length) infoRows.push(['Supported GNSS', info.supportedGNSS.join(', ')]);
        if (info.packetFormats?.length) infoRows.push(['Packet formats', info.packetFormats.join(', ')]);
        if (info.signals?.length) infoRows.push(['Enabled signals', formatSignalGroups(info.signals)]);
        if (cfg) {
            if (cfg.mode) infoRows.push(['Mode', cfg.mode]);
            if (cfg.timeGNSS) infoRows.push(['Time GNSS', String(cfg.timeGNSS)]);
            if (cfg.ppsEnabled !== undefined) infoRows.push(['PPS', cfg.ppsEnabled ? 'Enabled' : 'Disabled']);
            if (tp) {
                if (tp.width !== undefined) infoRows.push(['PPS width', tp.width + ' s']);
                if (tp.period !== undefined) infoRows.push(['PPS period', tp.period + ' s']);
            }
            if (cfg.antennaCableDelay !== undefined) infoRows.push(['Cable delay', cfg.antennaCableDelay + ' ns']);
            if (cfg.minElevation !== undefined) infoRows.push(['Min elevation', cfg.minElevation + ' deg']);
            if (cfg.navMsgAuth) infoRows.push(['Nav msg auth', String(cfg.navMsgAuth)]);
        }
    }

    return (
        <div class="p-4 overflow-y-auto h-full">
            <div class="flex items-center gap-3 mb-4">
                <h2 class="text-base font-semibold">Receiver information</h2>
                <button
                    class="px-3.5 py-1 rounded text-xs border border-gray-200 dark:border-gray-700 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 cursor-pointer hover:bg-blue-600 hover:border-blue-600 hover:text-white disabled:opacity-50 disabled:cursor-default disabled:hover:bg-gray-200 dark:disabled:hover:bg-gray-700 disabled:hover:border-gray-200 dark:disabled:hover:border-gray-700 disabled:hover:text-gray-900 dark:disabled:hover:text-gray-100"
                    disabled={!connected}
                    onClick={handleDetect}
                >
                    Detect
                </button>
            </div>

            {!info && (
                <p class="text-gray-500 dark:text-gray-400 italic text-sm">
                    Connect to a GPS receiver, then click Detect.
                </p>
            )}

            {info && infoRows.length > 0 && (
                <dl class="grid grid-cols-[160px_1fr] gap-x-4 gap-y-2 max-w-xl">
                    {infoRows.map(([label, value]) => (
                        <>
                            <dt class="text-gray-500 dark:text-gray-400 text-xs">{label}</dt>
                            <dd class="text-sm">{value}</dd>
                        </>
                    ))}
                </dl>
            )}

        </div>
    );
}

import {h, Fragment} from 'preact';
import {useState, useEffect} from 'preact/hooks';
import {ECEFtoLLH} from '../wailsjs/go/main/App';
import type {SurveyMsg} from './app';

interface Props {
    msg: SurveyMsg | null;
}

interface LLH {
    lat: number;
    lon: number;
    height: number;
}

// umToM converts micrometers (int64 JSON number) to meters.
function umToM(um: number): number {
    return um / 1e6;
}

function formatCoord(lat: number, lon: number, digits: number): string {
    return `${lat.toFixed(digits)},${lon.toFixed(digits)}`;
}

function mapsURL(lat: number, lon: number): string {
    return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(formatCoord(lat, lon, 7))}`;
}

function formatDuration(ns: number): string {
    const secs = Math.floor(ns / 1e9);
    const h = Math.floor(secs / 3600);
    const m = Math.floor((secs % 3600) / 60);
    const s = secs % 60;
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
}

export function SurveyPanel({msg}: Props) {
    const [llh, setLLH] = useState<LLH | null>(null);

    useEffect(() => {
        if (!msg || !msg.position) {
            setLLH(null);
            return;
        }
        const [px, py, pz] = msg.position;
        if (px === 0 && py === 0 && pz === 0) {
            setLLH(null);
            return;
        }
        ECEFtoLLH(umToM(px), umToM(py), umToM(pz)).then(r => {
            if (r) setLLH(r);
        }).catch(() => setLLH(null));
    }, [msg?.position?.[0], msg?.position?.[1], msg?.position?.[2]]);

    if (!msg) {
        return (
            <div class="p-4 overflow-y-auto h-full">
                <h2 class="text-base font-semibold mb-4">Survey</h2>
                <p class="text-gray-500 dark:text-gray-400 italic text-sm">
                    Waiting for survey data...
                </p>
            </div>
        );
    }

    const rows: [string, string | preact.ComponentChildren][] = [];

    // Status
    if (msg.valid) {
        rows.push(['Status', 'Valid']);
    } else if (msg.inProgress) {
        rows.push(['Status', 'In progress']);
    }

    // Accuracy (micrometers to meters)
    if (msg.accuracy) {
        rows.push(['Accuracy', `${umToM(msg.accuracy).toFixed(4)} m`]);
    }

    // ECEF position
    if (msg.position) {
        const [px, py, pz] = msg.position;
        if (px !== 0 || py !== 0 || pz !== 0) {
            rows.push(['ECEF X', umToM(px).toFixed(4)]);
            rows.push(['ECEF Y', umToM(py).toFixed(4)]);
            rows.push(['ECEF Z', umToM(pz).toFixed(4)]);
        }
    }

    // Lat/lon from ECEFtoLLH
    if (llh) {
        rows.push(['Coordinates', (
            <a href={mapsURL(llh.lat, llh.lon)} target="_blank" class="underline hover:text-blue-500">
                {formatCoord(llh.lat, llh.lon, 5)}
            </a>
        )]);
        rows.push(['Altitude', `${llh.height.toFixed(2)} m`]);
    }

    // Observation count and time
    if (msg.obsCount) {
        rows.push(['Observations', String(msg.obsCount)]);
    }
    if (msg.obsTime) {
        rows.push(['Observation time', formatDuration(msg.obsTime)]);
    }

    return (
        <div class="p-4 overflow-y-auto h-full">
            <h2 class="text-base font-semibold mb-4">Survey</h2>
            {rows.length > 0 ? (
                <dl class="grid grid-cols-[140px_1fr] gap-x-4 gap-y-2 max-w-xl">
                    {rows.map(([label, value]) => (
                        <>
                            <dt class="text-gray-500 dark:text-gray-400 text-xs">{label}</dt>
                            <dd class="text-sm tabular-nums">{value}</dd>
                        </>
                    ))}
                </dl>
            ) : (
                <p class="text-gray-500 dark:text-gray-400 italic text-sm">
                    Waiting for survey data...
                </p>
            )}
        </div>
    );
}

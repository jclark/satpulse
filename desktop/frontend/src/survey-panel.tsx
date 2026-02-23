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

const blank = '\u2014';

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
        ECEFtoLLH(px, py, pz).then(r => {
            if (r) setLLH(r);
        }).catch(() => setLLH(null));
    }, [msg?.position?.[0], msg?.position?.[1], msg?.position?.[2]]);

    let status = blank, accuracy = blank;
    let ecefX = blank, ecefY = blank, ecefZ = blank;
    let coords: string | preact.ComponentChildren = blank;
    let altitude = blank, observations = blank, obsTime = blank;

    if (msg) {
        if (msg.valid) status = 'Valid';
        else if (msg.inProgress) status = 'In progress';

        if (msg.accuracy) accuracy = `${msg.accuracy.toFixed(4)} m`;

        if (msg.position) {
            const [px, py, pz] = msg.position;
            if (px !== 0 || py !== 0 || pz !== 0) {
                ecefX = px.toFixed(4);
                ecefY = py.toFixed(4);
                ecefZ = pz.toFixed(4);
            }
        }

        if (msg.obsCount) observations = String(msg.obsCount);
        if (msg.obsTime) obsTime = formatDuration(msg.obsTime);
    }

    if (llh) {
        coords = (
            <a href={mapsURL(llh.lat, llh.lon)} target="_blank" class="underline hover:text-blue-500">
                {formatCoord(llh.lat, llh.lon, 5)}
            </a>
        );
        altitude = `${llh.height.toFixed(2)} m`;
    }

    const rows: [string, string | preact.ComponentChildren][] = [
        ['Status', status],
        ['Accuracy', accuracy],
        ['ECEF X', ecefX],
        ['ECEF Y', ecefY],
        ['ECEF Z', ecefZ],
        ['Coordinates', coords],
        ['Altitude', altitude],
        ['Observations', observations],
        ['Observation time', obsTime],
    ];

    return (
        <dl class="grid grid-cols-[140px_1fr] gap-x-4 gap-y-2 max-w-xl">
            {rows.map(([label, value]) => (
                <>
                    <dt class="text-gray-500 dark:text-gray-400 text-xs">{label}</dt>
                    <dd class="text-sm tabular-nums">{value}</dd>
                </>
            ))}
        </dl>
    );
}

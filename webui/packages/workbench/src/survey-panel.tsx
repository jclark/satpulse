import {h} from 'preact';
import {useState, useEffect} from 'preact/hooks';
import {transport} from './transport';
import type {SurveyMsg} from './app';
import {MonitorDataView} from './ui';

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

function formatDuration(secs: number): string {
    return `${secs} s`;
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
        transport.ecefToLLH(px, py, pz).then(r => {
            if (r) setLLH(r);
        }).catch(() => setLLH(null));
    }, [msg?.position?.[0], msg?.position?.[1], msg?.position?.[2]]);

    let status = blank, accuracy = blank;
    let ecef = blank;
    let coords: string | preact.ComponentChildren = blank;
    let height = blank, observations = blank, obsTime = blank;

    if (msg) {
        if (msg.valid) status = 'Valid';
        else if (msg.inProgress) status = 'In progress';

        if (msg.accuracy) accuracy = `${msg.accuracy.toFixed(4)} m`;

        if (msg.position) {
            const [px, py, pz] = msg.position;
            if (px !== 0 || py !== 0 || pz !== 0) {
                ecef = `${px.toFixed(4)}, ${py.toFixed(4)}, ${pz.toFixed(4)}`;
            }
        }

        if (msg.obsCount) observations = String(msg.obsCount);
        if (msg.obsTime) obsTime = formatDuration(msg.obsTime);
    }

    if (llh) {
        coords = (
            <a href={mapsURL(llh.lat, llh.lon)} target="_blank" class="underline hover:text-accent">
                {formatCoord(llh.lat, llh.lon, 5)}
            </a>
        );
        height = `${llh.height.toFixed(2)} m`;
    }

    const rows: [string, string | preact.ComponentChildren][] = [
        ['Status', status],
        ['Accuracy', accuracy],
        ['Coordinates', coords],
        ['Height', height],
        ['ECEF', ecef],
        ['Observations', observations],
        ['Observation time', obsTime],
    ];

    return <MonitorDataView rows={rows.map(([label, value]) => ({label, value}))} class="max-w-xl grid-cols-[140px_1fr]" />;
}

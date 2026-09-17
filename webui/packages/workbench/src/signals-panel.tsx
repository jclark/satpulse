import {h} from 'preact';
import {useMemo, useState} from 'preact/hooks';
import type {SatellitesMsg, SVInfo} from './app';

interface Props {
    msg: SatellitesMsg | null;
}

type GnssKey = 'gps' | 'galileo' | 'beidou' | 'glonass' | 'qzss' | 'navic' | 'sbas' | 'unknown';

const GNSS_LABEL: Record<GnssKey, string> = {
    gps: 'GPS',
    galileo: 'GAL',
    beidou: 'BDS',
    glonass: 'GLO',
    qzss: 'QZSS',
    navic: 'NAVIC',
    sbas: 'SBAS',
    unknown: '?',
};

const GNSS_ORDER: GnssKey[] = ['gps', 'galileo', 'beidou', 'glonass', 'qzss', 'navic', 'sbas', 'unknown'];

const GNSS_BG: Record<GnssKey, string> = {
    gps: 'bg-gnss-gps',
    galileo: 'bg-gnss-galileo',
    beidou: 'bg-gnss-beidou',
    glonass: 'bg-gnss-glonass',
    qzss: 'bg-gnss-qzss',
    navic: 'bg-gnss-navic',
    sbas: 'bg-gnss-gps',
    unknown: 'bg-gnss-unknown',
};

const GNSS_RANK: Record<GnssKey, number> = GNSS_ORDER.reduce((acc, k, i) => {
    acc[k] = i;
    return acc;
}, {} as Record<GnssKey, number>);

function gnssKey(svid: string): GnssKey {
    switch (svid[0]) {
        case 'G': return 'gps';
        case 'S': return 'sbas';
        case 'E': return 'galileo';
        case 'C': return 'beidou';
        case 'R': return 'glonass';
        case 'J': return 'qzss';
        case 'I': return 'navic';
        default: return 'unknown';
    }
}

function svidNum(svid: string): number {
    const n = parseInt(svid.slice(1), 10);
    return isNaN(n) ? 0 : n;
}

const CN0_MAX = 55;

interface Row {
    svid: string;
    gnss: GnssKey;
    az: number | null;
    el: number | null;
    sigID: string;
    cn0: number;
    used: boolean;
}

function buildRows(msg: SatellitesMsg | null): Row[] {
    if (!msg?.info) return [];
    const usedFromSignal = msg.usedValidity === 2;
    const usedFromSV = msg.usedValidity === 1;
    const rows: Row[] = [];
    for (const sv of msg.info as SVInfo[]) {
        const az = sv.lookAngles ? sv.lookAngles.azimuth : null;
        const el = sv.lookAngles ? sv.lookAngles.elevation : null;
        const gnss = gnssKey(sv.id);
        for (const sig of sv.signals ?? []) {
            const used = usedFromSignal ? !!sig.used : usedFromSV ? !!sv.used : false;
            rows.push({
                svid: sv.id,
                gnss,
                az,
                el,
                sigID: sig.id ?? '',
                cn0: sig.cn0,
                used,
            });
        }
    }
    rows.sort((a, b) => {
        const g = GNSS_RANK[a.gnss] - GNSS_RANK[b.gnss];
        if (g !== 0) return g;
        const n = svidNum(a.svid) - svidNum(b.svid);
        if (n !== 0) return n;
        return a.sigID.localeCompare(b.sigID);
    });
    return rows;
}

export function SignalsPanel({msg}: Props) {
    const allRows = useMemo(() => buildRows(msg), [msg]);

    const presentGnss = useMemo(() => {
        const seen = new Set<GnssKey>();
        for (const r of allRows) seen.add(r.gnss);
        return GNSS_ORDER.filter(k => seen.has(k));
    }, [allRows]);

    const [excluded, setExcluded] = useState<Set<GnssKey>>(new Set());
    const [usedOnly, setUsedOnly] = useState(false);
    const usedValid = msg?.usedValidity === 1 || msg?.usedValidity === 2;

    const toggleGnss = (k: GnssKey) => {
        setExcluded(prev => {
            const next = new Set(prev);
            if (next.has(k)) next.delete(k);
            else next.add(k);
            return next;
        });
    };

    const filtered = useMemo(() => {
        return allRows.filter(r => {
            if (excluded.has(r.gnss)) return false;
            if (usedOnly && !r.used) return false;
            return true;
        });
    }, [allRows, excluded, usedOnly]);

    if (allRows.length === 0) {
        return <div class="text-xs text-text-muted">No satellite data</div>;
    }

    return (
        <div class="flex flex-col gap-2">
            <div class="flex flex-wrap items-center gap-2">
                <span class="text-xs text-text-secondary">Constellations:</span>
                {presentGnss.map(k => {
                    const on = !excluded.has(k);
                    return (
                        <button
                            key={k}
                            type="button"
                            onClick={() => toggleGnss(k)}
                            class={`inline-flex items-center gap-1.5 rounded border px-2 py-0.5 text-xs cursor-pointer ${
                                on
                                    ? 'border-border-strong bg-surface-2 text-text-primary'
                                    : 'border-border-subtle bg-transparent text-text-muted'
                            }`}
                        >
                            <span class={`inline-block w-2 h-2 rounded-full ${GNSS_BG[k]} ${on ? '' : 'opacity-40'}`} />
                            {GNSS_LABEL[k]}
                        </button>
                    );
                })}
                <label class={`ml-auto inline-flex items-center gap-1.5 text-xs ${usedValid ? 'text-text-primary cursor-pointer' : 'text-text-muted'}`}>
                    <input
                        type="checkbox"
                        checked={usedOnly}
                        disabled={!usedValid}
                        onChange={e => setUsedOnly((e.target as HTMLInputElement).checked)}
                        class="accent-accent"
                    />
                    Used only
                </label>
            </div>

            <table class="w-full text-xs tabular-nums">
                <thead>
                    <tr class="text-left text-text-secondary">
                        <th class="font-normal pr-3 py-1">GNSS</th>
                        <th class="font-normal pr-3 py-1">SV</th>
                        <th class="font-normal pr-3 py-1 whitespace-nowrap">Signal</th>
                        {usedValid && <th class="font-normal pr-3 py-1 text-center">Used</th>}
                        <th class="font-normal pr-3 py-1 text-right">Azim</th>
                        <th class="font-normal pr-3 py-1 text-right">Elev</th>
                        <th class="font-normal pr-3 py-1 text-right">CN0</th>
                        <th class="font-normal py-1 w-full"></th>
                    </tr>
                </thead>
                <tbody>
                    {filtered.map(r => {
                        const widthPct = Math.min(100, (r.cn0 / CN0_MAX) * 100);
                        return (
                            <tr
                                key={`${r.svid}|${r.sigID}`}
                                class="border-t border-border-subtle"
                            >
                                <td class="pr-3 py-0.5 text-text-primary">
                                    <span class="inline-flex items-center gap-1.5">
                                        <span class={`inline-block w-2 h-2 rounded-full ${GNSS_BG[r.gnss]}`} />
                                        {GNSS_LABEL[r.gnss]}
                                    </span>
                                </td>
                                <td class="pr-3 py-0.5 text-text-primary">{r.svid.slice(1)}</td>
                                <td class="pr-3 py-0.5 text-text-primary whitespace-nowrap">{r.sigID || '-'}</td>
                                {usedValid && (
                                    <td class="pr-3 py-0.5 text-text-primary text-center">{r.used ? '✓' : ''}</td>
                                )}
                                <td class="pr-3 py-0.5 text-text-primary text-right">
                                    {r.az != null ? r.az.toFixed(0) + '°' : ''}
                                </td>
                                <td class="pr-3 py-0.5 text-text-primary text-right">
                                    {r.el != null ? r.el.toFixed(0) + '°' : ''}
                                </td>
                                <td class="pr-3 py-0.5 text-text-primary text-right">
                                    {r.cn0 > 0 ? r.cn0.toFixed(0) : ''}
                                </td>
                                <td class="py-0.5">
                                    <div class="h-2 w-full bg-surface-3 rounded-sm overflow-hidden">
                                        <div
                                            class={`h-full ${GNSS_BG[r.gnss]}`}
                                            style={{width: widthPct + '%'}}
                                        />
                                    </div>
                                </td>
                            </tr>
                        );
                    })}
                </tbody>
            </table>
        </div>
    );
}

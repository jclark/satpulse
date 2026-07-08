// SPARTN message descriptions from SPARTN 2.0.3, Table 6.1.

// Constellation by subtype for OCB (type 0) and HPAC (type 1) messages.
const CONSTELLATION: Record<number, string> = {
    0: 'GPS', 1: 'GLONASS', 2: 'Galileo', 3: 'BeiDou', 4: 'QZSS',
};

// Subtype names for EAS (type 4) messages.
const EAS_SUBTYPE: Record<number, string> = {
    0: 'Dynamic Key',
    1: 'Group Authentication',
};

// Subtype names for proprietary (type 120) messages.
const PROPRIETARY_SUBTYPE: Record<number, string> = {
    0: 'In-house Test',
    1: 'u-blox AG',
    2: 'Swift Navigation',
};

export interface SpartnInfo {
    description: string;
}

// gnss appends the constellation name to an acronym, e.g. "GPS Orbit, Clock,
// Bias (OCB)". An unknown subtype yields just the acronym expansion.
function gnss(full: string, acronym: string, st: number): string {
    const c = CONSTELLATION[st];
    return c ? `${c} ${full} (${acronym})` : `${full} (${acronym})`;
}

/** Return a description for a SPARTN "type.subtype" message ID. */
export function spartnInfo(msgID: string): SpartnInfo {
    const [t, st] = msgID.split('.').map(s => parseInt(s, 10));
    if (isNaN(t) || isNaN(st)) return {description: ''};
    switch (t) {
        case 0:
            return {description: gnss('Orbit, Clock, Bias', 'OCB', st)};
        case 1:
            return {description: gnss('High-precision Atmosphere', 'HPAC', st)};
        case 2:
            return {description: st === 0 ? 'Geographic Area Definition (GAD)' : ''};
        case 3:
            return {description: st === 0 ? 'Basic-precision Atmosphere (BPAC)' : ''};
        case 4:
            return {description: easDesc(st)};
        case 120:
            return {description: proprietaryDesc(st)};
    }
    return {description: ''};
}

function easDesc(st: number): string {
    const sub = EAS_SUBTYPE[st];
    return sub ? `Encryption/Authentication Support (EAS): ${sub}`
        : 'Encryption/Authentication Support (EAS)';
}

function proprietaryDesc(st: number): string {
    const sub = PROPRIETARY_SUBTYPE[st];
    return sub ? `Proprietary: ${sub}` : 'Proprietary';
}

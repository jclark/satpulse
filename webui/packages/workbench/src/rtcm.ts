// RTCM message descriptions from RTCM 10403 standard.

const MSM_BASES: [number, string][] = [
    [1070, 'GPS'], [1080, 'GLONASS'], [1090, 'Galileo'], [1100, 'SBAS'],
    [1110, 'QZSS'], [1120, 'BeiDou'], [1130, 'NavIC'],
];

// Description templates with {} as placeholder for constellation name.
const MSM_DESC: Record<number, string> = {
    1: 'Compact {} Pseudoranges',
    2: 'Compact {} PhaseRanges',
    3: 'Compact {} Pseudoranges and PhaseRanges',
    4: 'Full {} Pseudoranges and PhaseRanges plus CNR',
    5: 'Full {} Pseudoranges, PhaseRanges, PhaseRangeRate and CNR',
    6: 'Full {} Pseudoranges and PhaseRanges plus CNR (high resolution)',
    7: 'Full {} Pseudoranges, PhaseRanges, PhaseRangeRate and CNR (high resolution)',
};

const NON_MSM: Record<string, string> = {
    '1001': 'GPS Basic RTK, L1 Only',
    '1002': 'GPS Extended RTK, L1 Only',
    '1003': 'GPS Basic RTK, L1 & L2',
    '1004': 'GPS Extended RTK, L1 & L2',
    '1005': 'Stationary Antenna Reference Point, No Height Information',
    '1006': 'Stationary Antenna Reference Point, with Height Information',
    '1007': 'Antenna Descriptor',
    '1008': 'Antenna Descriptor & Serial Number',
    '1009': 'GLONASS Basic RTK, L1 Only',
    '1010': 'GLONASS Extended RTK, L1 Only',
    '1011': 'GLONASS Basic RTK, L1 & L2',
    '1012': 'GLONASS Extended RTK, L1 & L2',
    '1013': 'System Parameters',
    '1019': 'GPS Satellite Ephemeris Data',
    '1020': 'GLONASS Satellite Ephemeris Data',
    '1033': 'Receiver and Antenna Descriptors',
    '1041': 'NavIC Satellite Ephemeris Data',
    '1042': 'BeiDou Satellite Ephemeris Data',
    '1044': 'QZSS Satellite Ephemeris Data',
    '1045': 'Galileo F/NAV Satellite Ephemeris Data',
    '1046': 'Galileo I/NAV Satellite Ephemeris Data',
    '1230': 'GLONASS L1 and L2 Code-Phase Biases',
};

export interface RtcmInfo {
    description: string;
    msm: string; // e.g. "MSM4", empty for non-MSM
}

/** Return description and MSM type for an RTCM message type number. */
export function rtcmInfo(msgType: string): RtcmInfo {
    const n = parseInt(msgType, 10);
    if (isNaN(n)) return {description: '', msm: ''};
    for (const [base, name] of MSM_BASES) {
        const off = n - base;
        if (off >= 1 && off <= 7) {
            return {
                description: MSM_DESC[off].replace('{}', name),
                msm: `MSM${off}`,
            };
        }
    }
    return {description: NON_MSM[msgType] || '', msm: ''};
}

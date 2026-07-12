// Source: gps/gpsprot/configtarget.go -- keep spellings in sync.

// NMEA
export const NMEAMsgRMC = 'RMC';
export const NMEAMsgGGA = 'GGA';
export const NMEAMsgGSA = 'GSA';
export const NMEAMsgGSV = 'GSV';
export const NMEAMsgZDA = 'ZDA';
export const NMEAMsgVTG = 'VTG';
export const NMEAMsgGLL = 'GLL';
export const NMEAMsgOther = 'other';
export const NMEAMsgNames = [NMEAMsgRMC, NMEAMsgGGA, NMEAMsgGSA, NMEAMsgGSV, NMEAMsgZDA, NMEAMsgVTG, NMEAMsgGLL] as const;

// RTCM
export const RTCMMsgMSM4 = 'MSM4';
export const RTCMMsgMSM7 = 'MSM7';
export const RTCMMsgARP = 'ARP';
export const RTCMMsgLax = 'lax';
export const RTCMMsgOther = 'other';

// PVT
export const PVTMsgPos = 'pos';
export const PVTMsgVel = 'vel';
export const PVTMsgTime = 'time';
export const PVTMsgTimePulse = 'timePulse';
export const PVTMsgLeapSecond = 'leapSecond';
export const PVTMsgSurvey = 'survey';
export const PVTMsgTAI = 'tai';
export const PVTMsgECEF = 'ecef';
export const PVTMsgTimePulseAfter = 'timePulseAfter';
export const PVTMsgQuality = 'quality';
export const PVTMsgEpoch = 'epoch';
export const PVTMsgOff = 'off';
export const PVTMsgNames = [
    PVTMsgPos, PVTMsgVel, PVTMsgTime, PVTMsgTimePulse, PVTMsgLeapSecond, PVTMsgSurvey,
    PVTMsgTAI, PVTMsgECEF, PVTMsgTimePulseAfter, PVTMsgQuality, PVTMsgEpoch, PVTMsgOff,
] as const;

// Sats
export const SatsMsgSat = 'sat';
export const SatsMsgSignal = 'signal';
export const SatsMsgNames = [SatsMsgSat, SatsMsgSignal] as const;

// Raw
export const RawMsgObs = 'obs';
export const RawMsgNavData = 'navData';
export const RawMsgNames = [RawMsgObs, RawMsgNavData] as const;

export function msgFlag(name: string, names: readonly string[]): number {
    return 1 << names.indexOf(name);
}

export function msgFlagNames(flags: number, names: readonly string[]): string[] {
    return names.filter((name, i) => (flags & (1 << i)) !== 0);
}

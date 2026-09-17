import type {Angle, GNSS, Length} from './gpsprot';

export const NMEAMsgRMC = 'RMC';
export const NMEAMsgGGA = 'GGA';
export const NMEAMsgGSA = 'GSA';
export const NMEAMsgGSV = 'GSV';
export const NMEAMsgZDA = 'ZDA';
export const NMEAMsgVTG = 'VTG';
export const NMEAMsgGLL = 'GLL';
export const NMEAMsgOther = 'other';
export const NMEAMsgNames = [NMEAMsgRMC, NMEAMsgGGA, NMEAMsgGSA, NMEAMsgGSV, NMEAMsgZDA, NMEAMsgVTG, NMEAMsgGLL, NMEAMsgOther] as const;
export type NMEAMsgFlag = typeof NMEAMsgNames[number];
export type NMEAMsgFlags = NMEAMsgFlag[];

export const RTCMMsgMSM4 = 'MSM4';
export const RTCMMsgMSM7 = 'MSM7';
export const RTCMMsgARP = 'ARP';
export const RTCMMsgLax = 'lax';
export const RTCMMsgOther = 'other';
export const RTCMMsgNames = [RTCMMsgMSM4, RTCMMsgMSM7, RTCMMsgARP, RTCMMsgLax, RTCMMsgOther] as const;
export type RTCMMsgFlag = typeof RTCMMsgNames[number];
export type RTCMMsgFlags = RTCMMsgFlag[];

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
export type PVTMsgFlag = typeof PVTMsgNames[number];
export type PVTMsgFlags = PVTMsgFlag[];

export const SatsMsgSat = 'sat';
export const SatsMsgSignal = 'signal';
export const SatsMsgNames = [SatsMsgSat, SatsMsgSignal] as const;
export type SatsMsgFlag = typeof SatsMsgNames[number];
export type SatsMsgFlags = SatsMsgFlag[];

export const RawMsgObs = 'obs';
export const RawMsgNavData = 'navData';
export const RawMsgNames = [RawMsgObs, RawMsgNavData] as const;
export type RawMsgFlag = typeof RawMsgNames[number];
export type RawMsgFlags = RawMsgFlag[];

export const SurveyAgain = 'again';
export const SurveyFlagNames = [SurveyAgain] as const;
export type SurveyFlag = typeof SurveyFlagNames[number];
export type SurveyFlags = SurveyFlag[];

export const SaveNone = 'none';
export const SaveMinimal = 'minimal';
export const SaveAll = 'all';
export const SaveTypeNames = [SaveNone, SaveMinimal, SaveAll] as const;
export type SaveType = typeof SaveTypeNames[number];
export const ResetNone = 'none';
export const ResetReload = 'reload';
export const ResetCold = 'cold';
export const ResetFactory = 'factory';
export const ResetTypeNames = [ResetNone, ResetReload, ResetCold, ResetFactory] as const;
export type ResetType = typeof ResetTypeNames[number];

export const PropIDNames = [
    'signalsEnabled', 'timeGNSS', 'timePulse', 'timePulse.width', 'timePulse.period',
    'timePulse.alignToGNSS', 'timePulse.onlyWhenLocked', 'timePulse.polarityRising',
    'mode', 'antennaCableDelay', 'navMsgAuth', 'rtcmBaseID', 'minElevation', 'baudRate', 'port',
] as const;
export type PropID = typeof PropIDNames[number];
export type PropIDs = PropID[];

export type SignalMap = Record<string, string[]>;

export interface TimePulse {
    width?: number;
    period?: number;
    alignToGNSS?: boolean;
    onlyWhenLocked?: boolean;
    polarityRising?: boolean;
}

export interface Mode {
    static: boolean;
    fixedPosECEF?: [Length, Length, Length];
    fixedPosLLH?: [Angle, Angle];
    height?: Length;
    fixedPosAcc?: Length;
}

export type NavMsgAuth = 'none' | 'OSNMA';

export interface ConfigProps {
    signalsEnabled?: SignalMap;
    timeGNSS?: GNSS;
    timePulse?: TimePulse;
    mode?: Mode;
    antennaCableDelay?: number;
    navMsgAuth?: NavMsgAuth;
    rtcmBaseID?: number;
    minElevation?: Angle;
    baudRate?: number;
    port?: string;
}

export type ConfigTargetProps = Omit<ConfigProps, 'port'>;

export interface Survey {
    Flags: SurveyFlags;
    MinDur: number;
    AccLimit: Length;
}

export interface LeapSecondState {
    UTCOffset: number;
    LeapTonight: number;
}

export interface TimeEstimate {
    EstimatedTime: string;
    TimeOfEstimate: string;
    Accuracy: number;
    LeapSecond: LeapSecondState;
    Trusted: boolean;
}

export interface OSNMAOptions {
    MerkleTreeRoot: number[];
}

export interface ConfigOptions {
    Socket?: boolean;
    ForceProbe?: boolean;
    Save?: SaveType;
    Reset?: ResetType;
    PVTMsg?: PVTMsgFlags;
    NMEAMsg?: NMEAMsgFlags;
    RTCMMsg?: RTCMMsgFlags;
    SatsMsg?: SatsMsgFlags;
    RawMsg?: RawMsgFlags;
    Survey?: Survey;
    SetStatic?: boolean;
    TimeAssist?: TimeEstimate;
    OSNMA?: OSNMAOptions;
}

export interface ConfigTarget {
    Props?: ConfigTargetProps;
    Get?: PropIDs;
    Opts?: ConfigOptions;
}

// ConfigTargetVocabulary is used by the generated Go JSON fixture to
// compile-check every enum and property name, including shapes that cannot
// occur together in one ConfigTarget.
export interface ConfigTargetVocabulary {
    Props: ConfigProps[];
    Save: SaveType[];
    Reset: ResetType[];
    Get: PropIDs[];
}

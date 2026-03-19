import type {Time, UTCTime} from './ptime';

// Scalar value types

export type Length = number;
export type Angle = number;
export type Speed = number;
export type Duration = number;
export type GNSS = string;
export type SVID = string;
export type SignalID = string;
export type Tag = string;
export type FixLevel = string;
export type SolutionDim = string;
export type TimeRef = number;
export type SatelliteUsedValidity = number;
export type CorrKind = string[];
export type AuxSrc = string[];
export type GNSSSet = string[];
export type BandsUsed = string[];
export type StdDuration = number;
export type StdTime = string;

// Message interfaces

export interface LookAngles {
    azimuth: number;
    elevation: number;
}

export interface SignalInfo {
    id?: SignalID;
    cn0: number;
    used?: boolean;
}

export interface SVInfo {
    id: SVID;
    lookAngles?: LookAngles;
    signals: SignalInfo[];
    used?: boolean;
}

export interface SatellitesMsg {
    tag?: Tag;
    nativeMsgID?: string;
    info: SVInfo[];
    usedValidity?: SatelliteUsedValidity;
}

export interface TimeMsg {
    taiTime?: Time;
    utcTime?: UTCTime;
    accuracy?: StdDuration;
    utcOffset?: number;
    pulseOffset?: number;
    gnss?: GNSS;
    ref?: TimeRef;
    readDelay?: Duration;
    tag?: Tag;
    nativeMsgID?: string;
}

export interface SurveyMsg {
    position?: [Length, Length, Length];
    accuracy: Length;
    obsCount: number;
    obsTime: Duration;
    valid: boolean;
    inProgress: boolean;
}

export interface PosGeoMsg {
    latLon: [Angle, Angle];
    height?: Length;
    heightMSL?: Length;
    tag: Tag;
    nativeMsgID: string;
}

export interface PosECEFMsg {
    pos: [Length, Length, Length];
    tag: Tag;
    nativeMsgID: string;
}

export interface VelGeoMsg {
    velNED?: [Speed, Speed, Speed];
    groundSpeed?: Speed;
    speed3D?: Speed;
    course?: Angle;
    tag: Tag;
    nativeMsgID: string;
}

export interface VelECEFMsg {
    vel: [Speed, Speed, Speed];
    tag: Tag;
    nativeMsgID: string;
}

export interface PVMsgBundle {
    posGeo?: PosGeoMsg;
    posECEF?: PosECEFMsg;
    velGeo?: VelGeoMsg;
    velECEF?: VelECEFMsg;
}

export interface Accuracy {
    pos?: Length;
    hor?: Length;
    vert?: Length;
    speed?: Speed;
    groundSpeed?: Speed;
    course?: Angle;
}

export interface DOP {
    geom?: number;
    pos?: number;
    hor?: number;
    vert?: number;
    time?: number;
    north?: number;
    east?: number;
}

export interface NavEpochMsg {
    fixLevel?: FixLevel;
    solutionDim?: SolutionDim;
    correction?: CorrKind;
    auxSrc?: AuxSrc;
    acc?: Accuracy;
    dop?: DOP;
    diffAge?: Duration;
    rtcmRefBaseID?: number;
    numSVUsed?: number;
    numSVTracked?: number;
    numSVInView?: number;
    gnssUsed?: GNSSSet;
    bandsUsed?: BandsUsed;
    tag?: Tag;
}

export interface LeapSecondMsg {
    OffChangeTime: Time;
    UTCOffBefore: number;
    UTCOffAfter: number;
    gnss?: GNSS;
}

export interface ReceiverInfo {
    vendor: string;
    firmware: string;
    hardware: string;
    supportedGNSS: GNSSSet;
}

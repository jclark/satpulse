// The transport is the UI's interface to its backend. The universal
// core (Transport) covers snapshots, configuration, corrections,
// decoding, geodesy, and event subscription; every backend implements
// it. Connection management (ports, connect/disconnect) and
// message files are optional capabilities: a backend with a permanently
// connected receiver owns no port and implements neither.
//
// Implementations: fetch+SSE (satpulsewb) and Wails bindings (desktop).

import type {ConfigProps, ConfigTarget} from '@satpulse/gps/configtarget';

export type ConnState =
    | 'disconnected'
    | 'connecting'
    | 'reconnecting'
    | 'connected'
    | 'configuring'
    | 'sending';

export interface PortInfo {
    device: string;
    display: string;
}

// LLH is a geodetic position: latitude and longitude in degrees,
// height above the WGS84 ellipsoid in meters.
export interface LLH {
    lat: number;
    lon: number;
    height: number;
}

export interface DecodeOptions {
    hex: boolean; // data is hex-encoded binary
    out: boolean; // packet is outbound (sent to receiver)
}

export interface CorrectionSource {
    mode: string; // 'tcp' or 'ntrip'
    host: string;
    port: number;
    mountpoint: string; // ntrip only
    username: string;   // ntrip only, may be empty
    password: string;   // ntrip only, may be empty
    nmeaSend: boolean;  // ntrip only: upload position as NMEA GGA (VRS)
}

export interface MsgFileTag {
    tag: string;
    desc?: string;
    msgCount: number;
    needsPort?: boolean;
    saveAware?: boolean;
}

export interface MsgFileInfo {
    path: string;
    tags: MsgFileTag[];
}

// MsgFileEntry is one message file in the library catalog, identified
// by its vendor directory and file name (without the .toml extension);
// path is the file the name resolved to on the server's library search
// path, for display. Entries arrive in search order and unsorted:
// sorting and grouping are the UI's job.
export interface MsgFileEntry {
    vendor: string;
    file: string;
    path: string;
}

export interface MsgFileCatalog {
    names: MsgFileEntry[];
    preselect?: string; // vendor to preselect, or absent
}

// Transport methods reject with an Error whose message is suitable for
// display when an operation fails.
export interface Transport {
    // Snapshots, for initial sync and late-joining clients.
    getConnState(): Promise<ConnState>;
    getReceiverState(): Promise<any>;
    getCorrectionsState(): Promise<any>;
    getAllSignals(gnss: string[]): Promise<Record<string, string[]> | null>;

    // Configuration.
    readConfig(): Promise<ConfigProps>;
    applyConfig(target: ConfigTarget): Promise<void>;

    // Corrections.
    startCorrections(src: CorrectionSource): Promise<void>;
    stopCorrections(): Promise<void>;

    // Packet decoding.
    decodePacket(data: string, opts: DecodeOptions): Promise<Record<string, any> | null>;

    // Geodesy.
    ecefToLLH(x: number, y: number, z: number): Promise<LLH>;
    llhToECEF(lat: number, lon: number, height: number): Promise<[number, number, number]>;
    checkOnEarth(x: number, y: number, z: number): Promise<boolean>;
    velNEDtoECEF(n: number, e: number, d: number): Promise<[number, number, number] | null>;
    velECEFtoNED(x: number, y: number, z: number): Promise<[number, number, number] | null>;

    // eventsOn subscribes to a session event and returns an unsubscribe
    // function. Subscribing is what makes the backend stream a gated
    // high-rate event (gps:packet). The Workbench packet panel keeps that
    // subscription for its mounted lifetime so packets persist across tabs.
    eventsOn(name: string, cb: (data: any) => void): () => void;

    // openURL opens an external link.
    openURL(url: string): void;

    // reclaim takes the write seat for this window (the "Use here" action on a
    // read-only window). Optional: a backend with a single guaranteed window
    // (the desktop app) has no seat and omits it, so the window is always the
    // writer.
    reclaim?(): Promise<void>;

    // Optional capabilities.
    connection?: ConnectionTransport;
    msgFile?: MsgFileTransport;
}

// ConnectionTransport is the connection-management capability,
// implemented by backends that own the receiver port (direct serial,
// proxy connections).
export interface ConnectionTransport {
    connect(device: string, speed: number): Promise<void>;
    disconnect(): Promise<void>;
    listPorts(): Promise<PortInfo[]>;
}

// MsgFileTransport is the message-file capability. A backend obtains the
// file to load in whichever of two ways it supports: a native file dialog
// (desktop's loadMsgFile) or the server-side library catalog (the web's
// listMsgFiles/selectMsgFile). The panel renders the picker when the
// catalog pair is present, else the Open... button when loadMsgFile is.
export interface MsgFileTransport {
    loadMsgFile?(): Promise<MsgFileInfo | null>;
    listMsgFiles?(): Promise<MsgFileCatalog>;
    selectMsgFile?(vendor: string, file: string): Promise<MsgFileInfo>;
    sendMsgFile(tag: string, port: string, save: boolean): Promise<void>;
    cancelMsgSend(): Promise<void>;
}

// transport is the backend for this app instance, installed by the
// entry point before the first render.
export let transport: Transport;

export function setTransport(t: Transport) {
    transport = t;
}

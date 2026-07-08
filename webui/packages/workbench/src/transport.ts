// The transport is the UI's interface to its backend. The universal
// core (Transport) covers snapshots, configuration, corrections,
// decoding, geodesy, and event subscription; every backend implements
// it. Connection management (ports, vendors, connect/disconnect) and
// message files are optional capabilities: a backend with a permanently
// connected receiver owns no port and implements neither.
//
// Implementations: fetch+SSE (satpulsewb) and Wails bindings (desktop).

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

// Transport methods reject with an Error whose message is suitable for
// display when an operation fails.
export interface Transport {
    // Snapshots, for initial sync and late-joining clients.
    getConnState(): Promise<ConnState>;
    getReceiverState(): Promise<any>;
    getCorrectionsState(): Promise<any>;
    getAllSignals(gnss: string[]): Promise<Record<string, string[]> | null>;

    // Configuration.
    readConfig(): Promise<Record<string, any>>;
    applyConfig(target: {Props: Record<string, any>; Opts: Record<string, any>}): Promise<void>;

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
    // high-rate event (gps:packet), so subscribe only while displaying.
    eventsOn(name: string, cb: (data: any) => void): () => void;

    // openURL opens an external link.
    openURL(url: string): void;

    // Optional capabilities.
    connection?: ConnectionTransport;
    msgFile?: MsgFileTransport;
}

// ConnectionTransport is the connection-management capability,
// implemented by backends that own the receiver port (direct serial,
// proxy connections).
export interface ConnectionTransport {
    connect(device: string, speed: number, vendor: string): Promise<void>;
    disconnect(): Promise<void>;
    listPorts(): Promise<PortInfo[]>;
    listVendors(): Promise<string[]>;
}

// MsgFileTransport is the message-file capability.
export interface MsgFileTransport {
    loadMsgFile(): Promise<MsgFileInfo | null>;
    sendMsgFile(tag: string, port: string, save: boolean): Promise<void>;
    cancelMsgSend(): Promise<void>;
}

// transport is the backend for this app instance, installed by the
// entry point before the first render.
export let transport: Transport;

export function setTransport(t: Transport) {
    transport = t;
}

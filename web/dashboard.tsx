import { createContext, FunctionComponent } from 'preact';
import { useContext, useEffect, useState } from 'preact/hooks';
import { formatUTCLocal, formatNanoseconds, formatTAI, formatDateTime } from './timefmt';
import { SkyView, SVInfo, SignalGraph, simplifySignals } from './svg';

export const EventSourceContext = createContext<EventSource | null>(null);

// Define a type for JSON values
type JSONValue = string | number | boolean | null | JSONObject | JSONArray;
type JSONObject = { [key: string]: JSONValue };
type JSONArray = JSONValue[];

// Use a more specific type for our parsed event data
const EVENT_TYPES = ["satellites", "time", "phc", "mode", "survey", "receiver", "posvel", "quality", "corReport", "init"] as const;
type EventType = typeof EVENT_TYPES[number];

type Map = {[key: string]: any};

export const Dashboard: FunctionComponent = () => {
    const context = useContext(EventSourceContext) as EventSource;
    const [events, setEvents] = useState<Map>({});
    // phc accumulates fields from both `phc` and `mode` events as they
    // arrive. The shared `mode` field is therefore last-write-wins by
    // arrival order, so a stale cached mode delivered at connect is
    // corrected by the next live event from either stream.
    const [phc, setPhc] = useState<Map | null>(null);
    const [rtcm, setRTCM] = useState<RTCMState | null>(null);
    const [haveLookAngles, setHaveLookAngles] = useState(false);
    const [everMoving, setEverMoving] = useState(false);

    useEffect(() => {
        const handler = (type: string) => (e: MessageEvent<string>) => {
            const parsedEvents = parseSSEMessage(type, e.data);
            for (const [eventType, eventData] of parsedEvents) {
                const obj : Map|null = validateEvent(eventType, eventData);
                if (obj !== null) {
                    if (eventType === 'phc' || eventType === 'mode') {
                        setPhc(prev => ({ ...prev, ...obj }));
                        continue;
                    }
                    if (eventType === 'corReport') {
                        const ev = obj as RTCMEvent;
                        setRTCM(prev => {
                            const next = prev ? prev.clone() : new RTCMState(ev.source);
                            next.update(ev);
                            return next;
                        });
                        continue;
                    }
                    if (eventType === 'satellites' && obj.svs && obj.svs.length > 0) {
                        setHaveLookAngles(obj.svs.some((sv: any) => sv.lookAngles));
                    }
                    if (eventType === 'posvel' && typeof obj.groundSpeed === 'number' && obj.groundSpeed >= 0.1) {
                        setEverMoving(true);
                    }
                    setEvents(prev => ({ ...prev, [eventType]: obj }));
                }
            }
        };
        const listeners = EVENT_TYPES.map(type => [type, handler(type)] as const);
        for (const [type, listener] of listeners) {
            context.addEventListener(type, listener);
        }
        return () => {
            for (const [type, listener] of listeners) {
                context.removeEventListener(type, listener);
            }
        };
    }, []);
    
    const svs = events.satellites ? simplifySignals(events.satellites.svs) : [];
    
    return (
        <CardsElement>
        {events.satellites && haveLookAngles && <SkyViewCard svs={svs} />}
        {events.satellites && <SignalGraphCard svs={svs} />}
        {events.time && <PropertyCard title="Current GPS Time" data={events.time} format={timeFormat} />}
        {phc && <PropertyCard title="PTP Hardware Clock" data={phc} format={phcFormat} />}
        {events.receiver && <PropertyCard title="Receiver" data={events.receiver} format={receiverFormat} />}
        {events.quality && <PropertyCard title="Status" data={events.quality} format={statusFormat} />}
        {events.posvel && <PropertyCard title="Position" data={events.posvel} format={positionFormat} />}
        {events.posvel && everMoving && <PropertyCard title="Velocity" data={events.posvel} format={velocityFormat} />}
        {events.quality && <PropertyCard title="Position Quality" data={events.quality} format={positionQualityFormat} />}
        {events.survey && <PropertyCard title="Survey-in Status" data={events.survey} format={surveyFormat} />}
        {rtcm && <RTCMCard state={rtcm} />}
        </CardsElement>
    );
};

/**
* Parse an SSE event message and split init events into their components
* @param type Event type name
* @param data Raw JSON string from event
* @returns Array of [eventType, eventData] pairs
*/
function parseSSEMessage(type: string, data: string): [string, JSONValue][] {
    try {
        const parsed = JSON.parse(data) as JSONValue; // throws if invalid JSON
        if (type !== "init") {
            return [[type, parsed]];
        }
        
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
            console.warn(`Invalid init event data:`, parsed);
            return [];
        }
        
        const results: [string, JSONValue][] = [];
        for (const key in parsed) {
            if (key === "init") {
                console.warn("init event cannot have an init key in its data");
            } else {
                results.push([key, parsed[key]]);
            }
        }

        return results;
    } catch (err) {
        console.warn(`Error parsing ${type} event:`, err);
        return [];
    }
}

/**
* Validate event data for specific event types
* @param type Event type
* @param data Parsed event data
* @returns Validated data or null if invalid
*/
export function validateEvent(type: string, data: JSONValue): JSONObject | null {
    // Check that the type is one of the known types but not "init"
    if (type === "init" || !EVENT_TYPES.includes(type as EventType)) {
        console.warn(`Invalid event type: ${type}`);
        return null;
    }

    // Validate that data is a JSONObject
    if (!data || typeof data !== 'object' || Array.isArray(data)) {
        console.warn(`Invalid ${type} event data: not an object`, data);
        return null;
    }

    // Type-specific validation
    switch (type) {
        case "satellites":
            const svs = data.svs;
            if (!svs || !Array.isArray(svs)) {
                console.warn("Invalid satellites event: missing svs array", data);
                return null;
            }
            break;
        case "corReport":
            if (data.tag !== "RTCM") return null;
            if (typeof data.msgID !== "string" || data.msgID === "") return null;
            if (data.checksumOK === false) return null;
            if (data.source !== "pull" && data.source !== "receiver") return null;
            break;
    }

    return data;
}

interface CardsElementProps {
    children: preact.ComponentChildren;
}

const CardsElement: FunctionComponent<CardsElementProps> = ({ children }) => {
    return (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 p-4">
        {children}
        </div>
    );
};

interface CardElementProps {
    children: preact.ComponentChildren;
    title?: string;
}

const CardElement: FunctionComponent<CardElementProps> = ({ children, title }) => {
    return (
        <div className="p-4 rounded-lg mb-4 shadow-md break-inside-avoid bg-white dark:bg-gray-800 border-l-4 border-orange-500 h-full flex flex-col">
        {title && <h3 className="mt-0 mb-4 text-xl cursor-pointer text-blue-600 dark:text-blue-400">{title}</h3>}
        <div className="transition-all duration-300 overflow-hidden flex-grow">
        {children}
        </div>
        </div>
    );
};

interface FieldElementProps {
    children: preact.ComponentChildren;
    desc: string;
}

const FieldElement: FunctionComponent<FieldElementProps> = ({ children, desc }) => {
    return (
        <div className="flex justify-between text-base mb-2 text-gray-600 dark:text-gray-300">
        <span className="font-bold text-gray-800 dark:text-blue-200">{desc}:</span> 
        <span className="tabular-nums text-gray-900 dark:text-gray-100">{children}</span>
        </div>
    );
};


type PropertyCardProps = {
    title: string;
    data: Map;
    format: EventFormat;
};

const PropertyCard: FunctionComponent<PropertyCardProps> = ({ title, data, format }) => {
    const fields: FormattedField[] = [];
    addFields(fields, data, format);
    
    return (
        <CardElement title={title}>
        {fields.map(([desc, value]) => (
            <FieldElement desc={desc}>{value}</FieldElement>
        ))}
        </CardElement>
    );
};

type FormattedField = [string, any];

function addFields(fields: FormattedField[], state: Map, format: EventFormat) {
    for (const [key, f] of Object.entries(format)) {
        if (!(key in state)) {
            continue;
        }
        if (typeof f === 'function') {
            const formatted = f(state[key], state);
            fields.push(...formatted)
        }
        else {
            const [desc, formatter] = f;
            const val = state[key];
            const formatted = formatter ? formatter(val) : val;
            fields.push([desc, formatted]);
        }       
    }
}

type SimpleFormatter = (arg: any) => string;
type ComplexFormatter = (arg: any, obj: Map) => FormattedField[];

type EventFormat = {
    [key: string]: [string, SimpleFormatter?]|ComplexFormatter;
};

const receiverFormat: EventFormat = {
    vendor: ["Vendor"],
    hardware: ["Hardware"],
    firmware: ["Firmware"],
    supportedGNSS: ["Supported GNSS", formatGNSS],
}

function formatGNSS(gnssArray: string[]): string {
    if (!gnssArray || !Array.isArray(gnssArray)) {
        return "";
    }
    return gnssArray.join(", ");
}

const timeFormat: EventFormat = {
    utc: formatUTC,
    tai: ["TAI", formatTAI],
}

const phcFormat: EventFormat = {
    mode: ["State"],
    offset: ["Offset from GPS", formatNanoseconds],
    freq: ["Frequency offset", (arg: number) => `${arg.toFixed(2)} ppb`],
    stepCount: (count: number, obj: Map) => [
        ["Stepped", count + (obj.stepCountChanging ? "/" + (count + 1) : "") + " times"],
    ]
}

const surveyFormat: EventFormat = {
    accuracy: ["Accuracy", (arg: number) => `${arg.toFixed(4)} m`],
    x: ["ECEF X", formatECEF],
    y: ["ECEF Y", formatECEF],
    z: ["ECEF Z", formatECEF],
    latLon: formatLL,
    alt: ["Height", formatAlt],
    obsCount: ["Observations"],
    obsTime: ["Observation time"],
    valid: ["Valid", formatBoolean],
    inProgress: ["In Progress", formatBoolean],
}

const statusFormat: EventFormat = {
    fixLevel: ["Fix type"],
    solutionDim: ["Solution dimensionality"],
    corrections: ["Corrections", (arg: string[]) => arg.join(", ")],
    tdop: ["Time DOP", (arg: number) => arg.toFixed(2)],
    gdop: ["Geometric DOP", (arg: number) => arg.toFixed(2)],
    numSVUsed: ["Satellites used"],
    numSVTracked: ["Satellites tracked"],
    numSVInView: ["Satellites in view"],
    gnssUsed: ["Constellations used", (arg: string[]) => arg.join(", ")],
    bandsUsed: ["Bands used", (arg: string[]) => arg.join(", ")],
}

const positionFormat: EventFormat = {
    latLon: formatLL,
    heightMSL: ["Elevation", (arg: number) => `${arg.toFixed(2)} m`],
    height: ["Height", (arg: number) => `${arg.toFixed(2)} m`],
    posECEFX: ["ECEF X", formatECEF],
    posECEFY: ["ECEF Y", formatECEF],
    posECEFZ: ["ECEF Z", formatECEF],
}

const velocityFormat: EventFormat = {
    groundSpeed: ["Ground speed", (arg: number) => `${arg.toFixed(2)} m/s`],
    course: ["Course", (arg: number) => `${arg.toFixed(1)} deg`],
    velN: ["Vel north", (arg: number) => `${arg.toFixed(3)} m/s`],
    velE: ["Vel east", (arg: number) => `${arg.toFixed(3)} m/s`],
    velD: ["Vel down", (arg: number) => `${arg.toFixed(3)} m/s`],
}

const positionQualityFormat: EventFormat = {
    accHor: ["Horizontal accuracy", (arg: number) => `${arg.toFixed(3)} m`],
    accVert: ["Vertical accuracy", (arg: number) => `${arg.toFixed(3)} m`],
    accPos: ["3D accuracy", (arg: number) => `${arg.toFixed(3)} m`],
    hdop: ["Horizontal DOP", (arg: number) => arg.toFixed(2)],
    vdop: ["Vertical DOP", (arg: number) => arg.toFixed(2)],
    pdop: ["Position DOP", (arg: number) => arg.toFixed(2)],
    ndop: ["Northing DOP", (arg: number) => arg.toFixed(2)],
    edop: ["Easting DOP", (arg: number) => arg.toFixed(2)],
    diffAge: ["Differential age", (arg: number) => `${arg.toFixed(1)} s`],
    rtcmRefBaseID: ["RTCM base station"],
}

function formatECEF(arg: number): string {
    return arg.toFixed(4);
}

function formatLL(arg: [number, number], _obj: Map): FormattedField[] {
    return [
        ["Coordinates", <a href={mapsURL(arg)} target="_blank" className="underline hover:text-blue-500">{coordsToString(arg, 5)}</a>]
    ];
}

function mapsURL(arg: [number, number]): string {
    return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(coordsToString(arg,7))}`;
}

function coordsToString(arg: [number, number], nDigits: number): string {
    return `${arg[0].toFixed(nDigits)},${arg[1].toFixed(nDigits)}`;
}

function formatAlt(arg: number): string {
    return `${arg.toFixed(2)} m`;
}

function formatUTC(utc: string): FormattedField[] {
    const dt = formatUTCLocal(utc);
    if (dt == null) {
        return []
    }
    const {date, time} = dt;
    return [
        ["Local time", time],
        ["Local date", date],
        ["UTC", formatDateTime(utc)]
    ]; 
}

function formatBoolean(arg: boolean): string {
    return arg ? "Yes" : "No";
}

interface SkyViewCardProps {
    svs: SVInfo[];
}

const SkyViewCard: FunctionComponent<SkyViewCardProps> = ({ svs }) => {
    return (
        <div className="md:col-span-2 lg:col-span-2 md:row-span-2 lg:row-span-2">
        <CardElement>
        {SkyView(svs)}
        </CardElement>
        </div>
    );
};

interface SignalGraphCardProps {
    svs: SVInfo[];
}

const SignalGraphCard: FunctionComponent<SignalGraphCardProps> = ({ svs }) => {
    const [maxSatelliteCount, setMaxSatelliteCount] = useState(0);

    // Only ever increase the max count (high water mark approach)
    useEffect(() => {
        if (svs.length > maxSatelliteCount) {
            setMaxSatelliteCount(svs.length);
        }
    }, [svs, maxSatelliteCount]);

    // Apply row-span-2 when max satellite count exceeds threshold
    const isDoubleRow = maxSatelliteCount >= 15;
    const rowSpanClass = isDoubleRow ? "md:row-span-2 lg:row-span-2" : "";

    return (
        <div className={`${rowSpanClass} h-full`}>
            <CardElement title="Signal Levels">
                {SignalGraph(svs, maxSatelliteCount, isDoubleRow)}
            </CardElement>
        </div>
    );
};

// RTCMEvent is the subset of the corReport SSE payload that the RTCM
// card cares about. validateEvent has already enforced tag === "RTCM"
// and a non-empty msgID.
export type RTCMEvent = {
    source: 'pull' | 'receiver';
    msgID: string;
    used?: boolean;
};

// RTCMState is the per-msgID accumulator backing the RTCM card.
// totalCount[id] is the number of accepted events for that msgID in
// the current source session. unusedCount is null until at least one
// event in the session has carried a `used` field, after which it
// counts the `used === false` events; M = total - unused is the
// "used" count shown as M/N. Both reset when source flips.
export class RTCMState {
    source: 'pull' | 'receiver';
    totalCount: { [msgID: string]: number } = {};
    unusedCount: { [msgID: string]: number } | null = null;

    constructor(source: 'pull' | 'receiver') {
        this.source = source;
    }

    // update mutates this state to incorporate ev. A source flip
    // clears the counters before counting the new event.
    update(ev: RTCMEvent) {
        if (this.source !== ev.source) {
            this.source = ev.source;
            this.totalCount = {};
            this.unusedCount = null;
        }
        this.totalCount[ev.msgID] = (this.totalCount[ev.msgID] ?? 0) + 1;
        if (ev.used !== undefined) {
            if (this.unusedCount === null) this.unusedCount = {};
            if (ev.used === false) {
                this.unusedCount[ev.msgID] = (this.unusedCount[ev.msgID] ?? 0) + 1;
            }
        }
    }

    // clone returns a shallow copy of this state. Used by the React
    // boundary so setRTCM sees a fresh reference each tick.
    clone(): RTCMState {
        const c = new RTCMState(this.source);
        c.totalCount = { ...this.totalCount };
        c.unusedCount = this.unusedCount === null ? null : { ...this.unusedCount };
        return c;
    }

    title(): string {
        if (this.source !== 'receiver') return 'RTCM Messages Received';
        if (this.unusedCount === null) return 'RTCM Messages Used';
        return 'RTCM Messages Used/Received';
    }

    rowValue(id: string): string {
        const n = this.totalCount[id];
        if (this.unusedCount === null) return String(n);
        const unused = this.unusedCount[id] ?? 0;
        return `${n - unused}/${n}`;
    }
}

// sortRTCMMsgIDs sorts by [main, sub] pairs so '4072.0' precedes
// '4072.1' precedes '4072.10'.
export function sortRTCMMsgIDs(ids: string[]): string[] {
    return [...ids].sort((a, b) => {
        const [am, as] = parseMsgID(a);
        const [bm, bs] = parseMsgID(b);
        return am - bm || as - bs;
    });
}

function parseMsgID(id: string): [number, number] {
    const [main, sub = "0"] = id.split(".");
    return [Number(main), Number(sub)];
}

interface RTCMCardProps {
    state: RTCMState;
}

const RTCMCard: FunctionComponent<RTCMCardProps> = ({ state }) => {
    const ids = sortRTCMMsgIDs(Object.keys(state.totalCount));
    return (
        <CardElement title={state.title()}>
        {ids.map(id => (
            <FieldElement desc={id}>{state.rowValue(id)}</FieldElement>
        ))}
        </CardElement>
    );
};

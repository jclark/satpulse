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
const EVENT_TYPES = ["satellites", "time", "phc", "survey", "receiver", "posvel", "quality", "init"] as const;
type EventType = typeof EVENT_TYPES[number];

type Map = {[key: string]: any};

export const Dashboard: FunctionComponent = () => {
    const context = useContext(EventSourceContext) as EventSource;
    const [events, setEvents] = useState<Map>({});
    const [haveLookAngles, setHaveLookAngles] = useState(false);
    
    useEffect(() => {
        const handler = (type: string) => (e: MessageEvent<string>) => {
            const parsedEvents = parseSSEMessage(type, e.data);
            for (const [eventType, eventData] of parsedEvents) {
                const obj : Map|null = validateEvent(eventType, eventData);
                if (obj !== null) {
                    if (eventType === 'satellites' && obj.svs && obj.svs.length > 0) {
                        setHaveLookAngles(obj.svs.some((sv: any) => sv.lookAngles));
                    }
                    setEvents(prev => ({ ...prev, [eventType]: obj }));
                }
            }
        };
        for (const type of EVENT_TYPES) {
            context.addEventListener(type, handler(type));
        }
        return () => {
            for (const type of EVENT_TYPES) {
                context.removeEventListener(type, handler(type));
            }
        };
    }, []);
    
    const svs = events.satellites ? simplifySignals(events.satellites.svs) : [];
    
    return (
        <CardsElement>
        {events.satellites && haveLookAngles && <SkyViewCard svs={svs} />}
        {events.satellites && <SignalGraphCard svs={svs} />}
        {events.time && <PropertyCard title="Current GPS Time" data={events.time} format={timeFormat} />}
        {events.phc && <PropertyCard title="PTP Hardware Clock" data={events.phc} format={phcFormat} />}
        {events.receiver && <PropertyCard title="Receiver" data={events.receiver} format={receiverFormat} />}
        {events.quality && <PropertyCard title="Status" data={events.quality} format={statusFormat} />}
        {events.posvel && <PropertyCard title="Position" data={events.posvel} format={positionFormat} />}
        {events.posvel && showVelocity(events.posvel) && <PropertyCard title="Velocity" data={events.posvel} format={velocityFormat} />}
        {events.quality && <PropertyCard title="Position Quality" data={events.quality} format={positionQualityFormat} />}
        {events.survey && <PropertyCard title="Survey-in Status" data={events.survey} format={surveyFormat} />}
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
function validateEvent(type: string, data: JSONValue): JSONObject | null {
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
    syncState: ["State"],
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
    fixDim: ["Fix dimensionality"],
    corrections: ["Corrections", (arg: string[]) => arg.join(", ")],
    tdop: ["Time DOP", (arg: number) => arg.toFixed(2)],
    gdop: ["Geometric DOP", (arg: number) => arg.toFixed(2)],
    numSVUsed: ["Satellites used"],
    numSVTracked: ["Satellites tracked"],
    signalsUsed: formatSignalsUsed,
}

function formatSignalsUsed(signals: {[gnss: string]: string[]}, _obj: Map): FormattedField[] {
    return Object.entries(signals).map(([gnss, sigs]) =>
        [`${gnss} signals used`, sigs.join(", ")]
    );
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

function showVelocity(posvel: Map): boolean {
    return typeof posvel.groundSpeed === 'number' && posvel.groundSpeed >= 0.1;
}

const positionQualityFormat: EventFormat = {
    accHor: ["Horizontal accuracy", (arg: number) => `${arg.toFixed(3)} m`],
    accVert: ["Vertical accuracy", (arg: number) => `${arg.toFixed(3)} m`],
    accPos: ["3D accuracy", (arg: number) => `${arg.toFixed(3)} m`],
    hdop: ["Horizontal DOP", (arg: number) => arg.toFixed(2)],
    vdop: ["Vertical DOP", (arg: number) => arg.toFixed(2)],
    pdop: ["Position DOP", (arg: number) => arg.toFixed(2)],
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

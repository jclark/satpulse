import { createContext, FunctionComponent } from 'preact';
import { useContext, useEffect, useState } from 'preact/hooks';
import { formatUTCLocal, formatNanoseconds, formatTAI, formatDateTime } from './timefmt';

export const EventSourceContext = createContext<EventSource | null>(null);

export const Dashboard: FunctionComponent = () => {
    const context = useContext(EventSourceContext) as EventSource;
    const [events, setEvents] = useState<Map>({});
    
    useEffect(() => {
        const types = ["time", "phc", "survey", "version", "init"];
        const handler = (type: string) => (e: MessageEvent<string>) => {
            try {
                const data = JSON.parse(e.data);
                
                // Basic validation that data is an object
                if (!data || typeof data !== 'object') {
                    console.warn(`Invalid ${type} event data:`, data);
                    return;
                }
                
                if (type === "init") {
                    for (const key of ["version", "time", "phc", "survey"]) {
                        // Validate each property is a proper object before adding to state
                        if (data[key] && typeof data[key] === 'object') {
                            setEvents(prev => ({ ...prev, [key]: data[key] }));
                        }
                    }
                } else {
                    // Only update state if we have a valid object
                    setEvents(prev => ({ ...prev, [type]: data }));
                }
            } catch (err) {
                console.warn(`Error parsing ${type} event:`, err);
            }
        };
        
        for (const type of types) {
            context.addEventListener(type, handler(type));
        }
        
        return () => {
            for (const type of types) {
                context.removeEventListener(type, handler(type));
            }
        };
    }, []);
    
    return (
        <CardsElement>
        {events.time && <Card title="Current GPS Time" data={events.time} format={timeFormat} />}
        {events.phc && <Card title="PTP Hardware Clock" data={events.phc} format={phcFormat} />}    
        {events.version && <Card title="GPS Receiver Version" data={events.version} format={versionFormat} />}
        {events.survey && <Card title="Survey-in Status" data={events.survey} format={surveyFormat} />}
        </CardsElement>
    );
};

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
    title: string;
}

const CardElement: FunctionComponent<CardElementProps> = ({ children, title }) => {
    return (
        <div className="p-4 rounded-lg mb-4 shadow-md break-inside-avoid bg-white dark:bg-gray-800 border-l-4 border-orange-500">
        <h3 className="mt-0 mb-4 text-xl cursor-pointer text-blue-600 dark:text-blue-400">{title}</h3>
        <div className="transition-all duration-300 max-h-[1000px] overflow-hidden">
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

/*
interface InitVersion {
hw?: string;
sw?: string;
extensions?: string[];
fw?: FWVer 
prot?: ProtVer 
mod?: string            
flash?: boolean              
gnss?: string[] 
}*/

interface ProtVer {
    major: number;
    minor: number;
}

interface FWVer {
    productCategory: string;
    major: number;
    minor: number;
}

type CardProps = {
    title: string;
    data: Map;
    format: EventFormat;
};

type Map = {[key: string]: any};

const Card: FunctionComponent<CardProps> = ({ title, data, format }) => {
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

const versionFormat: EventFormat = {
    hw: ["Hardware"],
    mod: ["Module"],
    prot: ["UBX Protocol", (arg: ProtVer) => `${arg.major}.${arg.minor}`],
    fw: ["Firmware", (arg: FWVer) => `${arg.productCategory} ${arg.major}.${arg.minor}`],
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
    alt: ["Altitude", formatAlt],
    obsCount: ["Observations"],
    obsTime: ["Observation time"],
    valid: ["Valid", formatBoolean],
    inProgress: ["In Progress", formatBoolean],
}

function formatECEF(arg: number): string {
    return arg.toFixed(4);
}

function formatLL(arg: [number, number], _obj: Map): FormattedField[] {
    if (isNaN(arg[0]) || isNaN(arg[1])) {
        return [];
    }
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
    if (isNaN(arg)) {
        return "";
    }
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

import { render, createContext, FunctionComponent } from 'preact';
import { useContext, useEffect, useState } from 'preact/hooks';
import { formatUTCLocal, formatNanoseconds, formatTAI, formatDateTime } from './timefmt';

export const EventSourceContext = createContext<EventSource | null>(null);

export const Dashboard: FunctionComponent = () => {
  return (
    <CardsElement>
      <Card title="Current GPS Time" event={["time", timeFormat]} />
      <Card title="PTP Hardware Clock" event={["phc", phcFormat]} />
      <Card title="Survey-in Status" event={["survey", surveyFormat]} />
      <Card title="GPS Receiver Version" init={["version", versionFormat]} />
    </CardsElement>
  );
};

interface CardsElementProps {
    children: preact.ComponentChildren;
}

const CardsElement: FunctionComponent<CardsElementProps> = ({ children }) => {
    return (
        <div className="columns-[18rem] gap-4 p-4">
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
    event?: [string, EventFormat];
    init?: [string, EventFormat];

}

type Map = {[key: string]: any};

function useEvent(name: string, key?: string): Object {
    const context = useContext(EventSourceContext) as EventSource;
    const [state, setState] = useState<Map>({});

    const handleEvent = (event: MessageEvent<string>) => {
        try {
            const data = JSON.parse(event.data);
            if (data != null && typeof data === 'object') {
                if (key) {
                    const map = data as Map;
                    if (key in map) {
                        const newState = map[key];
                        if (newState !== null && typeof newState === 'object') {
                            setState(newState);
                        }
                    }
                } else {
                    setState(data);
                }
            }
        }
        catch (e) {
        }
    };
    useEffect(() => {
        context.addEventListener(name, handleEvent);

        return () => {
            context.removeEventListener(name, handleEvent);
        };
    }, []);
    return state;
}

type FormattedField = [string, any];

const Card: FunctionComponent<CardProps> = ({title, event, init}) => {
    const fields: FormattedField[] = [];
    if (init) {
        const [key, format] = init;
        const state = useEvent("init", key)
        addFields(fields, state, format)
    }
    if (event) {
        const [key, format] = event;
        const state = useEvent(key)
        addFields(fields, state, format)
    }
    return (
        <CardElement title={title}>
            {fields.map(([desc, value]) => <FieldElement desc={desc}>{value}</FieldElement>)}
        </CardElement>
    );
};

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

type CardFormat = {
    [key: string]: EventFormat;
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

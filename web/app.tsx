
import { render, createContext, FunctionComponent } from 'preact';
import { useContext, useEffect, useState } from 'preact/hooks';
import { formatUTCLocal, formatNanoseconds, formatTAI, formatDateTime } from './timefmt';

const EventSourceContext = createContext<EventSource | null>(null);

interface CardsElementProps {
    children: preact.ComponentChildren;
}

const CardsElement: FunctionComponent<CardsElementProps> = ({ children }) => {
    return (
        <div class="cards">
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
        <div class="card">
            <h3 class="card-title">{title}</h3>
            <div class="fields">
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
        <div class="field"><span class="field-name">{desc}:</span> <span class="field-value">{children}</span></div>
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
    offset: ["Offset from GPS", formatNanoseconds],
    freq: ["Frequency offset", (arg: number) => `${arg.toFixed(2)} ppb`],
    stepCount: (count: number, obj: Map) => [
        ["Stepped", count + (obj.stepCountChanging ? "/" + (count + 1) : "") + " times"],
    ]
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

function createEventSource(): EventSource {
    const docURL = new URL(window.location.href);
    const sseURL = docURL.origin + docURL.pathname + "/sse";
    return new EventSource(sseURL);
}

const rootElement = document.getElementById('root') as HTMLElement;
render(
    <EventSourceContext.Provider value={createEventSource()}>
        <CardsElement>
            <Card title="Current GPS Time" event={["time", timeFormat]}/>
            <Card title="PTP Hardware Clock" event={["phc", phcFormat]}/>
            <Card title="GPS Receiver Version" init={["version", versionFormat]}/>
        </CardsElement>
    </EventSourceContext.Provider>,
    rootElement
);




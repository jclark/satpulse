import { render, createContext } from 'preact';
import { useContext, useEffect, useState } from 'preact/hooks';
import { DateTimeParts, formatTimestamp, formatDate } from './timefmt';

const EventSourceContext = createContext<EventSource | null>(null);

const INITIAL_YEAR = 2000;

const DateTimeComponent = () => {
    const context = useContext(EventSourceContext) as EventSource;
    const [state, setState] = useState<DateTimeParts>(formatDate(new Date(INITIAL_YEAR, 0), "00"));

    const handleTime = (event: MessageEvent<string>) => {
        const newState = formatTimestamp(event.data);
        if (newState != null) {
            setState(newState);
        }
    };

    useEffect(() => {
        context.addEventListener('time', handleTime);

        return () => {
            context.removeEventListener('time', handleTime);
        };
    }, []);

    return (
        <div class="dateTime">
            <div class="timeZoneName">{state.timeZoneName}</div>
            <div class="time">{state.time}</div>
            <div class="date">{state.date}</div>
        </div>
    );
};

function createEventSource(): EventSource {
    const docURL = new URL(window.location.href);
    const sseURL = docURL.origin + docURL.pathname + "/sse";
    return new EventSource(sseURL);
}

const rootElement = document.getElementById('root') as HTMLElement;
render(
    <EventSourceContext.Provider value={createEventSource()}>
        <DateTimeComponent />
    </EventSourceContext.Provider>,
    rootElement
);

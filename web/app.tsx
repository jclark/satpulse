import { Component, render, createContext } from 'preact';
import { DateTimeParts, formatTimestamp, formatDate } from './timefmt';

const EventSourceContext = createContext<EventSource | null>(null);

const INITIAL_YEAR = 2000;

class DateTimeComponent extends Component<{}, DateTimeParts> {
    static contextType = EventSourceContext;
    context!: EventSource;

    constructor(props: {}) {
        super(props);
        this.handleTime = this.handleTime.bind(this);
        this.state = formatDate(new Date(INITIAL_YEAR, 0), "00")
    }

    componentDidMount() {
        this.context.addEventListener('time', this.handleTime);
    }

    componentWillUnmount() {
        this.context.removeEventListener('time', this.handleTime);
    }

    handleTime(event: MessageEvent) {
        this.setTimestamp(event.data)
    }

    setTimestamp(timestamp: string) {
        const newState = formatTimestamp(timestamp)
        if (newState != null) {
            this.setState(newState);
        }
    }

    render() {
        return <div class="dateTime">
                 <div class="timeZoneName">{this.state.timeZoneName}</div>
                 <div class="time">{this.state.time}</div>
                 <div class="date">{this.state.date}</div>
               </div>;
    }
}

function createEventSource() : EventSource {
    const docURL = new URL(window.location.href);
    const sseURL = docURL.origin + docURL.pathname + "/sse";
    return new EventSource(sseURL);
}

const rootElement = document.getElementById('root') as HTMLElement;
render(
    <EventSourceContext.Provider value={createEventSource()}>
        <DateTimeComponent/>
    </EventSourceContext.Provider>,
    rootElement);

import { render } from 'preact';
import { Dashboard, EventSourceContext } from './dashboard';

function createEventSource(): EventSource {
  const base = new URL(window.location.href);
  const sseURL = base.origin + base.pathname + '/sse';
  return new EventSource(sseURL);
}

const root = document.getElementById('root')!;
render(
  <EventSourceContext.Provider value={createEventSource()}>
    <Dashboard />
  </EventSourceContext.Provider>,
  root
);
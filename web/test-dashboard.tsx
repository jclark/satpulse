import { render } from 'preact';
import { Dashboard, EventSourceContext } from './dashboard';
import { SVInfo } from './svg';

// Local minimal EventSource interface just for test
interface MinimalEventSource {
  addEventListener(type: string, listener: EventListener): void;
  removeEventListener(type: string, listener: EventListener): void;
  close(): void;
}

interface ReceiverInfo {
  vendor: string;
  firmware: string;
  hardware: string;
  supportedGNSS: string[];
}

const surveyEvent = {
  x: 123.45,
  y: 456.78,
  z: 789.01,
  accuracy: 0.02,
  alt: 12.5,
  latLon: [13.7563, 100.5018],
  obsTime: 3600,
  obsCount: 1234,
  inProgress: false,
  valid: true,
};

const timeEvent = {
  utc: '2025-04-09T12:34:56Z',
  tai: 1744192496,
};

// Add phcEvent to match SampleEvent from monitor.go
const phcEvent = {
  offset: 15,           // in nanoseconds
  freq: 6537,             // in parts per billion
  stepCount: 5,
  stepCountChanging: false,
  outlier: false,
  syncState: "in sync"
};

// Add initEvent with receiver data matching ReceiverInfo struct
const initEvent: { receiver: ReceiverInfo } = {
  receiver: {
    vendor: "u-blox",
    hardware: "ZED-F9T",
    firmware: "TIM 1.02 PROTVER 18.00",
    supportedGNSS: ["GPS", "GLO", "GAL", "BDS"]
  }
};

const svs: SVInfo[] = [
  { id: 'G15', azimuth: 352, elevation: 69, signals: [ { cn0: 45, id: "L1 C/A" }, { cn0: 50, id: "L5" } ], used: true },
  { id: 'S143', azimuth: 333, elevation: 26, signals: [ { cn0: 23 } ] },
  { id: 'G23', azimuth: 297, elevation: 23, signals: [ { cn0: 36 } ] },
  { id: 'G24', azimuth: 154, elevation: 38, signals: [ { cn0: 39 } ], used: true },
  { id: 'G25', azimuth: 212, elevation: 10, signals: [ { cn0: 32 } ] },
  { id: 'G29', azimuth: 262, elevation: 64, signals: [ { cn0: 46 } ], used: true },
  { id: 'E02', azimuth: 206, elevation: 15, signals: [ { cn0: 33 } ] },
  { id: 'E03', azimuth: 220, elevation: 62, signals: [ { cn0: 44 } ], used: true },
  { id: 'E05', azimuth: 33, elevation: 62, signals: [ { cn0: 29 } ], used: true },
  { id: 'E08', azimuth: 217, elevation: 11, signals: [ { cn0: 35 } ], used: true },
  { id: 'E09', azimuth: 35, elevation: 10, signals: [ { cn0: 12 } ] },
  { id: 'E15', azimuth: 321, elevation: 8, signals: [ { cn0: 17 } ] },
  { id: 'E25', azimuth: 158, elevation: -3, signals: [ { cn0: 8 } ] },
  { id: 'E30', azimuth: 256, elevation: 9, signals: [ { cn0: 21 } ] },
  { id: 'E34', azimuth: 359, elevation: 45, signals: [ { cn0: 27 } ] },
  { id: 'E36', azimuth: 79, elevation: 45, signals: [ { cn0: 24 } ] },
  { id: 'C06', azimuth: 159, elevation: 36, signals: [ { cn0: 36 } ] },
  { id: 'C07', azimuth: 185, elevation: 46, signals: [ { cn0: 42 } ], used: true },
  { id: 'C09', azimuth: 172, elevation: 28, signals: [ { cn0: 38 } ], used: true },
  { id: 'C10', azimuth: 202, elevation: 52, signals: [ { cn0: 44 } ], used: true },
  { id: 'C11', azimuth: 129, elevation: 69, signals: [ { cn0: 46 } ], used: true },
  { id: 'C12', azimuth: 142, elevation: 15, signals: [ { cn0: 27 } ] },
  { id: 'C13', azimuth: 345, elevation: 38, signals: [ { cn0: 22 } ] },
  { id: 'C14', azimuth: 351, elevation: 51, signals: [ { cn0: 28 } ] },
  { id: 'J02', azimuth: 147, elevation: 15, signals: [ { cn0: 33 } ], used: true },
  { id: 'J03', azimuth: 95, elevation: 51, signals: [ { cn0: 29 } ] },
  { id: 'J04', azimuth: 49, elevation: 46, signals: [ { cn0: 26 } ] },
  { id: 'J07', azimuth: 116, elevation: 56, signals: [ { cn0: 36 } ], used: true },
  { id: 'R03', azimuth: 2, elevation: 39, signals: [ { cn0: 12 } ] },
  { id: 'R04', azimuth: 283, elevation: 55, signals: [ { cn0: 50 } ], used: true },
  { id: 'R05', azimuth: 212, elevation: -22, signals: [ { cn0: 46 } ], used: true },
  { id: 'R17', azimuth: 123, elevation: 52, signals: [ { cn0: 48 } ], used: true },
  { id: 'R18', azimuth: 14, elevation: 62, signals: [ { cn0: 35 } ], used: true }
];

class MockEventSource implements MinimalEventSource {
  private svidPrefixes: string;

  constructor(svidPrefixes: string) {
    this.svidPrefixes = svidPrefixes;
  }

  addEventListener(type: string, listener: EventListener): void {
    if (type === 'survey') {
      setTimeout(() => {
        listener(new MessageEvent('survey', { data: JSON.stringify(surveyEvent) }));
      }, 500);
    } else if (type === 'time') {
      setTimeout(() => {
        listener(new MessageEvent('time', { data: JSON.stringify(timeEvent) }));
      }, 300);
    } else if (type === 'phc') {
      setTimeout(() => {
        listener(new MessageEvent('phc', { data: JSON.stringify(phcEvent) }));
      }, 400);
    } else if (type === 'init') {
      setTimeout(() => {
        listener(new MessageEvent('init', { data: JSON.stringify(initEvent) }));
      }, 200);
    } else if (type === 'satellites') {
      setTimeout(() => {
        const filteredSvs = this.svidPrefixes 
          ? svs.filter(sv => this.svidPrefixes.includes(sv.id[0]))
          : svs;
        listener(new MessageEvent('satellites', { data: JSON.stringify({ svs: filteredSvs }) }));
      }, 350);
    } else if (type === 'receiver') {
      setTimeout(() => {
        listener(new MessageEvent('receiver', { data: JSON.stringify(initEvent.receiver) }));
      }, 250);
    }
  }

  removeEventListener(): void {}
  close(): void {}
}

// Export a rendering function that will be called by the HTML wrapper
export function renderDashboard(searchString: string) {
  const params = new URLSearchParams(searchString);
  const root = document.getElementById('root')!;
  render(
    <EventSourceContext.Provider value={new MockEventSource(params.get('g') || '') as any}>
      <Dashboard />
    </EventSourceContext.Provider>,
    root
  );
}
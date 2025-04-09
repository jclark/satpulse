import { render } from 'preact';
import { Dashboard, EventSourceContext } from './dashboard';

// Local minimal EventSource interface just for test
interface MinimalEventSource {
  addEventListener(type: string, listener: EventListener): void;
  removeEventListener(type: string, listener: EventListener): void;
  close(): void;
}

// Define interfaces based on dashboard.tsx
interface ProtVer {
  major: number;
  minor: number;
}

interface FWVer {
  productCategory: string;
  major: number;
  minor: number;
}

interface Version {
  hw: string;
  sw: string;
  extensions?: string[];
  fw?: FWVer;
  prot?: ProtVer;
  mod: string;
  flash: boolean;
  gnss: string;
}

// Simulated test data
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

// Add initEvent with version data matching Version struct from ubxver.go
const initEvent: { version: Version } = {
  version: {
    hw: "00190000",
    sw: "EXT CORE 4.04 (7eb82)",
    extensions: [
      "FWVER=TIM 1.02",
      "PROTVER=18.00",
      "MOD=ZED-F9T",
      "GPS;QZSS",
      "GLO",
      "GAL",
      "BDS",
      "FIS=0xEF4015 (100111)"
    ],
    fw: {
      productCategory: "TIM",
      major: 1,
      minor: 2
    },
    prot: {
      major: 18,
      minor: 0
    },
    mod: "ZED-F9T",
    flash: true,
    gnss: "GPS,GAL,GLO,BDS"
  }
};

// A lightweight mock EventSource
class MockEventSource implements MinimalEventSource {
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
    }
  }

  removeEventListener(): void {}
  close(): void {}
}

// Render test view
const root = document.getElementById('root')!;
render(
  <EventSourceContext.Provider value={new MockEventSource() as any}>
    <Dashboard />
  </EventSourceContext.Provider>,
  root
);
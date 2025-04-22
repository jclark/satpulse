import { render } from 'preact';
import { Dashboard, EventSourceContext } from './dashboard';
import { SVInfo } from './svg';

// Local minimal EventSource interface just for test
interface MinimalEventSource {
  addEventListener(type: string, listener: EventListener): void;
  removeEventListener(type: string, listener: EventListener): void;
  close(): void;
}

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

const svs: SVInfo[] = [
  { svid: 'G15', azimuth: 352, elevation: 69, cno: 45 },
  { svid: 'G18', azimuth: 333, elevation: 26, cno: 23 },
  { svid: 'G23', azimuth: 297, elevation: 23, cno: 36 },
  { svid: 'G24', azimuth: 154, elevation: 38, cno: 39 },
  { svid: 'G25', azimuth: 212, elevation: 10, cno: 32 },
  { svid: 'G29', azimuth: 262, elevation: 64, cno: 46 },
  { svid: 'E02', azimuth: 206, elevation: 15, cno: 33 },
  { svid: 'E03', azimuth: 220, elevation: 62, cno: 44 },
  { svid: 'E05', azimuth: 33, elevation: 62, cno: 29 },
  { svid: 'E08', azimuth: 217, elevation: 11, cno: 35 },
  { svid: 'E09', azimuth: 35, elevation: 10, cno: 12 },
  { svid: 'E15', azimuth: 321, elevation: 8, cno: 17 },
  { svid: 'E25', azimuth: 158, elevation: 3, cno: 8 },
  { svid: 'E30', azimuth: 256, elevation: 9, cno: 21 },
  { svid: 'E34', azimuth: 359, elevation: 45, cno: 27 },
  { svid: 'E36', azimuth: 79, elevation: 45, cno: 24 },
  { svid: 'C06', azimuth: 159, elevation: 36, cno: 36 },
  { svid: 'C07', azimuth: 185, elevation: 46, cno: 42 },
  { svid: 'C09', azimuth: 172, elevation: 28, cno: 38 },
  { svid: 'C10', azimuth: 202, elevation: 52, cno: 44 },
  { svid: 'C11', azimuth: 129, elevation: 69, cno: 46 },
  { svid: 'C12', azimuth: 142, elevation: 15, cno: 27 },
  { svid: 'C13', azimuth: 345, elevation: 38, cno: 22 },
  { svid: 'C14', azimuth: 351, elevation: 51, cno: 28 },
  { svid: 'J02', azimuth: 147, elevation: 15, cno: 33 },
  { svid: 'J03', azimuth: 95, elevation: 51, cno: 29 },
  { svid: 'J04', azimuth: 49, elevation: 46, cno: 26 },
  { svid: 'J07', azimuth: 116, elevation: 56, cno: 36 },
  { svid: 'R03', azimuth: 2, elevation: 39, cno: 12 },
  { svid: 'R04', azimuth: 283, elevation: 55, cno: 50 },
  { svid: 'R05', azimuth: 212, elevation: -22, cno: 46 },
  { svid: 'R17', azimuth: 123, elevation: 52, cno: 48 },
  { svid: 'R18', azimuth: 14, elevation: 62, cno: 35 }
];

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
    } else if (type === 'satellites') {
      setTimeout(() => {
        listener(new MessageEvent('satellites', { data: JSON.stringify({ svs }) }));
      }, 350);
    } else if (type === 'version') {
      setTimeout(() => {
        listener(new MessageEvent('version', { data: JSON.stringify(initEvent.version) }));
      }, 250);
    }
  }

  removeEventListener(): void {}
  close(): void {}
}

const root = document.getElementById('root')!;
render(
  <EventSourceContext.Provider value={new MockEventSource() as any}>
    <Dashboard />
  </EventSourceContext.Provider>,
  root
);
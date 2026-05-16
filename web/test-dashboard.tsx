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
  mode: "tracking"
};

const modeEvent = {
  mode: "tracking",
};

const posvelEvent = {
  latLon: [13.7563, 100.5018],
  height: 15.432,
  heightMSL: -10.821,
  posECEFX: -1139434.7219,
  posECEFY: 6093677.0461,
  posECEFZ: 1504580.2941,
  groundSpeed: 0.03,
  course: 247.3,
  velN: -0.012,
  velE: -0.028,
  velD: 0.005,
};

const corReportEvents = [
  { tag: "RTCM", source: "pull", msgID: "1077", checksumOK: true },
  { tag: "RTCM", source: "pull", msgID: "1087", checksumOK: true },
  { tag: "RTCM", source: "pull", msgID: "1077", checksumOK: true },
  { tag: "RTCM", source: "pull", msgID: "4072.0", checksumOK: true },
  { tag: "RTCM", source: "pull", msgID: "4072.1", checksumOK: true },
  { tag: "RTCM", source: "pull", msgID: "1230", checksumOK: true },
  { tag: "RTCM", source: "receiver", msgID: "1077", checksumOK: true, used: true },
  { tag: "RTCM", source: "receiver", msgID: "1087", checksumOK: true, used: true },
  { tag: "RTCM", source: "receiver", msgID: "1077", checksumOK: true, used: true },
  { tag: "RTCM", source: "receiver", msgID: "1230", checksumOK: true, used: false },
  { tag: "RTCM", source: "receiver", msgID: "4072.1", checksumOK: true, used: true },
];

const qualityEvent = {
  fixLevel: "carrierFixed",
  solutionDim: "3D",
  corrections: ["OSR"],
  accHor: 0.014,
  accVert: 0.021,
  accPos: 0.025,
  gdop: 1.83,
  pdop: 1.52,
  hdop: 0.78,
  vdop: 1.30,
  tdop: 0.92,
  numSVUsed: 24,
  numSVTracked: 32,
  numSVInView: 38,
  gnssUsed: ["GPS", "GAL", "BDS"],
  bandsUsed: ["L1", "L5"],
  diffAge: 1.2,
  rtcmRefBaseID: 4072,
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
  { id: 'G15', lookAngles: { azimuth: 352, elevation: 69 }, signals: [ { cn0: 45, id: "L1 C/A", used: true }, { cn0: 50, id: "L5", used: true } ], used: true },
  { id: 'S143', lookAngles: { azimuth: 333, elevation: 26 }, signals: [ { cn0: 23 } ] },
  { id: 'G23', lookAngles: { azimuth: 297, elevation: 23 }, signals: [ { cn0: 36 } ] },
  { id: 'G24', lookAngles: { azimuth: 154, elevation: 38 }, signals: [ { cn0: 39, used: true } ], used: true },
  { id: 'G25', lookAngles: { azimuth: 212, elevation: 10 }, signals: [ { cn0: 32 } ] },
  { id: 'G29', lookAngles: { azimuth: 262, elevation: 64 }, signals: [ { cn0: 46, used: true } ], used: true },
  { id: 'E02', lookAngles: { azimuth: 206, elevation: 15 }, signals: [ { cn0: 33 } ] },
  { id: 'E03', lookAngles: { azimuth: 220, elevation: 62 }, signals: [ { cn0: 44, used: true } ], used: true },
  { id: 'E05', lookAngles: { azimuth: 33, elevation: 62 }, signals: [ { cn0: 29, used: true } ], used: true },
  { id: 'E08', lookAngles: { azimuth: 217, elevation: 11 }, signals: [ { cn0: 35, used: true } ], used: true },
  { id: 'E09', lookAngles: { azimuth: 35, elevation: 10 }, signals: [ { cn0: 12 } ] },
  { id: 'E15', lookAngles: { azimuth: 321, elevation: 8 }, signals: [ { cn0: 17 } ] },
  { id: 'E25', lookAngles: { azimuth: 158, elevation: -3 }, signals: [ { cn0: 8 } ] },
  { id: 'E30', lookAngles: { azimuth: 256, elevation: 9 }, signals: [ { cn0: 21 } ] },
  { id: 'E34', lookAngles: { azimuth: 359, elevation: 45 }, signals: [ { cn0: 27 } ] },
  { id: 'E36', lookAngles: { azimuth: 79, elevation: 45 }, signals: [ { cn0: 24 } ] },
  { id: 'C06', lookAngles: { azimuth: 159, elevation: 36 }, signals: [ { cn0: 36 } ] },
  { id: 'C07', lookAngles: { azimuth: 185, elevation: 46 }, signals: [ { cn0: 42, used: true } ], used: true },
  { id: 'C09', lookAngles: { azimuth: 172, elevation: 28 }, signals: [ { cn0: 38, used: true } ], used: true },
  { id: 'C10', lookAngles: { azimuth: 202, elevation: 52 }, signals: [ { cn0: 44, used: true } ], used: true },
  { id: 'C11', lookAngles: { azimuth: 129, elevation: 69 }, signals: [ { cn0: 46, used: true } ], used: true },
  { id: 'C12', lookAngles: { azimuth: 142, elevation: 15 }, signals: [ { cn0: 27 } ] },
  { id: 'C13', lookAngles: { azimuth: 345, elevation: 38 }, signals: [ { cn0: 22 } ] },
  { id: 'C14', lookAngles: { azimuth: 351, elevation: 51 }, signals: [ { cn0: 28 } ] },
  { id: 'J02', lookAngles: { azimuth: 147, elevation: 15 }, signals: [ { cn0: 33, used: true } ], used: true },
  { id: 'J03', lookAngles: { azimuth: 95, elevation: 51 }, signals: [ { cn0: 29 } ] },
  { id: 'J04', lookAngles: { azimuth: 49, elevation: 46 }, signals: [ { cn0: 26 } ] },
  { id: 'J07', lookAngles: { azimuth: 116, elevation: 56 }, signals: [ { cn0: 36, used: true } ], used: true },
  { id: 'R03', lookAngles: { azimuth: 2, elevation: 39 }, signals: [ { cn0: 12 } ] },
  { id: 'R04', lookAngles: { azimuth: 283, elevation: 55 }, signals: [ { cn0: 50, used: true } ], used: true },
  { id: 'R05', lookAngles: { azimuth: 212, elevation: -22 }, signals: [ { cn0: 46, used: true } ], used: true },
  { id: 'R17', lookAngles: { azimuth: 123, elevation: 52 }, signals: [ { cn0: 48, used: true } ], used: true },
  { id: 'R18', lookAngles: { azimuth: 14, elevation: 62 }, signals: [ { cn0: 35, used: true } ], used: true }
];

class MockEventSource implements MinimalEventSource {
  private svidPrefixes: string;
  private disableLookAngles: boolean;

  constructor(svidPrefixes: string, disableLookAngles: boolean = false) {
    this.svidPrefixes = svidPrefixes;
    this.disableLookAngles = disableLookAngles;
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
    } else if (type === 'mode') {
      setTimeout(() => {
        listener(new MessageEvent('mode', { data: JSON.stringify(modeEvent) }));
      }, 380);
    } else if (type === 'init') {
      setTimeout(() => {
        listener(new MessageEvent('init', { data: JSON.stringify(initEvent) }));
      }, 200);
    } else if (type === 'satellites') {
      setTimeout(() => {
        let filteredSvs = this.svidPrefixes 
          ? svs.filter(sv => this.svidPrefixes.includes(sv.id[0]))
          : svs;
        
        if (this.disableLookAngles) {
          filteredSvs = filteredSvs.map(sv => ({ ...sv, lookAngles: undefined }));
        }
        
        listener(new MessageEvent('satellites', { data: JSON.stringify({ svs: filteredSvs }) }));
      }, 350);
    } else if (type === 'receiver') {
      setTimeout(() => {
        listener(new MessageEvent('receiver', { data: JSON.stringify(initEvent.receiver) }));
      }, 250);
    } else if (type === 'posvel') {
      setTimeout(() => {
        listener(new MessageEvent('posvel', { data: JSON.stringify(posvelEvent) }));
      }, 320);
    } else if (type === 'quality') {
      setTimeout(() => {
        listener(new MessageEvent('quality', { data: JSON.stringify(qualityEvent) }));
      }, 330);
    } else if (type === 'corReport') {
      corReportEvents.forEach((ev, i) => {
        setTimeout(() => {
          listener(new MessageEvent('corReport', { data: JSON.stringify(ev) }));
        }, 600 + i * 200);
      });
    }
  }

  removeEventListener(): void {}
  close(): void {}
}

// Export a rendering function that will be called by the HTML wrapper
export function renderDashboard(searchString: string) {
  const params = new URLSearchParams(searchString);
  const disableLookAngles = params.get('angles') === '0';
  const root = document.getElementById('root')!;
  render(
    <EventSourceContext.Provider value={new MockEventSource(params.get('g') || '', disableLookAngles) as any}>
      <Dashboard />
    </EventSourceContext.Provider>,
    root
  );
}
import {describe, expect, it} from 'vitest';
import {NMEAMsgGGA, PVTMsgOff, PVTMsgPos, RawMsgNavData, SatsMsgSat, toggleMsgFlag} from './msg-flags';
import {nmeaWireValue} from './nmea-group';
import {pvtWireValue} from './pvt-group';
import {rawWireValue} from './raw-group';
import {rtcmWireValue} from './rtcm-group';
import {satsWireValue} from './sats-group';

describe('message flag sets', () => {
    it('toggles flags immutably', () => {
        const flags = new Set([PVTMsgPos]);
        const added = toggleMsgFlag(flags, PVTMsgOff, true);
        const removed = toggleMsgFlag(added, PVTMsgPos, false);
        expect(flags).toEqual(new Set([PVTMsgPos]));
        expect(added).toEqual(new Set([PVTMsgPos, PVTMsgOff]));
        expect(removed).toEqual(new Set([PVTMsgOff]));
        expect(added).not.toBe(flags);
        expect(removed).not.toBe(added);
    });
});

describe('message wire values', () => {
    it('builds NMEA arrays including other', () => {
        expect(nmeaWireValue(false, false, new Set())).toBeUndefined();
        expect(nmeaWireValue(true, true, new Set())).toEqual([]);
        expect(nmeaWireValue(true, false, new Set([NMEAMsgGGA]))).toEqual(['GGA', 'other']);
    });

    it('builds PVT arrays', () => {
        expect(pvtWireValue(false, new Set())).toBeUndefined();
        expect(pvtWireValue(true, new Set())).toEqual([]);
        expect(pvtWireValue(true, new Set([PVTMsgOff, PVTMsgPos]))).toEqual(['pos', 'off']);
    });

    it('builds satellite arrays', () => {
        expect(satsWireValue(false, new Set())).toBeUndefined();
        expect(satsWireValue(true, new Set())).toEqual([]);
        expect(satsWireValue(true, new Set([SatsMsgSat]))).toEqual(['sat']);
    });

    it('builds raw message arrays', () => {
        expect(rawWireValue(false, new Set())).toBeUndefined();
        expect(rawWireValue(true, new Set())).toEqual([]);
        expect(rawWireValue(true, new Set([RawMsgNavData]))).toEqual(['navData']);
    });

    it('builds RTCM arrays', () => {
        expect(rtcmWireValue(true, false, 'msm7', true, true)).toEqual(['other', 'MSM7', 'lax', 'ARP']);
    });
});

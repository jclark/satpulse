import {describe, expect, it} from 'vitest';
import {NMEAMsgGGA, NMEAMsgOther, NMEASelectableMsgNames, PVTMsgOff, PVTMsgPos, PVTMsgNames, msgFlag, msgFlagNames} from './msg-flags';
import {nmeaFlag, nmeaWireValue} from './nmea-group';
import {pvtFlag, pvtWireValue} from './pvt-group';
import {rtcmWireValue} from './rtcm-group';

describe('message flag conversion', () => {
    it('converts names to UI flags and back', () => {
        const flags = msgFlag(PVTMsgPos, PVTMsgNames) | msgFlag(PVTMsgOff, PVTMsgNames);
        expect(msgFlagNames(flags, PVTMsgNames)).toEqual([PVTMsgPos, PVTMsgOff]);
    });

    it('rejects unknown names', () => {
        expect(() => msgFlag('unknown' as any, NMEASelectableMsgNames)).toThrow('unknown message flag name: unknown');
        expect(() => msgFlag(NMEAMsgOther as any, NMEASelectableMsgNames)).toThrow('unknown message flag name: other');
    });
});

describe('message wire values', () => {
    it('builds NMEA arrays including other', () => {
        expect(nmeaWireValue(false, false, 0)).toBeUndefined();
        expect(nmeaWireValue(true, true, 0)).toEqual([]);
        expect(nmeaWireValue(true, false, nmeaFlag(NMEAMsgGGA))).toEqual(['GGA', 'other']);
    });

    it('builds PVT arrays', () => {
        expect(pvtWireValue(true, pvtFlag(PVTMsgPos) | pvtFlag(PVTMsgOff))).toEqual(['pos', 'off']);
    });

    it('builds RTCM arrays', () => {
        expect(rtcmWireValue(true, false, 'msm7', true, true)).toEqual(['other', 'MSM7', 'lax', 'ARP']);
    });
});

import {
    sortRTCMMsgIDs,
    validateEvent,
    RTCMState,
} from './dashboard';

test('RTCMState.update: pull-source events accumulate by msgID', () => {
    const s = new RTCMState('pull');
    s.update({ source: 'pull', msgID: '1077' });
    s.update({ source: 'pull', msgID: '1087' });
    s.update({ source: 'pull', msgID: '1077' });
    expect(s.source).toBe('pull');
    expect(s.totalCount).toEqual({ '1077': 2, '1087': 1 });
    expect(s.unusedCount).toBeNull();
    expect(s.title()).toBe('RTCM Messages Received');
    expect(s.rowValue('1077')).toBe('2');
});

test('RTCMState.update: receiver event after pull clears counts', () => {
    const s = new RTCMState('pull');
    s.update({ source: 'pull', msgID: '1077' });
    s.update({ source: 'pull', msgID: '1087' });
    s.update({ source: 'receiver', msgID: '1077', used: true });
    expect(s.source).toBe('receiver');
    expect(s.totalCount).toEqual({ '1077': 1 });
    expect(s.unusedCount).toEqual({});
    expect(s.title()).toBe('RTCM Messages Used/Received');
    expect(s.rowValue('1077')).toBe('1/1');
});

test('RTCMState.update: receiver event with no used field stays null', () => {
    const s = new RTCMState('receiver');
    s.update({ source: 'receiver', msgID: '1077' });
    s.update({ source: 'receiver', msgID: '1077' });
    expect(s.unusedCount).toBeNull();
    expect(s.totalCount).toEqual({ '1077': 2 });
    expect(s.title()).toBe('RTCM Messages Used');
    expect(s.rowValue('1077')).toBe('2');
});

test('RTCMState.update: used:false increments unusedCount but not used', () => {
    const s = new RTCMState('receiver');
    s.update({ source: 'receiver', msgID: '1230', used: true });
    s.update({ source: 'receiver', msgID: '1230', used: false });
    expect(s.totalCount).toEqual({ '1230': 2 });
    expect(s.unusedCount).toEqual({ '1230': 1 });
    expect(s.rowValue('1230')).toBe('1/2');
});

test('validateEvent corReport: rejects non-RTCM, empty msgID, bad checksum', () => {
    const base = { tag: 'RTCM', msgID: '1077', source: 'pull', checksumOK: true };
    expect(validateEvent('corReport', base)).not.toBeNull();
    expect(validateEvent('corReport', { ...base, tag: 'NMEA' })).toBeNull();
    expect(validateEvent('corReport', { ...base, msgID: '' })).toBeNull();
    expect(validateEvent('corReport', { ...base, checksumOK: false })).toBeNull();
    expect(validateEvent('corReport', { ...base, source: 'other' })).toBeNull();
});

test('sortRTCMMsgIDs: numeric and subtype ordering', () => {
    expect(sortRTCMMsgIDs(['1230', '4072.10', '1077', '4072.0', '4072.1']))
        .toEqual(['1077', '1230', '4072.0', '4072.1', '4072.10']);
});

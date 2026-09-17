import {
    sortCorrectionMsgIDs,
    validateEvent,
    CorrectionsState,
} from './dashboard';

test('CorrectionsState.update: pull-source events accumulate by msgID', () => {
    const s = new CorrectionsState('RTCM', 'pull');
    s.update({ tag: 'RTCM', source: 'pull', msgID: '1077' });
    s.update({ tag: 'RTCM', source: 'pull', msgID: '1087' });
    s.update({ tag: 'RTCM', source: 'pull', msgID: '1077' });
    expect(s.source).toBe('pull');
    expect(s.totalCount).toEqual({ '1077': 2, '1087': 1 });
    expect(s.unusedCount).toBeNull();
    expect(s.title()).toBe('RTCM Messages Received');
    expect(s.rowValue('1077')).toBe('2');
});

test('CorrectionsState.update: receiver event after pull clears counts', () => {
    const s = new CorrectionsState('RTCM', 'pull');
    s.update({ tag: 'RTCM', source: 'pull', msgID: '1077' });
    s.update({ tag: 'RTCM', source: 'pull', msgID: '1087' });
    s.update({ tag: 'RTCM', source: 'receiver', msgID: '1077', used: true });
    expect(s.source).toBe('receiver');
    expect(s.totalCount).toEqual({ '1077': 1 });
    expect(s.unusedCount).toEqual({});
    expect(s.title()).toBe('RTCM Messages Used/Received');
    expect(s.rowValue('1077')).toBe('1/1');
});

test('CorrectionsState.update: receiver event with no used field stays null', () => {
    const s = new CorrectionsState('RTCM', 'receiver');
    s.update({ tag: 'RTCM', source: 'receiver', msgID: '1077' });
    s.update({ tag: 'RTCM', source: 'receiver', msgID: '1077' });
    expect(s.unusedCount).toBeNull();
    expect(s.totalCount).toEqual({ '1077': 2 });
    expect(s.title()).toBe('RTCM Messages Used');
    expect(s.rowValue('1077')).toBe('2');
});

test('CorrectionsState.update: used:false increments unusedCount but not used', () => {
    const s = new CorrectionsState('RTCM', 'receiver');
    s.update({ tag: 'RTCM', source: 'receiver', msgID: '1230', used: true });
    s.update({ tag: 'RTCM', source: 'receiver', msgID: '1230', used: false });
    expect(s.totalCount).toEqual({ '1230': 2 });
    expect(s.unusedCount).toEqual({ '1230': 1 });
    expect(s.rowValue('1230')).toBe('1/2');
});

test('CorrectionsState: SPARTN tag accumulates dotted msgIDs', () => {
    const s = new CorrectionsState('SPARTN', 'pull');
    s.update({ tag: 'SPARTN', source: 'pull', msgID: '0.0' });
    s.update({ tag: 'SPARTN', source: 'pull', msgID: '0.1' });
    s.update({ tag: 'SPARTN', source: 'pull', msgID: '0.0' });
    expect(s.totalCount).toEqual({ '0.0': 2, '0.1': 1 });
    expect(s.title()).toBe('SPARTN Messages Received');
    expect(sortCorrectionMsgIDs(['0.1', '0.0'])).toEqual(['0.0', '0.1']);
});

test('validateEvent corReport: accepts RTCM and SPARTN, rejects unknown tag, empty msgID, bad checksum', () => {
    const base = { tag: 'RTCM', msgID: '1077', source: 'pull', checksumOK: true };
    expect(validateEvent('corReport', base)).not.toBeNull();
    expect(validateEvent('corReport', { ...base, tag: 'SPARTN', msgID: '0.0' })).not.toBeNull();
    expect(validateEvent('corReport', { ...base, tag: 'NMEA' })).toBeNull();
    expect(validateEvent('corReport', { ...base, msgID: '' })).toBeNull();
    expect(validateEvent('corReport', { ...base, checksumOK: false })).toBeNull();
    expect(validateEvent('corReport', { ...base, source: 'other' })).toBeNull();
});

test('sortCorrectionMsgIDs: numeric and subtype ordering', () => {
    expect(sortCorrectionMsgIDs(['1230', '4072.10', '1077', '4072.0', '4072.1']))
        .toEqual(['1077', '1230', '4072.0', '4072.1', '4072.10']);
});

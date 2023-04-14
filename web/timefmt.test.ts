import { formatTimestamp } from './timefmt';

test('formatTimestamp', () => {
    const parts = formatTimestamp("2020-01-15T11:24:59Z", "en-US")
    expect(parts).not.toBeNull()
    expect(parts!.date).toContain("2020")
    expect(parts!.date).toContain("January")
    expect(parts!.time).toContain("59")
});
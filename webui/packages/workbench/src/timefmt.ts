const timeOptions: Intl.DateTimeFormatOptions = {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
};

const dateOptions: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
};

export type DateTime = {
    date: string;
    time: string;
};

const timestampRegex: RegExp = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:)(\d{2})(?:\.\d+)?(?:[+-]00:00|Z)$/;

export function formatUTCLocal(t: string, locales?: string | string[]): DateTime | null {
    const match = timestampRegex.exec(t);
    if (!match) return null;
    const [, initial, second] = match;
    // Extract second separately so leap seconds format correctly.
    // Timezone offsets are always integral minutes, so seconds are unaffected.
    const d = new Date(initial + '00Z');
    return formatDateLocal(d, second, locales);
}

function formatDateLocal(d: Date, second: string, locales?: string | string[]): DateTime {
    const date = d.toLocaleDateString(locales, dateOptions);
    const parts = Intl.DateTimeFormat(locales, timeOptions).formatToParts(d);
    let time = '';
    for (const {type, value} of parts) {
        switch (type) {
            case 'hour':
            case 'minute':
            case 'literal':
                time += value;
                break;
            case 'second':
                time += second;
                break;
        }
    }
    return {date, time};
}

// parseTAITime parses a ptime.Time string ("seconds.nanoseconds") to seconds.
export function parseTAITime(t: string): number {
    const dot = t.indexOf('.');
    if (dot < 0) return parseInt(t, 10);
    return parseInt(t.substring(0, dot), 10);
}

// taiToUTC computes a UTC ISO string from TAI seconds and a leap second offset.
export function taiToUTC(taiSecs: number, utcOff: number): string {
    const utcSecs = taiSecs - utcOff;
    return new Date(utcSecs * 1000).toISOString();
}

export function formatTAI(secs: number): string {
    return formatDateTime(new Date(secs * 1000).toISOString());
}

// Separate date and time with a space instead of T, and strip timezone.
export function formatDateTime(t: string): string {
    return t.replace('T', ' ').substring(0, 19);
}

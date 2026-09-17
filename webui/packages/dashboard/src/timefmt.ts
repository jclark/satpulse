
const timeOptions : Intl.DateTimeFormatOptions = {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZoneName: "short"
}

const dateOptions : Intl.DateTimeFormatOptions = {
    year: "numeric",
    month: "long",
    day: "numeric",
}


export type TimePart = {
    type: "time"|"timeZoneName"|"literal";
    value: string;
}

export type DateTime = {
    date: string;
    time: string;
}

const timestampRegex: RegExp = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:)(\d{2})(?:\.\d+)?(?:[+-]00:00|Z)$/;

export function formatUTCLocal(t: string, locales?: string|string[]|undefined) : DateTime|null {
    const match = timestampRegex.exec(t)
    if (!match) {
        return null
    }

    const [, initial, second,] = match;

    // We take the second out here, and put it back later, so that we can format leap seconds correctly.
    // Offset for normal time zones is always integral number of minutes.
    const d = new Date(initial + "00Z");

    return formatDateLocal(d, second, locales)
}

function formatDateLocal(d: Date, second: string, locales?: string|string[]|undefined) : DateTime {
    const date = d.toLocaleDateString(locales, dateOptions)
   
    const parts = Intl.DateTimeFormat(locales, timeOptions).formatToParts(d)

    let time = ""

    for (const { type, value } of parts) {
        switch (type) {
        case 'hour': case 'minute': case 'timeZoneName': case 'literal':
            time += value;
            break
        case 'second':
            time += second
            break
        }
    }
    return { date, time }
}

export function formatNanoseconds(t: number): string {
    // We could use Intl.NumberFormat here, but I'm not sure whether that's a good idea.
    if (t >= 1e9) {
        return (t / 1e9).toFixed(3) + " s"
    }
    if (t >= 1e6) {
        return (t / 1e6).toFixed(3) + " ms"
    }
    if (t >= 1e3) {
        return (t / 1e3).toFixed(3) + " \u00B5s"
    }
    return t + " ns"
}

export function formatTAI(secs: number): string {
    return formatDateTime(new Date(secs * 1000).toISOString())
}

// This separates the date and time with a space, rather than T, and removes the timezone.
export function formatDateTime(t: string): string {
    return t.replace('T', ' ').substring(0, 19);
}
  
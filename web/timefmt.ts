
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

const timestampRegex: RegExp = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:)(\d{2})(?:\.\d+)?([+-]\d{2}:\d{2}|Z)$/;

export function formatTimestamp(t: string, locales?: string|string[]|undefined) : DateTime|null {
    const match = timestampRegex.exec(t)
    if (!match) {
        return null
    }

    const [, initial, second, tzOffset] = match;
    if (tzOffset != "" && tzOffset != "Z" && tzOffset.substring(1) != "00:00") {
        return null
    }

    // We take the second out here, and put it back later, so that we can format leap seconds correctly.
    // Offset for normal time zones is always integral number of minutes.
    const d = new Date(initial + "00Z");

    return formatDate(d, second, locales)
}

export function formatDate(d: Date, second: string, locales?: string|string[]|undefined) : DateTime {
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
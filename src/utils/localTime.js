// Instants on the Job surfaces, in the reader's own zone and in one form: the
// /jobs cards' "2026-09-26 14:17", with seconds where the moment matters.
// ISO order and a 24-hour clock read the same whatever the browser's locale,
// so a card and the Job page never write one instant two ways.

function pad(value) {
    return String(value).padStart(2, '0');
}

/** "2026-09-26 14:17", or with `seconds` "2026-09-26 14:17:13"; '' for no instant. */
export function formatLocalTime(value, { seconds = false } = {}) {
    if (value === null || value === undefined || value === '') return '';
    const date = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    const minute = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
    return seconds ? `${minute}:${pad(date.getSeconds())}` : minute;
}

/** The reader's time zone as the browser names it ("Europe/Berlin"), or ''. */
export function readerTimeZone() {
    try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || '';
    } catch {
        return '';
    }
}

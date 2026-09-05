// Date/time fields are wall-clock components plus an optional fixed offset.
// Avoid Date.parse: it guesses formats, normalizes invalid dates and shifts zones.
type Parts = { year: number; month: number; day: number; hour: number; minute: number; second: number; fraction: string; zone: string };
type Segment = { token: string; pattern: string } | { literal: string };
const months = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];
const tokens: Record<string, string> = {
  '2006': '\\d{4}', January: months.join('|'), Jan: months.map(m => m.slice(0, 3)).join('|'),
  '01': '\\d{2}', '02': '\\d{2}', '15': '\\d{2}', '03': '\\d{2}', '04': '\\d{2}', '05': '\\d{2}',
  '1': '\\d{1,2}', '2': '\\d{1,2}', '3': '\\d{1,2}', '4': '\\d{1,2}', '5': '\\d{1,2}',
  PM: 'AM|PM', pm: 'am|pm', 'Z07:00': 'Z|[+-]\\d{2}:\\d{2}', '-07:00': '[+-]\\d{2}:\\d{2}',
  'Z0700': 'Z|[+-]\\d{4}', '-0700': '[+-]\\d{4}',
};
const tokenNames = Object.keys(tokens).sort((a, b) => b.length - a.length);
const escapeRegex = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const pad = (n: number, width = 2) => String(n).padStart(width, '0');

// Supported Go layout tokens are shared by storage and display layouts.
function segments(layout: string): Segment[] {
  const result: Segment[] = [];
  for (let i = 0; i < layout.length;) {
    const fraction = layout.slice(i).match(/^\.(0{1,9})(?!0)/)?.[0];
    const token = fraction || tokenNames.find(t => layout.startsWith(t, i));
    if (token) {
      result.push({ token, pattern: fraction ? `\\.\\d{${fraction.length - 1}}` : tokens[token] });
      i += token.length;
    } else {
      result.push({ literal: layout[i++] });
    }
  }
  return result;
}

function valid(p: Parts): boolean {
  const leap = p.year % 4 === 0 && (p.year % 100 !== 0 || p.year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  const zone = p.zone.replace(':', '').match(/^[+-](\d{2})(\d{2})$/);
  return p.year >= 1 && p.year <= 9999 && p.month >= 1 && p.month <= 12 && p.day >= 1 && p.day <= days[p.month - 1]
    && p.hour >= 0 && p.hour < 24 && p.minute >= 0 && p.minute < 60 && p.second >= 0 && p.second < 60
    && (!zone || (+zone[1] < 24 && +zone[2] < 60));
}

function parse(value: unknown, layout: string): Parts | null {
  if (typeof value !== 'string' || !value || !layout) return null;
  const segs = segments(layout);
  const match = new RegExp('^' + segs.map(s => 'literal' in s ? escapeRegex(s.literal) : `(${s.pattern})`).join('') + '$').exec(value);
  if (!match || match[0] !== value) return null;
  const p: Parts = { year: 2000, month: 1, day: 1, hour: 0, minute: 0, second: 0, fraction: '', zone: '' };
  let index = 1;
  let meridiem = '';
  let twelveHour = false;
  let hasField = false;
  for (const s of segs) {
    if ('literal' in s) continue;
    hasField = true;
    const v = match[index++];
    switch (s.token) {
      case '2006': p.year = +v; break;
      case 'January': p.month = months.indexOf(v) + 1; break;
      case 'Jan': p.month = months.findIndex(m => m.startsWith(v)) + 1; break;
      case '01': case '1': p.month = +v; break;
      case '02': case '2': p.day = +v; break;
      case '15': p.hour = +v; break;
      case '03': case '3': p.hour = +v; twelveHour = true; break;
      case '04': case '4': p.minute = +v; break;
      case '05': case '5': p.second = +v; break;
      case 'PM': case 'pm': meridiem = v.toLowerCase(); break;
      default: if (s.token.startsWith('.')) p.fraction = v.slice(1); else p.zone = v;
    }
  }
  if (twelveHour) {
    if (p.hour < 1 || p.hour > 12) return null;
    p.hour = p.hour % 12 + (meridiem === 'pm' ? 12 : 0);
  }
  return hasField && valid(p) ? p : null;
}

function format(p: Parts, layout: string): string {
  const values: Record<string, string> = {
    '2006': pad(p.year, 4), January: months[p.month - 1], Jan: months[p.month - 1].slice(0, 3),
    '01': pad(p.month), '1': String(p.month), '02': pad(p.day), '2': String(p.day),
    '15': pad(p.hour), '03': pad(p.hour % 12 || 12), '3': String(p.hour % 12 || 12),
    '04': pad(p.minute), '4': String(p.minute), '05': pad(p.second), '5': String(p.second),
    PM: p.hour < 12 ? 'AM' : 'PM', pm: p.hour < 12 ? 'am' : 'pm',
  };
  return segments(layout).map(s => {
    if ('literal' in s) return s.literal;
    if (s.token.startsWith('.')) return '.' + p.fraction.padEnd(s.token.length - 1, '0').slice(0, s.token.length - 1);
    if (s.token in values) return values[s.token];
    if (!p.zone) return ''; // Never invent a timezone for a local value.
    const compact = p.zone === 'Z' ? '+0000' : p.zone.replace(':', '');
    if (s.token.startsWith('Z') && compact === '+0000') return 'Z';
    return s.token.includes(':') ? compact.slice(0, 3) + ':' + compact.slice(3) : compact;
  }).join('');
}

function inferLayout(value: unknown): string | null {
  if (typeof value !== 'string') return null;
  const match = /^(?:(\d{4}-\d{2}-\d{2})([Tt ]))?(\d{2}:\d{2})(:\d{2})?(\.\d{1,9})?(Z|[+-]\d{2}:?\d{2})?$/.exec(value);
  if (match) return (match[1] ? '2006-01-02' + match[2] : '') + '15:04' + (match[4] ? ':05' : '')
    + (match[5] ? '.' + '0'.repeat(match[5].length - 1) : '')
    + (match[6] ? (match[6] === 'Z' ? 'Z07:00' : match[6].includes(':') ? '-07:00' : '-0700') : '');
  return /^\d{4}-\d{2}-\d{2}$/.test(value) ? '2006-01-02' : null;
}

function inputType(layout: string): 'date' | 'time' | 'datetime-local' {
  const fields = segments(layout).flatMap(s => 'token' in s ? [s.token] : []);
  const date = fields.some(t => ['2006', 'January', 'Jan', '01', '1', '02', '2'].includes(t));
  const time = fields.some(t => ['15', '03', '3', '04', '4', '05', '5'].includes(t));
  return date ? time ? 'datetime-local' : 'date' : 'time';
}

export function dateTimeField(value: unknown, schema: { format?: string; default?: unknown } | null, inputLayout = '', displayLayout = '') {
  const defaultLayout = schema?.format === 'date' ? '2006-01-02'
    : schema?.format === 'time' ? '15:04:05' : schema?.format === 'date-time' ? '2006-01-02T15:04:05Z07:00' : '2006-01-02T15:04:05';
  const currentLayout = inputLayout || inferLayout(value) || defaultLayout;
  const current = parse(value, currentLayout);
  // Only fall back in the editor; the original invalid value remains the display.
  const layout = current ? currentLayout : inputLayout || inferLayout(schema?.default) || defaultLayout;
  const initial = current || parse(schema?.default, layout);
  const type = inputType(layout);
  const nativeLayout = type === 'date' ? '2006-01-02' : type === 'time' ? '15:04:05' : '2006-01-02T15:04:05';
  return {
    type,
    display: current ? format(current, displayLayout || currentLayout)
      : value == null ? '' : typeof value === 'object' ? JSON.stringify(value) : String(value),
    input: initial ? format(initial, nativeLayout) + (type !== 'date' && initial.fraction ? '.' + initial.fraction.slice(0, 3) : '') : '',
    initialValue: initial ? format(initial, layout) : '',
    // Browser controls normalize seconds and fractions. Reapply the stored layout
    // and offset instead of serializing through UTC or losing precision on open.
    serialize(input: string): string | null {
      if (!input) return '';
      const inferred = inferLayout(input);
      const next = inferred ? parse(input, inferred) : null;
      if (!next) return null;
      const hasZone = segments(layout).some(s => 'token' in s && ['Z07:00', '-07:00', 'Z0700', '-0700'].includes(s.token));
      next.zone = initial?.zone || (hasZone ? 'Z' : '');
      return format(next, layout);
    },
  };
}

import { describe, expect, it } from 'vitest';
import { dateTimeField } from './dateTimeValue';

describe('date/time field formats', () => {
  it.each([
    ['2026-09-05', '2026-09-06', 'date', '2026-09-06'],
    ['14:30', '15:45:00', 'time', '15:45'],
    ['14:30:12.120+0230', '15:45:13.125', 'time', '15:45:13.125+0230'],
    ['2026-09-05T14:30:12Z', '2026-09-06T15:45:13', 'datetime-local', '2026-09-06T15:45:13Z'],
    ['2026-09-05 14:30:12.000+02:00', '2026-09-06T15:45:13', 'datetime-local', '2026-09-06 15:45:13.000+02:00'],
    ['2026-09-05T14:30:12+00:00', '2026-09-06T15:45:13', 'datetime-local', '2026-09-06T15:45:13+00:00'],
  ])('preserves the representation of %s', (value, input, type, saved) => {
    const field = dateTimeField(value, null);
    expect(field.type).toBe(type);
    expect(field.display).toBe(value);
    expect(field.initialValue).toBe(value);
    expect(field.serialize(input)).toBe(saved);
  });

  it('keeps custom storage separate from custom display', () => {
    const field = dateTimeField('05/09/2026 02:30 PM', null, '02/01/2006 03:04 PM', 'January 2, 2006 at 15:04');
    expect(field.input).toBe('2026-09-05T14:30:00');
    expect(field.display).toBe('September 5, 2026 at 14:30');
    expect(field.serialize('2027-01-02T00:45')).toBe('02/01/2027 12:45 AM');
  });

  it.each(['not a date', '2026-01-01\n', '2026-02-29', '2026-04-31', '2026-01-00', '2026-13-01', '2026-09-05T25:00:00Z', '12:60:00', '12:00:00+24:00', '<script>alert(1)</script>'])('displays invalid %s verbatim and edits the schema default', value => {
    const field = dateTimeField(value, { format: 'date', default: '2024-02-29' }, '', 'January 2, 2006');
    expect(field.display).toBe(value);
    expect(field.input).toBe('2024-02-29');
    expect(field.initialValue).toBe('2024-02-29');
  });

  it('uses the schema format when both value and default are absent or invalid', () => {
    expect(dateTimeField(null, { format: 'date' }).type).toBe('date');
    expect(dateTimeField('bad', { format: 'time', default: 'also bad' }).input).toBe('');
    expect(dateTimeField(null, { format: 'date-time' }).serialize('2026-09-05T14:00')).toBe('2026-09-05T14:00:00Z');
  });

  it('initializes an explicitly zoned custom format without an existing value', () => {
    const field = dateTimeField('bad', null, '02/01/2006 15:04 -07:00');
    expect(field.serialize('2026-09-05T14:30')).toBe('05/09/2026 14:30 +00:00');
  });

  it('rejects impossible input and permits clearing', () => {
    const field = dateTimeField('2024-02-29', null);
    expect(field.serialize('2026-02-29')).toBeNull();
    expect(field.serialize('')).toBe('');
  });

  it('preserves nanosecond precision when opening without editing', () => {
    const value = '2026-09-05T14:00:00.123456789-03:00';
    expect(dateTimeField(value, null).initialValue).toBe(value);
  });
});

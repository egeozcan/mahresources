import { getLabeledEnumEntries, isLabeledEnum, resolveSchema, type JSONSchema } from '../schema-editor/schema-core';

type PillValue = string | number | boolean | null;
export type PillOption = { value: PillValue; label: string; color?: string };

function isScalar(value: unknown): value is PillValue {
  return value === null || typeof value === 'string' || typeof value === 'boolean'
    || (typeof value === 'number' && Number.isFinite(value));
}

// Manual options replace schema options, retaining JSON types for editMeta.
// Reject the whole list on invalid input so a typo cannot silently remove choices.
export function pillOptions(manual: string, schema: JSONSchema | null): PillOption[] {
  let entries: unknown;
  if (manual.trim()) {
    try { entries = JSON.parse(manual); } catch { return []; }
  } else if (schema) {
    const resolved = isLabeledEnum(schema) ? schema : resolveSchema(schema, schema);
    if (resolved) entries = isLabeledEnum(resolved)
      ? getLabeledEnumEntries(resolved) : resolved.enum;
  }
  if (!Array.isArray(entries)) return [];
  const options: PillOption[] = [];
  for (const entry of entries) {
    const option = isScalar(entry) ? { value: entry, label: String(entry) } : entry;
    if (!option || !isScalar(option.value) || typeof option.label !== 'string') return [];
    if (!options.some(existing => existing.value === option.value)) options.push(option);
  }
  return options;
}

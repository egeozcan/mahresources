import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { JSONSchema } from '../schema-core';
import {
  resolveRef,
  mergeSchemas,
  getDefaultValue,
  scoreSchemaMatch,
  evaluateCondition,
  inferType,
  inferSchema,
  isLabeledEnum,
  getLabeledEnumEntries,
  valueText,
  choiceKey,
  choiceIndex,
  findChoice,
  withTypedKeywords,
  ownValue,
  setOwn,
  onRefPath,
  followingSteps,
  expansionStep,
  onFreshRefPath,
  mergeAllOf,
  withSchemaBudget,
  compilePattern,
  countSchemaWork,
  sameValue,
  valueKey,
} from '../schema-core';
import { isLeafSchema, stripStaleKeys } from '../form-mode-helpers';
import { schemaFallback, guardSchemaAction, schemaActionError, clearSchemaActionError } from '../schema-fallback';

function generateFieldId(prefix: string, path: string): string {
  // Injective encoding: every non-alphanumeric character is replaced with
  // _XX (two-digit hex of its char code).  This guarantees distinct paths
  // always produce distinct IDs — no two different inputs can collide.
  const encoded = path.replace(/[^a-zA-Z0-9]/g, (ch) => {
    return '_' + ch.charCodeAt(0).toString(16).padStart(2, '0');
  });
  return `${prefix}-${encoded}`;
}

/**
 * BH-010: Render numeric input values with preview-aware semantics.
 * - undefined/null → empty input
 * - data === 0 AND the schema has no explicit `default` → empty input
 *   (a stray 0 from legacy callers must not seed the Preview Form with 0)
 * - otherwise → String(data)
 *
 * Rationale: when the Preview Form renders a number field with {minimum: 1900},
 * a seeded `0` fires a bogus "Must be at least 1900" onBlur. Treating `0` as
 * "empty" in the no-default case matches what the user expected to see.
 */
function formatNumericForInput(data: any, schema: JSONSchema): string {
  if (data === undefined || data === null) return '';
  if (data === 0 && !('default' in schema)) return '';
  return valueText(data);
}

/** The decimal places in x's shortest text, scientific notation included (1.5e-7: 8). */
function decimalPlaces(x: number): number {
  const [mantissa, exponent = '0'] = String(Math.abs(x)).split('e');
  const fraction = mantissa.split('.')[1]?.length ?? 0;
  return Math.max(0, fraction - Number(exponent));
}

/**
 * Whether n is a multiple of m. 0.3 % 0.1 is not 0 in floating point, though
 * 0.3 is 3 x 0.1, so the check works at m's decimal places: a value with more
 * places than m is no multiple of it (10000000.005 of 0.01), and otherwise
 * both are scaled to integers and divided exactly. Past exact integer range
 * a whole m divides exactly too, and only a fractional m falls back to a
 * rounding-error-sized tolerance.
 */
function isMultipleOf(n: number, m: number): boolean {
  if (!(m > 0)) return true;
  const quotient = n / m;
  // Past the float range (1e308 / 0.5) the quotient is Infinity; keep base's check.
  if (!Number.isFinite(quotient)) return n % m === 0;
  const places = decimalPlaces(m);
  if (decimalPlaces(n) > places) return false;
  const scale = 10 ** places;
  const scaledN = Math.round(n * scale);
  const scaledM = Math.round(m * scale);
  if (Number.isSafeInteger(scaledN) && Number.isSafeInteger(scaledM) && scaledM > 0) return scaledN % scaledM === 0;
  // Past that range a float n is a whole number, and % is exact for whole m.
  if (Number.isInteger(m)) return n % m === 0;
  return Math.abs(quotient - Math.round(quotient)) <= 4 * Number.EPSILON * Math.abs(quotient);
}

/**
 * A write the form makes on its own, not from user input: a default put in
 * place (init) or a const enforced (const). It travels up the onChange chain
 * like a value, each composer prefixing its key (under), and is applied at
 * the root to the form's current value (_applyAutoWrite). An init write sets
 * only its own path, never a stale snapshot of its parent, so sibling
 * initialisations no longer overwrite one another; where two inits target
 * the same path the later one wins, as base's chain composed them. A const
 * write applies only if its path still holds the value it was computed
 * from (expected). A write computed by a render that saw a whole value its
 * automatic sequence already rendered is a cycle and dropped.
 */
export class AutoWrite {
  constructor(
    readonly path: ReadonlyArray<string | number>,
    readonly value: unknown,
    readonly expected: unknown,
    readonly kind: 'init' | 'const',
    /** Computed by a render that saw a whole value its automatic sequence already rendered: a cycle. */
    readonly fromRepeatedState = false,
  ) {}

  under(key: string | number): AutoWrite {
    return new AutoWrite([key, ...this.path], this.value, this.expected, this.kind, this.fromRepeatedState);
  }
}

/** The value at path in value, or undefined. */
function valueAtPath(value: unknown, path: ReadonlyArray<string | number>): unknown {
  let current: any = value;
  for (const key of path) {
    if (current === null || typeof current !== 'object') return undefined;
    current = ownValue(current, String(key));
  }
  return current;
}

/** value with path set to next, copying each container on the way; missing containers are created. */
function withValueAtPath(value: unknown, path: ReadonlyArray<string | number>, next: unknown): unknown {
  if (path.length === 0) return next;
  const [key, ...rest] = path;
  const container: any = Array.isArray(value) ? [...value]
    : value !== null && typeof value === 'object' ? { ...(value as object) }
      : typeof key === 'number' ? [] : {};
  setOwn(container, String(key), withValueAtPath(ownValue(container, String(key)), rest, next));
  return container;
}

/** onChange for the child at key: an automatic write gains the key; a value is composed by compose. */
function childChange(onChange: (val: any) => void, key: string | number, compose: (val: any) => unknown): (val: any) => void {
  return (val: any) => (val instanceof AutoWrite ? onChange(val.under(key)) : onChange(compose(val)));
}


/**
 * A deep copy of a default value before it becomes editable data:
 * structuredClone, or a JSON round trip for a value it cannot clone, or the
 * value itself when neither can copy it.
 */
function copyDefault<T>(value: T): T {
  if (value === null || typeof value !== 'object') return value;
  try {
    return structuredClone(value);
  } catch {
    try {
      return JSON.parse(JSON.stringify(value));
    } catch {
      return value;
    }
  }
}

/**
 * A last resort against a stack overflow on pathological acyclic input; cycles
 * are detected by reference path, so real schemas never come close.
 */
const MAX_FORM_DEPTH = 1000;

/** The option key of a stored value that matches no choice. */
const CURRENT_CHOICE_KEY = 'current';

/** The visible text of an enum choice: JSON for a non-primitive, else the value itself. */
function optionText(value: any): any {
  return value !== null && typeof value === 'object' ? JSON.stringify(value) : value;
}

@customElement('schema-form-mode')
export class SchemaFormMode extends LitElement {
  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Object }) value: any = {};
  @property({ type: String }) name = 'Meta';

  @state() private _data: any = {};
  // BH-009: reactive map of fieldId → error message; drives aria-invalid in templates
  @state() private _errors: Map<string, string> = new Map();

  // Light DOM hidden input for form submission
  private _hiddenInput: HTMLInputElement | null = null;

  // Bound submit handler so we can remove it on disconnect
  private _submitHandler: ((e: Event) => void) | null = null;
  // Whether we set noValidate on the ancestor form (so we can restore it)
  private _setNoValidate = false;

  // Render in light DOM to inherit Tailwind styles from the host page
  override createRenderRoot() {
    return this;
  }

  override connectedCallback() {
    super.connectedCallback();
    this._hiddenInput = document.createElement('input');
    this._hiddenInput.type = 'hidden';
    this._hiddenInput.name = this.name;
    this._hiddenInput.value = JSON.stringify(this.value ?? {});
    this.appendChild(this._hiddenInput);

    // BH-009: Hook the ancestor <form>'s submit event so validation errors
    // surface even when the user never blurred the field.
    // Disable native form validation (noValidate) so the browser doesn't block
    // the submit event before our handler fires (which would prevent us from
    // showing inline validation errors for schema-driven required fields).
    const form = this.closest('form');
    if (form && !form.noValidate) {
      form.noValidate = true;
      this._setNoValidate = true;
    }
    if (form) {
      this._submitHandler = this._act((ev: Event) => {
        // Errors are recomputed from the fields as they are now; one left by a
        // field that has since been replaced must not block submission.
        this._errors = new Map();
        // On form submit, validate all schema-managed inputs and set errors in
        // reactive _errors state. We don't use native `required` to avoid the
        // browser blocking the submit event before our handler fires.
        // Walk every input inside schema-form-mode and compute errors directly.
        const newErrors = new Map<string, string>();
        this.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
          'input:not([type="hidden"]),textarea'
        ).forEach((el) => {
          const id = el.id;
          if (!id) return;
          // Dispatch blur so onBlur handlers also fire (for user-typed values)
          el.dispatchEvent(new FocusEvent('blur', { bubbles: false }));
          // Directly validate: aria-required is set for required fields.
          // Since we removed the native `required` attr, check value directly.
          const isReq = el.getAttribute('aria-required') === 'true';
          const val = el.value;
          let errorText = '';
          const isNumberInput = el instanceof HTMLInputElement && el.type === 'number';
          if (isReq && val === '') {
            errorText = 'This field is required';
          } else if (val !== '') {
            if (isNumberInput) {
              const num = parseFloat(val);
              if (isNaN(num)) {
                errorText = 'Invalid number';
              } else {
                const min = el.getAttribute('min');
                const max = el.getAttribute('max');
                const step = el.getAttribute('step');
                if (min !== null && num < parseFloat(min)) {
                  errorText = `Must be at least ${min}`;
                } else if (max !== null && num > parseFloat(max)) {
                  errorText = `Must be at most ${max}`;
                } else if (step === '1' && !Number.isInteger(num)) {
                  errorText = 'Must be a whole number';
                }
              }
            } else {
              const pat = el.getAttribute('pattern');
              const minLen = el instanceof HTMLInputElement ? el.minLength : -1;
              const maxLen = el instanceof HTMLInputElement ? el.maxLength : -1;
              const re = pat ? compilePattern(pat) : null;
              if (re && val !== '' && !re.test(val)) {
                errorText = `Must match the expected format (${pat})`;
              } else if (minLen > 0 && val.length < minLen) {
                errorText = `Must be at least ${minLen} characters`;
              } else if (maxLen > 0 && val.length > maxLen) {
                errorText = `Must be at most ${maxLen} characters`;
              }
            }
          }
          // The blur dispatched above ran the field's schema-aware check
          // (exclusive bounds, multipleOf, ...); its error blocks submit too.
          errorText ||= this._errors.get(id) || '';
          if (errorText) {
            newErrors.set(id, errorText);
          }
        });
        if (newErrors.size > 0) {
          this._errors = newErrors;
          ev.preventDefault();
          ev.stopPropagation();
        }
      });
      form.addEventListener('submit', this._submitHandler);
    }
  }

  override disconnectedCallback() {
    if (this._hiddenInput && this._hiddenInput.parentNode === this) {
      this.removeChild(this._hiddenInput);
    }
    this._hiddenInput = null;
    // Clean up the form submit listener and restore noValidate if we set it
    const form = this.closest('form');
    if (this._submitHandler) {
      if (form) {
        form.removeEventListener('submit', this._submitHandler);
      }
      this._submitHandler = null;
    }
    if (this._setNoValidate && form) {
      form.noValidate = false;
      this._setNoValidate = false;
    }
    super.disconnectedCallback();
  }

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('value') || changed.has('schema')) clearSchemaActionError(this);
    // When Alpine binds with :schema="currentSchema", the value may arrive as
    // a JSON string (from the API) rather than a parsed object.  Handle both.
    if (changed.has('schema') && typeof this.schema === 'string') {
      try {
        this.schema = JSON.parse(this.schema as unknown as string);
      } catch {
        this.schema = {};
      }
    }
    // A new schema is a fresh start for the loop guard on automatic assignments.
    if (changed.has('schema')) this._resetAutomaticStates();
    if (changed.has('value') || changed.has('schema')) {
      // Recomputed for every value: a failure caused by one value (a strip
      // overflowing on it, a value that cannot be copied) never outlives it.
      this._valueError = null;
      try {
        this._data = this.value != null ? (typeof this.value === 'string' ? this._safeParse(this.value) : structuredClone(this.value)) : {};
        // So is a value from outside, but not an echo of the value this form
        // just emitted (a wrapper such as schema-editor passes it back as
        // text): that echo is how an automatic loop keeps going.
        if (changed.has('value') && !sameValue(this._data, this._emittedValue)) this._resetAutomaticStates();

        // When the schema changes, recursively strip keys from _data that aren't
        // in the new schema's properties and aren't allowed by
        // additionalProperties.  This prevents stale keys from a previous schema
        // (e.g. after a category switch) from being silently submitted via the
        // hidden input — including nested objects with additionalProperties:false.
        if (changed.has('schema') && this._data && typeof this._data === 'object') {
          const before = JSON.stringify(this._data);
          try {
            withSchemaBudget(() => stripStaleKeys(this._data, this.schema, this.schema));
          } catch (error) {
            // The data is kept as it was; render shows the fallback until
            // the next value.
            this._valueError = error;
          }
          const after = JSON.stringify(this._data);

          // If keys were actually stripped, notify the Alpine wrapper so its
          // currentMeta stays in sync. Without this, the stale keys persist in
          // currentMeta and get rehydrated when the schema changes again (or
          // when x-if recreates the component).
          if (before !== after) {
            if (this._hiddenInput) {
              this._hiddenInput.value = after;
            }
            // Dispatch after the current Lit update cycle completes to avoid
            // side-effects during willUpdate.
            this.updateComplete.then(this._act(() => {
              this.dispatchEvent(new CustomEvent('value-change', {
                detail: { value: this._data },
                bubbles: true,
                composed: true,
              }));
            }));
          }
        }

        // Keep hidden input in sync when value/schema change externally
        // (e.g. when Alpine passes currentMeta with pre-existing edits).
        if (this._hiddenInput) {
          this._hiddenInput.value = JSON.stringify(this._data);
        }
      } catch (error) {
        // Cloning or serialising the whole value (too deep, not cloneable)
        // failed: render shows the fallback instead of the update throwing.
        this._data = this.value;
        this._valueError = error;
      }
    }
  }

  private _safeParse(s: string): any {
    try { return JSON.parse(s); } catch { return {}; }
  }

  private _emitChange(newValue: any, automatic = false) {
    // User input starts the const loop guard afresh.
    if (!automatic) this._resetAutomaticStates();
    this._emittedValue = newValue;
    this._data = newValue;
    // Keep the public `value` property in sync so that a subsequent schema
    // change (which triggers willUpdate → rehydrate from `this.value`) does
    // not overwrite the user's in-progress edits.
    this.value = newValue;
    if (this._hiddenInput) {
      this._hiddenInput.value = JSON.stringify(newValue);
    }
    this.dispatchEvent(new CustomEvent('value-change', {
      detail: { value: newValue },
      bubbles: true,
      composed: true,
    }));
    this.requestUpdate();
  }

  // BH-009: Store validation errors in reactive state so Lit templates can
  // render aria-invalid and error text without being overwritten on re-render.
  private _setError(fieldId: string, message: string) {
    const updated = new Map(this._errors);
    updated.set(fieldId, message);
    this._errors = updated;
  }

  private _clearError(fieldId: string) {
    if (!this._errors.has(fieldId)) return;
    const updated = new Map(this._errors);
    updated.delete(fieldId);
    this._errors = updated;
  }

  // ─── Render ──────────────────────────────────────────────────────────────

  /**
   * Run callback in a microtask, under the action boundary: a deferred
   * initialisation (a variant default, an object or array put in place) can
   * throw as an action can, and must fall back the same way.
   */
  private _defer(callback: () => unknown): void {
    queueMicrotask(this._act(callback));
  }

  /**
   * The whole-form values (value keys) rendered during the current
   * automatic sequence. A sequence starts at user input, a value from outside
   * (not an echo) or a schema change.
   */
  private _renderedStates = new Set<string>();
  /** Whether the render in progress sees a whole value the sequence already rendered. */
  private _renderRepeatsState = false;
  private _warnedAutomaticLoop = false;
  /** The last value this form emitted, to tell its own value (echoed back) from one set from outside. */
  private _emittedValue: unknown = undefined;

  /** An automatic write computed by the render in progress. */
  private _autoWrite(path: ReadonlyArray<string | number>, value: unknown, expected: unknown, kind: 'init' | 'const'): AutoWrite {
    return new AutoWrite(path, value, expected, kind, this._renderRepeatsState);
  }

  /**
   * Note the whole value this render sees. With every write applied to the
   * current value, the automatic writes a render queues, and so the value
   * they leave, are a function of the schema and that value. A render that
   * sees a value its sequence already rendered would repeat the same writes
   * and come back here forever: an initialisation a conditional's cleanup
   * removes again, or branches setting one field to opposite consts (both
   * loops at base too). Its writes are dropped (_applyAutoWrite), the
   * current value is kept and the form warns once. Any other sequence of
   * values, however long, runs as at base.
   */
  private _noteRenderedState(): void {
    const key = valueKey(this._data);
    this._renderRepeatsState = this._renderedStates.has(key);
    this._renderedStates.add(key);
  }

  /**
   * Apply a write the form makes on its own to its current value. A const
   * write whose path no longer holds the value it was computed from is
   * stale and dropped (the next render computes it again if it still
   * applies). A write computed by a render that repeated a value of its
   * sequence is a cycle and dropped (_noteRenderedState).
   */
  private _applyAutoWrite(write: AutoWrite): void {
    const current = valueAtPath(this._data, write.path);
    if (write.kind === 'const' && !sameValue(current, write.expected)) return;
    const next = withValueAtPath(this._data, write.path, copyDefault(write.value));
    if (sameValue(next, this._data)) return;
    if (write.fromRepeatedState) {
      if (!this._warnedAutomaticLoop) {
        console.warn('schema-form-mode: automatic assignments would return to a value they already produced; stopped');
        this._warnedAutomaticLoop = true;
      }
      return;
    }
    this._emitChange(next, true);
  }

  private _resetAutomaticStates(): void {
    this._renderedStates = new Set();
    this._warnedAutomaticLoop = false;
  }

  /**
   * Whether to queue a variant default: never for an undefined default (a
   * reference cycle), which assigning would leave undefined, so every render
   * would queue it again. Any other default that cannot stick is a cycle of
   * whole values, which _noteRenderedState stops.
   */
  private _shouldInitialize(defaultVal: unknown): boolean {
    return defaultVal !== undefined;
  }

  /** handler under the action boundary: an error it throws shows the fallback (guardSchemaAction). */
  private _act<A extends unknown[]>(handler: (...args: A) => unknown): (...args: A) => unknown {
    return guardSchemaAction(this, 'schema-form-mode', handler);
  }

  override render() {
    const actionError = schemaActionError(this);
    if (actionError) return schemaFallback('schema-form-mode', actionError, this._data);
    if (!this.schema || Object.keys(this.schema).length === 0) {
      return nothing;
    }

    if (this._valueError) return schemaFallback('schema-form-mode', this._valueError, this._data);
    try {
      this._noteRenderedState();
      return withSchemaBudget(() => this._renderField(this.schema, this._data, (val: any) => {
        if (val instanceof AutoWrite) this._applyAutoWrite(val);
        else this._emitChange(val);
      }, this.schema, undefined, ''));
    } catch (error) {
      return schemaFallback('schema-form-mode', error, this._data);
    }
  }

  /** Set when processing the incoming value (copying it, stripping stale keys) threw; render shows the fallback until the next value. */
  private _valueError: unknown = null;


  // ─── Recursive field renderer (port of generateFormElement) ──────────────

  /** _renderFieldUnguarded behind the last-resort nesting limit. */
  private _renderField(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, fieldId?: string, parentPath?: string, describedBy?: string | null, isRequired?: boolean, parentRequired?: boolean): TemplateResult | typeof nothing {
    if (this._renderDepth >= MAX_FORM_DEPTH) {
      return html`<div class="text-red-500 text-xs">Unresolvable reference: the schema refers to itself</div>`;
    }
    countSchemaWork();
    this._renderDepth++;
    try {
      return this._renderFieldUnguarded(schema, data, onChange, rootSchema, fieldId, parentPath, describedBy, isRequired, parentRequired);
    } finally {
      this._renderDepth--;
    }
  }

  private _renderDepth = 0;

  private _renderFieldUnguarded(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, fieldId?: string, parentPath?: string, describedBy?: string | null, isRequired?: boolean, parentRequired?: boolean): TemplateResult | typeof nothing {
    schema = withTypedKeywords(schema);
    // Handle $ref
    if (schema.$ref) {
      // A reference already being expanded on this path is a cycle.
      const resolved = onRefPath(schema) ? null : resolveRef(schema.$ref, rootSchema);
      if (resolved) {
        const siblings: JSONSchema = { ...schema };
        delete siblings.$ref;
        const mergedSchema = mergeSchemas(resolved, siblings);
        const render = () => {
          return this._renderField(mergedSchema, data, onChange, rootSchema, fieldId, parentPath, describedBy, isRequired, parentRequired);
        };
        return followingSteps([expansionStep(schema)], render);
      }
      return html`<div class="text-red-500 text-xs">Unresolvable reference: ${valueText(schema.$ref)}</div>`;
    }

    // Handle labeled enum (oneOf with const+title)
    if (isLabeledEnum(schema)) {
      return this._renderLabeledEnum(schema, data, onChange, fieldId, describedBy, isRequired);
    }

    // Handle oneOf
    if (schema.oneOf && Array.isArray(schema.oneOf)) {
      return this._renderOneOf(schema, data, onChange, rootSchema, parentPath);
    }

    // Handle allOf - merge all schemas
    if (schema.allOf && Array.isArray(schema.allOf)) {
      const { merged, followed } = mergeAllOf(schema, rootSchema);
      const render = () => {
        return this._renderField(merged, data, onChange, rootSchema, fieldId, parentPath, describedBy, isRequired, parentRequired);
      };
      return followingSteps(followed, render);
    }

    // Handle anyOf
    if (schema.anyOf && Array.isArray(schema.anyOf)) {
      return this._renderAnyOf(schema, data, onChange, rootSchema, parentPath);
    }

    // Handle if/then/else
    if (schema.if) {
      return this._renderConditional(schema, data, onChange, rootSchema);
    }

    // Handle enum
    if (schema.enum) {
      return this._renderEnum(schema, data, onChange, fieldId, describedBy, isRequired);
    }

    // Handle const
    if (schema.const !== undefined) {
      // Compared by value and assigned as a copy: the schema's own const
      // object must never become editable data.
      if (!sameValue(data, schema.const)) {
        onChange(this._autoWrite([], schema.const, data, 'const'));
      }
      return html`<input type="text" .value=${valueText(schema.const)} disabled
        id=${fieldId || nothing}
        aria-label="Constant value"
        aria-describedby=${describedBy || nothing}
        ?required=${!!isRequired}
        aria-required=${isRequired ? 'true' : nothing}
        class="shadow-sm bg-gray-100 block w-full sm:text-sm border-gray-300 rounded-md mt-1 text-gray-500">`;
    }

    // Normalize type
    let type = schema.type;
    if (Array.isArray(type)) {
      const currentType = inferType(data);
      if (type.includes(currentType)) {
        type = currentType;
      } else {
        type = type.find((t: string) => t !== 'null') || type[0];
      }
    }
    type = type || inferType(data);

    // Null type
    if (type === 'null') {
      return this._renderNull(schema, onChange);
    }

    // Object type
    if (type === 'object') {
      return this._renderObject(schema, data, onChange, rootSchema, parentPath, parentRequired, isRequired);
    }

    // Array type
    if (type === 'array') {
      return this._renderArray(schema, data, onChange, rootSchema, parentPath);
    }

    // Primitive types (string, number, integer, boolean)
    return this._renderPrimitive(schema, type, data, onChange, fieldId, describedBy, isRequired);
  }

  // ─── oneOf ──────────────────────────────────────────────────────────────

  private _renderOneOf(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, path?: string): TemplateResult {
    // Extract shared sibling properties (declared alongside oneOf on the parent)
    const sharedProps = schema.properties;
    const sharedRequired = schema.required;

    let activeIndex = 0;
    if (data !== undefined) {
      let maxScore = -1;
      schema.oneOf.forEach((s: JSONSchema, idx: number) => {
        const score = scoreSchemaMatch(s, data, rootSchema);
        if (score > maxScore) {
          maxScore = score;
          activeIndex = idx;
        }
      });
    }

    if (data === undefined) {
      // Merge shared properties with first variant for initial default
      const merged = sharedProps
        ? mergeSchemas({ type: 'object', properties: sharedProps, required: sharedRequired }, schema.oneOf[activeIndex])
        : schema.oneOf[activeIndex];
      // A copy: the value becomes editable data, and the schema's own
      // default object must never be changed through it.
      const defaultVal = copyDefault(getDefaultValue(merged, rootSchema));
      if (this._shouldInitialize(defaultVal)) {
        this._defer(() => onChange(this._autoWrite([], defaultVal, undefined, 'init')));
      }
    }

    const onSelectChange = (e: Event) => {
      const idx = parseInt((e.target as HTMLSelectElement).value, 10);
      const optSchema = schema.oneOf[idx];
      const variantDefault = copyDefault(getDefaultValue(optSchema, rootSchema));
      // Preserve shared property data when switching variants
      if (sharedProps && data && typeof data === 'object' && typeof variantDefault === 'object') {
        const preserved: any = { ...variantDefault };
        for (const key of Object.keys(sharedProps)) {
          if (ownValue(data, key) !== undefined) setOwn(preserved, key, data[key]);
        }
        onChange(preserved);
      } else {
        onChange(variantDefault);
      }
    };

    // Merge shared properties into the active variant for rendering
    const activeVariant = schema.oneOf[activeIndex];
    const renderSchema = sharedProps
      ? mergeSchemas({ type: schema.type || 'object', properties: sharedProps, required: sharedRequired }, activeVariant)
      : activeVariant;

    return html`
      <div class="schema-nesting space-y-2 border-l-4 border-indigo-100 pl-4 py-2 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}
        ${schema.description ? html`<p class="text-xs text-gray-500 mb-2">${schema.description}</p>` : nothing}
        <select class="block w-full pl-3 pr-10 py-2 text-base border-gray-300 focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm rounded-md mb-2"
          aria-label=${schema.title ? `Select variant for ${schema.title}` : 'Select variant'}
          @change=${this._act(onSelectChange)}>
          ${schema.oneOf.map((opt: JSONSchema, idx: number) => {
            const o = withTypedKeywords(opt);
            let typeLabel = o.type;
            if (Array.isArray(o.type)) typeLabel = o.type.join('/');
            if (o.$ref) typeLabel = 'ref';
            return html`<option value=${idx} ?selected=${idx === activeIndex}>
              ${o.title || o.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`}
            </option>`;
          })}
        </select>
        <div>
          ${this._renderField(renderSchema, data, (val: any) => {
            onChange(val);
          }, rootSchema)}
        </div>
      </div>
    `;
  }

  // ─── anyOf ──────────────────────────────────────────────────────────────

  private _renderAnyOf(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, path?: string): TemplateResult {
    const sharedProps = schema.properties;
    const sharedRequired = schema.required;

    let activeIndex = 0;
    if (data !== undefined) {
      let maxScore = -1;
      schema.anyOf.forEach((s: JSONSchema, idx: number) => {
        const score = scoreSchemaMatch(s, data, rootSchema);
        if (score > maxScore) {
          maxScore = score;
          activeIndex = idx;
        }
      });
    }

    if (data === undefined) {
      const merged = sharedProps
        ? mergeSchemas({ type: 'object', properties: sharedProps, required: sharedRequired }, schema.anyOf[activeIndex])
        : schema.anyOf[activeIndex];
      // A copy: the value becomes editable data, and the schema's own
      // default object must never be changed through it.
      const defaultVal = copyDefault(getDefaultValue(merged, rootSchema));
      if (this._shouldInitialize(defaultVal)) {
        this._defer(() => onChange(this._autoWrite([], defaultVal, undefined, 'init')));
      }
    }

    const onSelectChange = (e: Event) => {
      const idx = parseInt((e.target as HTMLSelectElement).value, 10);
      const optSchema = schema.anyOf[idx];
      const variantDefault = copyDefault(getDefaultValue(optSchema, rootSchema));
      if (sharedProps && data && typeof data === 'object' && typeof variantDefault === 'object') {
        const preserved: any = { ...variantDefault };
        for (const key of Object.keys(sharedProps)) {
          if (ownValue(data, key) !== undefined) setOwn(preserved, key, data[key]);
        }
        onChange(preserved);
      } else {
        onChange(variantDefault);
      }
    };

    const activeVariant = schema.anyOf[activeIndex];
    const renderSchema = sharedProps
      ? mergeSchemas({ type: schema.type || 'object', properties: sharedProps, required: sharedRequired }, activeVariant)
      : activeVariant;

    return html`
      <div class="schema-nesting space-y-2 border-l-4 border-green-100 pl-4 py-2 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}
        ${schema.description ? html`<p class="text-xs text-gray-500 mb-2">${schema.description}</p>` : nothing}
        <select class="block w-full pl-3 pr-10 py-2 text-base border-gray-300 focus:outline-hidden focus:ring-green-500 focus:border-green-500 sm:text-sm rounded-md mb-2"
          aria-label=${schema.title ? `Select variant for ${schema.title}` : 'Select variant'}
          @change=${this._act(onSelectChange)}>
          ${schema.anyOf.map((opt: JSONSchema, idx: number) => {
            const o = withTypedKeywords(opt);
            let typeLabel = o.type;
            if (Array.isArray(o.type)) typeLabel = o.type.join('/');
            if (o.$ref) typeLabel = 'ref';
            return html`<option value=${idx} ?selected=${idx === activeIndex}>
              ${o.title || o.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`}
            </option>`;
          })}
        </select>
        <div>
          ${this._renderField(renderSchema, data, (val: any) => {
            onChange(val);
          }, rootSchema)}
        </div>
      </div>
    `;
  }

  // ─── if/then/else ───────────────────────────────────────────────────────

  private _renderConditional(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema): TemplateResult {
    const baseSchema: JSONSchema = { ...schema };
    delete baseSchema.if;
    delete baseSchema.then;
    delete baseSchema.else;

    const conditionMet = evaluateCondition(schema.if, data, rootSchema);
    const activeBranch = conditionMet ? (schema.then || {}) : (schema.else || {});
    const inactiveBranch = conditionMet ? (schema.else || {}) : (schema.then || {});
    const applicable = mergeSchemas(baseSchema, activeBranch);

    // Strip stale keys from the inactive branch. First remove top-level
    // keys exclusive to the inactive branch, then recursively strip nested
    // stale keys under shared keys using stripStaleKeys on the merged schema.
    if (data && typeof data === 'object') {
      const before = JSON.stringify(data);
      // Strip a copy: data is the form's current value (or a part of it),
      // and stripping it in place changed that value without an emit, so
      // the write below compared equal and was never made.
      data = copyDefault(data);

      // Remove top-level keys exclusive to the inactive branch
      if (inactiveBranch.properties) {
        const baseKeys = new Set(Object.keys(baseSchema.properties || {}));
        const activeKeys = new Set(Object.keys(activeBranch.properties || {}));
        for (const key of Object.keys(inactiveBranch.properties)) {
          if (!baseKeys.has(key) && !activeKeys.has(key) && key in data) {
            delete data[key];
          }
        }
      }

      // Recursively strip nested stale keys under shared keys
      // (e.g., both branches have "spec" but with different nested properties)
      stripStaleKeys(data, applicable, rootSchema);

      if (JSON.stringify(data) !== before) {
        const stripped = { ...data };
        this._defer(() => onChange(this._autoWrite([], stripped, JSON.parse(before), 'init')));
      }
    }

    return html`<div>${this._renderField(applicable, data, onChange, rootSchema)}</div>`;
  }

  // ─── enum ───────────────────────────────────────────────────────────────

  private _renderEnum(schema: JSONSchema, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    return this._renderChoices(schema, schema.enum, schema.enum.map(optionText), data, onChange, fieldId, describedBy, isRequired);
  }

  // ─── labeled enum (oneOf + const + title) ──────────────────────────────

  private _renderLabeledEnum(schema: JSONSchema, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    const entries = getLabeledEnumEntries(schema);
    return this._renderChoices(schema, entries.map(e => e.value), entries.map(e => e.label), data, onChange, fieldId, describedBy, isRequired);
  }

  /**
   * A <select> over enum choices. Each option is keyed by its position
   * (choiceKey), never by its text, so two choices that print alike, such as
   * {"a":1} and the string '{"a":1}', stay distinct; labels are display only.
   */
  private _renderChoices(schema: JSONSchema, choices: any[], labels: any[], data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    const selected = findChoice(choices, data);
    const isNull = data === null || data === undefined;
    const showCurrent = !isNull && selected < 0;

    const onSelectChange = (e: Event) => {
      const key = (e.target as HTMLSelectElement).value;
      const index = choiceIndex(key, choices.length);
      if (index >= 0) {
        onChange(copyDefault(choices[index]));
      } else if (key === CURRENT_CHOICE_KEY) {
        onChange(data);
      } else if (schema.type === 'integer' || schema.type === 'number') {
        onChange(parseFloat(key));
      } else {
        onChange(key);
      }
    };

    return html`
      <select class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
        id=${fieldId || nothing}
        aria-label=${schema.title ? `Select ${schema.title}` : 'Select value'}
        aria-describedby=${describedBy || nothing}
        ?required=${!!isRequired}
        aria-required=${isRequired ? 'true' : nothing}
        @change=${this._act(onSelectChange)}>
        ${isNull ? html`<option value="" selected>-- select --</option>` : nothing}
        ${choices.map((_, index) => html`
          <option value=${choiceKey(index)} ?selected=${index === selected}>${labels[index]}</option>
        `)}
        ${showCurrent ? html`
          <option value=${CURRENT_CHOICE_KEY} selected>${optionText(data)} (current)</option>
        ` : nothing}
      </select>
    `;
  }

  // ─── null ───────────────────────────────────────────────────────────────

  private _renderNull(schema: JSONSchema, onChange: (val: any) => void): TemplateResult {
    const canInitialize = Array.isArray(schema.type) && schema.type.length > 1;

    const onInit = () => {
      const nextType = schema.type.find((t: string) => t !== 'null');
      let newVal: any;
      if (nextType === 'object') newVal = {};
      else if (nextType === 'array') newVal = [];
      else if (nextType === 'boolean') newVal = false;
      else if (nextType === 'number' || nextType === 'integer') newVal = 0;
      else newVal = '';
      onChange(newVal);
    };

    return html`
      <div class="mt-1 flex items-center text-sm text-gray-500 italic">
        <span>null</span>
        ${canInitialize ? html`
          <button type="button"
            class="ml-2 text-xs text-indigo-600 hover:text-indigo-800 underline"
            @click=${this._act(onInit)}>Initialize</button>
        ` : nothing}
      </div>
    `;
  }

  // ─── object ─────────────────────────────────────────────────────────────

  private _renderObject(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, parentPath?: string, parentRequired?: boolean, isRequired?: boolean): TemplateResult {
    // For optional nested objects whose data is undefined, show an
    // "Initialize" button instead of auto-creating {}. This prevents
    // submitting invalid partial payloads like { address: {} } when the
    // address schema requires city/state but the parent doesn't require address.
    // Root-level objects (no parentPath) and required objects always auto-create.
    const isRoot = !parentPath;
    if (data === undefined && !isRoot && !isRequired) {
      return html`
        <div class="mt-1 flex items-center text-sm text-gray-500 italic">
          <span>Not set</span>
          <button type="button"
            class="ml-2 text-xs text-indigo-600 hover:text-indigo-800 underline"
            @click=${this._act(() => { onChange({}); })}>Initialize</button>
        </div>
      `;
    }

    if (typeof data !== 'object' || data === null || Array.isArray(data)) {
      const seen = data;
      data = {};
      this._defer(() => onChange(this._autoWrite([], {}, seen, 'init')));
    }

    // Pre-populate undefined REQUIRED boolean properties with false (or the schema default).
    // A checkbox that the user never touches will never fire onChange, so without
    // this the hidden input would submit {} instead of {"active": false}.
    // Optional booleans stay undefined — only submitted if the user interacts.
    if (schema.properties) {
      const requiredSet = new Set<string>(schema.required || []);
      // One write per field, each setting only its own key.
      const writes: AutoWrite[] = [];
      let needsUpdate = false;
      for (const [key, propSchema] of Object.entries(schema.properties)) {
        const ps = propSchema as JSONSchema;
        const isBooleanType = ps.type === 'boolean' ||
          (Array.isArray(ps.type) && ps.type.includes('boolean'));
        if (ownValue(data, key) === undefined && isBooleanType && requiredSet.has(key)) {
          const initial = ps.default !== undefined ? copyDefault(ps.default) : false;
          data = { ...data, [key]: initial };
          writes.push(this._autoWrite([key], initial, undefined, 'init'));
          needsUpdate = true;
        }
      }
      if (needsUpdate) {
        for (const write of writes) this._defer(() => onChange(write));
      }
    }

    // parentRequired defaults to true for root-level objects
    const effectiveParentRequired = parentRequired !== undefined ? parentRequired : true;
    const requiredFields = new Set<string>(schema.required || []);
    const knownKeys = new Set<string>(schema.properties ? Object.keys(schema.properties) : []);
    const extraKeys = Object.keys(data).filter(k => !knownKeys.has(k));

    return html`
      <div class="schema-nesting space-y-4 border-l-2 border-gray-200 pl-4 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}
        ${schema.description ? html`<p class="text-xs text-gray-500 mb-2">${schema.description}</p>` : nothing}

        ${schema.properties ? Object.entries(schema.properties).map(([key, propSchemaIn]: [string, any]) => {
          const propSchema = withTypedKeywords(propSchemaIn);
          const fullPath = parentPath ? `${parentPath}.${key}` : key;
          const fieldId = generateFieldId('field', fullPath);
          // A field's HTML required attribute should only apply when the entire
          // ancestor chain is required. If the parent object itself is optional,
          // nested required fields should not block form submission.
          const isRequired = requiredFields.has(key) && effectiveParentRequired;
          // Once an optional object has data (user clicked Initialize or is
          // editing existing data), its own required array should be enforced
          // on its children. childParentRequired propagates to nested objects
          // so their required constraints kick in when data exists.
          const propData = ownValue(data, key);
          const hasData = propData !== undefined && propData !== null;
          const childParentRequired = isRequired || hasData;
          // Each property starts a fresh reference path: descending into a
          // property is progress, and the form offers Initialize for an absent
          // optional object instead of expanding it, so a recursive schema
          // stops where the data does, as at base. A reference repeated at
          // the same position (no property in between) is still a cycle.
          const renderChild = () => this._renderFieldWithAttributes(propSchema, propData, childChange(onChange, key, (val: any) => ({ ...data, [key]: val })), rootSchema, fieldId, propSchema.description ? `${fieldId}-desc` : null, isRequired, fullPath, childParentRequired);

          return html`
            <div>
              <label class="block text-sm font-medium text-gray-700" for=${fieldId}>
                ${propSchema.title || key}${isRequired ? html`<span class="text-red-500 ml-1" aria-hidden="true">*</span>` : nothing}
              </label>
              ${propSchema.description && propSchema.type !== 'object'
                ? html`<p class="text-xs text-gray-500" id="${fieldId}-desc">${propSchema.description}</p>`
                : nothing}
              <div>
                ${onFreshRefPath(renderChild)}
              </div>
            </div>
          `;
        }) : nothing}

        ${schema.additionalProperties !== false ? this._renderAdditionalProperties(data, extraKeys, onChange, rootSchema) : nothing}
      </div>
    `;
  }

  /**
   * Render a field and pass id/aria-describedby/required directly to leaf
   * input renderers. Container types (object, array, composition) do NOT
   * receive these attributes — applying them to the first descendant input
   * of a nested sub-form is incorrect.
   */
  private _renderFieldWithAttributes(
    schema: JSONSchema,
    data: any,
    onChange: (val: any) => void,
    rootSchema: JSONSchema,
    fieldId: string,
    describedBy: string | null,
    required: boolean,
    parentPath?: string,
    childParentRequired?: boolean,
  ): TemplateResult {
    if (isLeafSchema(schema, rootSchema)) {
      // Leaf fields: thread attributes directly into the input renderer
      return this._renderField(schema, data, onChange, rootSchema, fieldId, parentPath, describedBy, required) as TemplateResult;
    }
    // Container fields: render without id/required/aria-describedby on any child input.
    // Pass childParentRequired (which accounts for data existence on optional objects)
    // so nested objects enforce their required constraints when data has been provided.
    return this._renderField(schema, data, onChange, rootSchema, undefined, parentPath, undefined, undefined, childParentRequired !== undefined ? childParentRequired : required) as TemplateResult;
  }

  // ─── additional properties ──────────────────────────────────────────────

  private _renderAdditionalProperties(data: any, extraKeys: string[], onChange: (val: any) => void, rootSchema: JSONSchema): TemplateResult {
    const onAddField = () => {
      let newKey = 'newField';
      let counter = 1;
      while (data[newKey] !== undefined) {
        newKey = `newField${counter++}`;
      }
      onChange({ ...data, [newKey]: '' });
    };

    return html`
      <div>
        ${extraKeys.length > 0 ? html`
          <div class="relative py-2">
            <div class="absolute inset-0 flex items-center" aria-hidden="true">
              <div class="w-full border-t border-gray-300"></div>
            </div>
            <div class="relative flex justify-start">
              <span class="pr-2 bg-gray-50 text-xs text-gray-500">Additional Properties</span>
            </div>
          </div>
        ` : nothing}

        ${extraKeys.map(key => this._renderExtraProperty(key, data, onChange, rootSchema))}

        <button type="button"
          aria-label="Add new custom field"
          class="mt-2 inline-flex items-center px-2.5 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
          @click=${this._act(onAddField)}>Add Field</button>
      </div>
    `;
  }

  private _renderExtraProperty(key: string, data: any, onChange: (val: any) => void, rootSchema: JSONSchema): TemplateResult {
    const propData = data[key];
    const errorId = generateFieldId('key-error', key);

    const onKeyChange = (e: Event) => {
      const input = e.target as HTMLInputElement;
      const newKey = input.value;
      if (newKey && newKey !== key) {
        if (data[newKey] !== undefined) {
          // Show inline error instead of alert
          input.classList.add('border-red-500');
          input.classList.remove('border-gray-300');
          input.setAttribute('aria-invalid', 'true');
          const errorEl = input.parentElement?.querySelector(`#${errorId}`);
          if (errorEl) {
            errorEl.textContent = 'Key already exists';
            setTimeout(this._act(() => {
              errorEl.textContent = '';
              input.classList.remove('border-red-500');
              input.classList.add('border-gray-300');
              input.removeAttribute('aria-invalid');
            }), 3000);
          }
          input.value = key;
          return;
        }
        const { [key]: val, ...rest } = data;
        onChange({ ...rest, [newKey]: val });
      }
    };

    const onKeyInput = (e: Event) => {
      // Clear error on next change
      const input = e.target as HTMLInputElement;
      const errorEl = input.parentElement?.querySelector(`#${errorId}`);
      if (errorEl && errorEl.textContent) {
        errorEl.textContent = '';
        input.classList.remove('border-red-500');
        input.classList.add('border-gray-300');
        input.removeAttribute('aria-invalid');
      }
    };

    const onRemove = () => {
      const { [key]: _, ...rest } = data;
      onChange(rest);
    };

    return html`
      <div class="schema-extra-prop flex gap-2 items-start mb-2 bg-white p-2 rounded border border-gray-200 shadow-sm">
        <div class="schema-extra-key w-1/3">
          <input type="text" .value=${key}
            class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md"
            placeholder="Key"
            aria-label="Property name"
            aria-describedby=${errorId}
            @change=${this._act(onKeyChange)}
            @input=${this._act(onKeyInput)}>
          <span id=${errorId} class="block text-sm text-red-500 mt-1" role="alert"></span>
        </div>
        <div class="flex-grow">
          ${typeof propData === 'object' && propData !== null
            ? this._renderField(inferSchema(propData), propData, childChange(onChange, key, (val: any) => ({ ...data, [key]: val })), rootSchema)
            : this._renderExtraValueInput(key, propData, data, onChange)
          }
        </div>
        <button type="button" class="text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50 self-start mt-0.5"
          title="Remove field"
          aria-label="Remove field ${key}"
          @click=${this._act(onRemove)}>&times;</button>
      </div>
    `;
  }

  private _renderExtraValueInput(key: string, propData: any, data: any, onChange: (val: any) => void): TemplateResult {
    let displayVal: string;
    if (typeof propData === 'string') {
      try {
        const parsed = JSON.parse(propData);
        displayVal = typeof parsed !== 'string' ? JSON.stringify(propData) : propData;
      } catch {
        displayVal = propData;
      }
    } else {
      displayVal = propData === undefined ? '' : JSON.stringify(propData);
    }

    const onInput = (e: Event) => {
      onChange({ ...data, [key]: (e.target as HTMLInputElement).value });
    };

    const onBlur = (e: Event) => {
      const val = (e.target as HTMLInputElement).value;
      let finalVal: any = val;
      try { finalVal = JSON.parse(val); } catch { /* keep as string */ }
      if (data[key] !== finalVal) {
        onChange({ ...data, [key]: finalVal });
      }
    };

    return html`
      <input type="text" .value=${displayVal}
        class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md"
        aria-label="Value for ${key}"
        @input=${this._act(onInput)}
        @blur=${this._act(onBlur)}>
    `;
  }

  // ─── array ──────────────────────────────────────────────────────────────

  private _renderArray(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, parentPath?: string): TemplateResult {
    if (!Array.isArray(data)) {
      const seen = data;
      data = [];
      this._defer(() => onChange(this._autoWrite([], [], seen, 'init')));
    }

    const hasMinItems = schema.minItems !== undefined;
    const hasMaxItems = schema.maxItems !== undefined;
    const canRemove = !hasMinItems || data.length > schema.minItems;
    const canAdd = !hasMaxItems || data.length < schema.maxItems;

    let countText = `${data.length} item${data.length !== 1 ? 's' : ''}`;
    let countError = false;
    if (hasMinItems || hasMaxItems) {
      const min = schema.minItems || 0;
      const max = schema.maxItems !== undefined ? schema.maxItems : '\u221e';
      countText += ` (${min}-${max})`;
      if ((hasMinItems && data.length < schema.minItems) || (hasMaxItems && data.length > schema.maxItems)) {
        countError = true;
      }
    }

    const onAddItem = () => {
      if (!canAdd) return;
      onChange([...data, copyDefault(getDefaultValue(schema.items || { type: 'string' }, rootSchema))]);
    };

    return html`
      <div class="schema-nesting space-y-2 border-l-2 border-indigo-200 pl-4 py-2 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}

        <div class="space-y-2">
          ${data.map((item: any, index: number) => {
            const itemPath = parentPath ? `${parentPath}.${index}` : `${index}`;
            return html`
            <div class="flex gap-2 items-start">
              <div class="flex-grow">
                ${onFreshRefPath(() => this._renderField(schema.items || inferSchema(item), item, childChange(onChange, index, (val: any) => {
                  const updated = [...data];
                  updated[index] = val;
                  return updated;
                }), rootSchema, undefined, itemPath))}
              </div>
              <button type="button"
                title="Remove item"
                aria-label="Remove item ${index + 1}"
                class=${canRemove
                  ? 'text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50'
                  : 'text-gray-400 font-bold px-2 py-1 border rounded cursor-not-allowed opacity-50'}
                ?disabled=${!canRemove}
                @click=${this._act(() => {
                  if (!canRemove) return;
                  onChange(data.filter((_: any, i: number) => i !== index));
                })}>&times;</button>
            </div>
          `; })}
        </div>

        <div class="flex items-center gap-3">
          <button type="button"
            aria-label="Add item to ${schema.title || 'list'}"
            class=${canAdd
              ? 'mt-2 inline-flex items-center px-2.5 py-1.5 border border-transparent text-xs font-medium rounded text-indigo-700 bg-indigo-100 hover:bg-indigo-200'
              : 'mt-2 inline-flex items-center px-2.5 py-1.5 border border-transparent text-xs font-medium rounded text-gray-400 bg-gray-100 cursor-not-allowed opacity-50'}
            ?disabled=${!canAdd}
            @click=${this._act(onAddItem)}>Add Item</button>
          ${hasMinItems || hasMaxItems ? html`
            <span class=${countError ? 'text-xs text-red-500' : 'text-xs text-gray-500'}>${countText}</span>
          ` : nothing}
        </div>
      </div>
    `;
  }

  // ─── primitives ─────────────────────────────────────────────────────────

  private _renderPrimitive(schema: JSONSchema, type: string, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    if (type === 'boolean') {
      // Never apply HTML required to checkboxes — a required checkbox is only
      // valid when checked, but a boolean field always has a value (true/false).
      // JSON Schema "required" means "must be present", not "must be true".
      return html`
        <input type="checkbox"
          id=${fieldId || nothing}
          class="focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded mt-1"
          aria-describedby=${describedBy || nothing}
          .checked=${!!data}
          @change=${this._act((e: Event) => onChange((e.target as HTMLInputElement).checked))}>
      `;
    }

    if (type === 'integer' || type === 'number') {
      return this._renderNumberInput(schema, type, data, onChange, fieldId, describedBy, isRequired);
    }

    // String type
    return this._renderStringInput(schema, data, onChange, fieldId, describedBy, isRequired);
  }

  private _renderNumberInput(schema: JSONSchema, type: string, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    const errorId = fieldId ? `${fieldId}-error` : undefined;
    const constraints: string[] = [];
    if (schema.minimum !== undefined || schema.exclusiveMinimum !== undefined) {
      constraints.push(schema.exclusiveMinimum !== undefined ? `>${schema.exclusiveMinimum}` : `\u2265${schema.minimum}`);
    }
    if (schema.maximum !== undefined || schema.exclusiveMaximum !== undefined) {
      constraints.push(schema.exclusiveMaximum !== undefined ? `<${schema.exclusiveMaximum}` : `\u2264${schema.maximum}`);
    }

    const onInput = (e: Event) => {
      const val = (e.target as HTMLInputElement).value;
      if (val === '') {
        if (Array.isArray(schema.type) && schema.type.includes('null')) onChange(null);
        else onChange(undefined);
      } else {
        onChange(type === 'integer' ? parseInt(val, 10) : parseFloat(val));
      }
    };

    const onBlur = (e: Event) => {
      const input = e.target as HTMLInputElement;
      const val = input.value;
      if (!fieldId) return;
      let error = '';
      if (isRequired && (val === '' || val === undefined || val === null)) {
        error = 'This field is required';
      } else if (val === '' || val === undefined || val === null) {
        // Empty and not required — clear any previous error
        this._clearError(fieldId);
        return;
      } else {
        const num = parseFloat(val);
        if (isNaN(num)) {
          error = `Invalid ${type} value`;
        } else {
          if (schema.exclusiveMinimum !== undefined && num <= schema.exclusiveMinimum) {
            error = `Must be greater than ${schema.exclusiveMinimum}`;
          } else if (schema.exclusiveMaximum !== undefined && num >= schema.exclusiveMaximum) {
            error = `Must be less than ${schema.exclusiveMaximum}`;
          } else if (schema.minimum !== undefined && num < schema.minimum) {
            error = `Must be at least ${schema.minimum}`;
          } else if (schema.maximum !== undefined && num > schema.maximum) {
            error = `Must be at most ${schema.maximum}`;
          } else if (type === 'integer' && !Number.isInteger(num)) {
            error = 'Must be a whole number';
          } else if (schema.multipleOf !== undefined && !isMultipleOf(num, schema.multipleOf)) {
            error = `Must be a multiple of ${schema.multipleOf}`;
          }
        }
      }
      if (error) {
        this._setError(fieldId, error);
      } else {
        this._clearError(fieldId);
      }
    };

    const currentError = fieldId ? (this._errors.get(fieldId) || '') : '';
    const ariaDescParts = [describedBy, errorId].filter(Boolean).join(' ');

    return html`
      <div>
        <input type="number"
          id=${fieldId || nothing}
          step=${type === 'integer' ? '1' : 'any'}
          min=${schema.minimum !== undefined ? schema.minimum : nothing}
          max=${schema.maximum !== undefined ? schema.maximum : nothing}
          .value=${formatNumericForInput(data, schema)}
          class=${`shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm rounded-md mt-1 ${currentError ? 'border-red-500' : 'border-gray-300'}`}
          aria-describedby=${ariaDescParts || nothing}
          aria-invalid=${currentError ? 'true' : nothing}
          ?required=${!!isRequired}
          aria-required=${isRequired ? 'true' : nothing}
          @input=${this._act(onInput)}
          @blur=${this._act(onBlur)}>
        ${constraints.length > 0 ? html`<span class="text-xs text-gray-400 block mt-1">${constraints.join(', ')}</span>` : nothing}
        <span class="schema-form-error block text-sm text-red-500 mt-1" id=${errorId || nothing} role="alert">${currentError}</span>
      </div>
    `;
  }

  private _renderStringInput(schema: JSONSchema, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    const errorId = fieldId ? `${fieldId}-error` : undefined;
    let inputType = 'text';
    if (schema.format === 'date') inputType = 'date';
    else if (schema.format === 'date-time') inputType = 'datetime-local';
    else if (schema.format === 'email') inputType = 'email';
    else if (schema.format === 'uri' || schema.format === 'url') inputType = 'url';

    let hintText = '';
    if (schema.minLength !== undefined && schema.maxLength !== undefined) {
      hintText = `${schema.minLength}-${schema.maxLength} characters`;
    } else if (schema.minLength !== undefined) {
      hintText = `Min ${schema.minLength} characters`;
    } else if (schema.maxLength !== undefined) {
      hintText = `Max ${schema.maxLength} characters`;
    }

    const onInput = (e: Event) => {
      onChange((e.target as HTMLInputElement).value);
    };

    const onBlur = (e: Event) => {
      const input = e.target as HTMLInputElement;
      const val = input.value || '';
      if (!fieldId) return;
      let error = '';
      // Check required manually (no native `required` attribute to avoid blocking form submit)
      if (isRequired && val === '') {
        error = 'This field is required';
      } else if (schema.minLength !== undefined && val.length > 0 && val.length < schema.minLength) {
        error = `Must be at least ${schema.minLength} characters`;
      } else if (schema.maxLength !== undefined && val.length > schema.maxLength) {
        error = `Must be at most ${schema.maxLength} characters`;
      } else if (val !== '' && compilePattern(schema.pattern)?.test(val) === false) {
        error = (schema as any).patternDescription
          || `Must match the expected format (${schema.pattern})`;
      } else if (input.validity.typeMismatch) {
        error = `Invalid ${inputType} value`;
      }
      if (error) {
        this._setError(fieldId, error);
      } else {
        this._clearError(fieldId);
      }
    };

    const currentError = fieldId ? (this._errors.get(fieldId) || '') : '';
    const ariaDescParts = [describedBy, errorId].filter(Boolean).join(' ');

    return html`
      <div>
        <input type=${inputType}
          id=${fieldId || nothing}
          .value=${data !== null && typeof data === 'object' ? valueText(data) : data || ''}
          pattern=${schema.pattern || nothing}
          minlength=${schema.minLength !== undefined ? schema.minLength : nothing}
          maxlength=${schema.maxLength !== undefined ? schema.maxLength : nothing}
          class=${`shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm rounded-md mt-1 ${currentError ? 'border-red-500' : 'border-gray-300'}`}
          aria-describedby=${ariaDescParts || nothing}
          aria-invalid=${currentError ? 'true' : nothing}
          ?required=${!!isRequired}
          aria-required=${isRequired ? 'true' : nothing}
          @input=${this._act(onInput)}
          @blur=${this._act(onBlur)}>
        ${hintText ? html`<span class="text-xs text-gray-400 block mt-1">${hintText}</span>` : nothing}
        <span class="schema-form-error block text-sm text-red-500 mt-1" id=${errorId || nothing} role="alert">${currentError}</span>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-form-mode': SchemaFormMode;
  }
}

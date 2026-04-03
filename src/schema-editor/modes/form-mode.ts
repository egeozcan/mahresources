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
} from '../schema-core';
import { isLeafSchema, stripStaleKeys } from '../form-mode-helpers';

function generateFieldId(prefix: string, path: string): string {
  // Injective encoding: every non-alphanumeric character is replaced with
  // _XX (two-digit hex of its char code).  This guarantees distinct paths
  // always produce distinct IDs — no two different inputs can collide.
  const encoded = path.replace(/[^a-zA-Z0-9]/g, (ch) => {
    return '_' + ch.charCodeAt(0).toString(16).padStart(2, '0');
  });
  return `${prefix}-${encoded}`;
}

@customElement('schema-form-mode')
export class SchemaFormMode extends LitElement {
  @property({ type: Object }) schema: JSONSchema = {};
  @property({ type: Object }) value: any = {};
  @property({ type: String }) name = 'Meta';

  @state() private _data: any = {};

  // Light DOM hidden input for form submission
  private _hiddenInput: HTMLInputElement | null = null;

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
  }

  override disconnectedCallback() {
    if (this._hiddenInput && this._hiddenInput.parentNode === this) {
      this.removeChild(this._hiddenInput);
    }
    this._hiddenInput = null;
    super.disconnectedCallback();
  }

  override willUpdate(changed: Map<string, unknown>) {
    // When Alpine binds with :schema="currentSchema", the value may arrive as
    // a JSON string (from the API) rather than a parsed object.  Handle both.
    if (changed.has('schema') && typeof this.schema === 'string') {
      try {
        this.schema = JSON.parse(this.schema as unknown as string);
      } catch {
        this.schema = {};
      }
    }
    if (changed.has('value') || changed.has('schema')) {
      this._data = this.value != null ? (typeof this.value === 'string' ? this._safeParse(this.value) : structuredClone(this.value)) : {};

      // When the schema changes, recursively strip keys from _data that aren't
      // in the new schema's properties and aren't allowed by
      // additionalProperties.  This prevents stale keys from a previous schema
      // (e.g. after a category switch) from being silently submitted via the
      // hidden input — including nested objects with additionalProperties:false.
      if (changed.has('schema') && this._data && typeof this._data === 'object') {
        const before = JSON.stringify(this._data);
        stripStaleKeys(this._data, this.schema, this.schema);
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
          this.updateComplete.then(() => {
            this.dispatchEvent(new CustomEvent('value-change', {
              detail: { value: this._data },
              bubbles: true,
              composed: true,
            }));
          });
        }
      }

      // Keep hidden input in sync when value/schema change externally
      // (e.g. when Alpine passes currentMeta with pre-existing edits).
      if (this._hiddenInput) {
        this._hiddenInput.value = JSON.stringify(this._data);
      }
    }
  }

  private _safeParse(s: string): any {
    try { return JSON.parse(s); } catch { return {}; }
  }

  private _emitChange(newValue: any) {
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

  // ─── Render ──────────────────────────────────────────────────────────────

  override render() {
    if (!this.schema || Object.keys(this.schema).length === 0) {
      return nothing;
    }

    return this._renderField(this.schema, this._data, (val: any) => {
      this._emitChange(val);
    }, this.schema, undefined, '');
  }

  // ─── Recursive field renderer (port of generateFormElement) ──────────────

  private _renderField(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, fieldId?: string, parentPath?: string, describedBy?: string | null, isRequired?: boolean, parentRequired?: boolean): TemplateResult | typeof nothing {
    // Handle $ref
    if (schema.$ref) {
      const resolved = resolveRef(schema.$ref, rootSchema);
      if (resolved) {
        const mergedSchema = { ...resolved, ...schema };
        delete mergedSchema.$ref;
        return this._renderField(mergedSchema, data, onChange, rootSchema, fieldId, parentPath, describedBy, isRequired, parentRequired);
      }
      return html`<div class="text-red-500 text-xs">Unresolvable reference: ${schema.$ref}</div>`;
    }

    // Handle oneOf
    if (schema.oneOf && Array.isArray(schema.oneOf)) {
      return this._renderOneOf(schema, data, onChange, rootSchema);
    }

    // Handle allOf - merge all schemas
    if (schema.allOf && Array.isArray(schema.allOf)) {
      let merged: JSONSchema = { ...schema };
      delete merged.allOf;
      for (const sub of schema.allOf) {
        let resolved: JSONSchema;
        if (sub.$ref) {
          const refResult = resolveRef(sub.$ref, rootSchema);
          const siblings: JSONSchema = { ...sub };
          delete siblings.$ref;
          resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
        } else {
          resolved = sub;
        }
        if (resolved) merged = mergeSchemas(merged, resolved);
      }
      return this._renderField(merged, data, onChange, rootSchema, fieldId, parentPath, describedBy, isRequired, parentRequired);
    }

    // Handle anyOf
    if (schema.anyOf && Array.isArray(schema.anyOf)) {
      return this._renderAnyOf(schema, data, onChange, rootSchema);
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
      if (data !== schema.const) {
        onChange(schema.const);
      }
      return html`<input type="text" .value=${String(schema.const)} disabled
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

  private _renderOneOf(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema): TemplateResult {
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
      const defaultVal = getDefaultValue(schema.oneOf[activeIndex], rootSchema);
      // Defer onChange to avoid triggering during render
      queueMicrotask(() => onChange(defaultVal));
    }

    const onSelectChange = (e: Event) => {
      const idx = parseInt((e.target as HTMLSelectElement).value, 10);
      const optSchema = schema.oneOf[idx];
      const newVal = getDefaultValue(optSchema, rootSchema);
      onChange(newVal);
    };

    return html`
      <div class="space-y-2 border-l-4 border-indigo-100 pl-4 py-2 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}
        ${schema.description ? html`<p class="text-xs text-gray-500 mb-2">${schema.description}</p>` : nothing}
        <select class="block w-full pl-3 pr-10 py-2 text-base border-gray-300 focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm rounded-md mb-2"
          aria-label=${schema.title ? `Select variant for ${schema.title}` : 'Select variant'}
          @change=${onSelectChange}>
          ${schema.oneOf.map((opt: JSONSchema, idx: number) => {
            let typeLabel = opt.type;
            if (Array.isArray(opt.type)) typeLabel = opt.type.join('/');
            if (opt.$ref) typeLabel = 'ref';
            return html`<option value=${idx} ?selected=${idx === activeIndex}>
              ${opt.title || opt.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`}
            </option>`;
          })}
        </select>
        <div>
          ${this._renderField(schema.oneOf[activeIndex], data, (val: any) => {
            onChange(val);
          }, rootSchema)}
        </div>
      </div>
    `;
  }

  // ─── anyOf ──────────────────────────────────────────────────────────────

  private _renderAnyOf(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema): TemplateResult {
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
      const defaultVal = getDefaultValue(schema.anyOf[activeIndex], rootSchema);
      queueMicrotask(() => onChange(defaultVal));
    }

    const onSelectChange = (e: Event) => {
      const idx = parseInt((e.target as HTMLSelectElement).value, 10);
      const optSchema = schema.anyOf[idx];
      const newVal = getDefaultValue(optSchema, rootSchema);
      onChange(newVal);
    };

    return html`
      <div class="space-y-2 border-l-4 border-green-100 pl-4 py-2 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}
        ${schema.description ? html`<p class="text-xs text-gray-500 mb-2">${schema.description}</p>` : nothing}
        <select class="block w-full pl-3 pr-10 py-2 text-base border-gray-300 focus:outline-none focus:ring-green-500 focus:border-green-500 sm:text-sm rounded-md mb-2"
          aria-label=${schema.title ? `Select variant for ${schema.title}` : 'Select variant'}
          @change=${onSelectChange}>
          ${schema.anyOf.map((opt: JSONSchema, idx: number) => {
            let typeLabel = opt.type;
            if (Array.isArray(opt.type)) typeLabel = opt.type.join('/');
            if (opt.$ref) typeLabel = 'ref';
            return html`<option value=${idx} ?selected=${idx === activeIndex}>
              ${opt.title || opt.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`}
            </option>`;
          })}
        </select>
        <div>
          ${this._renderField(schema.anyOf[activeIndex], data, (val: any) => {
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

    const conditionMet = evaluateCondition(schema.if, data);
    const activeBranch = conditionMet ? (schema.then || {}) : (schema.else || {});
    const inactiveBranch = conditionMet ? (schema.else || {}) : (schema.then || {});
    const applicable = mergeSchemas(baseSchema, activeBranch);

    // Strip stale keys from the inactive branch. First remove top-level
    // keys exclusive to the inactive branch, then recursively strip nested
    // stale keys under shared keys using stripStaleKeys on the merged schema.
    if (data && typeof data === 'object') {
      const before = JSON.stringify(data);

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
        queueMicrotask(() => onChange({ ...data }));
      }
    }

    return html`<div>${this._renderField(applicable, data, onChange, rootSchema)}</div>`;
  }

  // ─── enum ───────────────────────────────────────────────────────────────

  private _renderEnum(schema: JSONSchema, data: any, onChange: (val: any) => void, fieldId?: string, describedBy?: string | null, isRequired?: boolean): TemplateResult {
    const hasValue = schema.enum.some((v: any) => v === data);
    const isNull = data === null || data === undefined;

    const onSelectChange = (e: Event) => {
      const valStr = (e.target as HTMLSelectElement).value;
      let val: any = valStr;
      const match = schema.enum.find((ev: any) => String(ev) === valStr);
      if (match !== undefined) val = match;

      if (match === undefined && (schema.type === 'integer' || schema.type === 'number')) {
        val = parseFloat(valStr);
      }
      onChange(val);
    };

    return html`
      <select class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
        id=${fieldId || nothing}
        aria-label=${schema.title ? `Select ${schema.title}` : 'Select value'}
        aria-describedby=${describedBy || nothing}
        ?required=${!!isRequired}
        aria-required=${isRequired ? 'true' : nothing}
        @change=${onSelectChange}>
        ${isNull ? html`<option value="" selected>-- select --</option>` : nothing}
        ${schema.enum.map((val: any) => html`
          <option value=${val} ?selected=${val === data}>${val}</option>
        `)}
        ${!isNull && !hasValue ? html`
          <option value=${data} selected>${data} (current)</option>
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
            @click=${onInit}>Initialize</button>
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
            @click=${() => { onChange({}); }}>Initialize</button>
        </div>
      `;
    }

    if (typeof data !== 'object' || data === null || Array.isArray(data)) {
      data = {};
      queueMicrotask(() => onChange(data));
    }

    // Pre-populate undefined REQUIRED boolean properties with false (or the schema default).
    // A checkbox that the user never touches will never fire onChange, so without
    // this the hidden input would submit {} instead of {"active": false}.
    // Optional booleans stay undefined — only submitted if the user interacts.
    if (schema.properties) {
      const requiredSet = new Set<string>(schema.required || []);
      let needsUpdate = false;
      for (const [key, propSchema] of Object.entries(schema.properties)) {
        const ps = propSchema as JSONSchema;
        const isBooleanType = ps.type === 'boolean' ||
          (Array.isArray(ps.type) && ps.type.includes('boolean'));
        if (data[key] === undefined && isBooleanType && requiredSet.has(key)) {
          data = { ...data, [key]: ps.default !== undefined ? ps.default : false };
          needsUpdate = true;
        }
      }
      if (needsUpdate) {
        queueMicrotask(() => onChange(data));
      }
    }

    // parentRequired defaults to true for root-level objects
    const effectiveParentRequired = parentRequired !== undefined ? parentRequired : true;
    const requiredFields = new Set<string>(schema.required || []);
    const knownKeys = new Set<string>(schema.properties ? Object.keys(schema.properties) : []);
    const extraKeys = Object.keys(data).filter(k => !knownKeys.has(k));

    return html`
      <div class="space-y-4 border-l-2 border-gray-200 pl-4 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}
        ${schema.description ? html`<p class="text-xs text-gray-500 mb-2">${schema.description}</p>` : nothing}

        ${schema.properties ? Object.entries(schema.properties).map(([key, propSchema]: [string, any]) => {
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
          const hasData = data[key] !== undefined && data[key] !== null;
          const childParentRequired = isRequired || hasData;

          return html`
            <div>
              <label class="block text-sm font-medium text-gray-700" for=${fieldId}>
                ${propSchema.title || key}${isRequired ? html`<span class="text-red-500 ml-1" aria-hidden="true">*</span>` : nothing}
              </label>
              ${propSchema.description && propSchema.type !== 'object'
                ? html`<p class="text-xs text-gray-500" id="${fieldId}-desc">${propSchema.description}</p>`
                : nothing}
              <div>
                ${this._renderFieldWithAttributes(propSchema, data[key], (val: any) => {
                  onChange({ ...data, [key]: val });
                }, rootSchema, fieldId, propSchema.description ? `${fieldId}-desc` : null, isRequired, fullPath, childParentRequired)}
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
          class="mt-2 inline-flex items-center px-2.5 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
          @click=${onAddField}>Add Field</button>
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
            setTimeout(() => {
              errorEl.textContent = '';
              input.classList.remove('border-red-500');
              input.classList.add('border-gray-300');
              input.removeAttribute('aria-invalid');
            }, 3000);
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
      <div class="flex gap-2 items-start mb-2 bg-white p-2 rounded border border-gray-200 shadow-sm">
        <div class="w-1/3">
          <input type="text" .value=${key}
            class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md"
            placeholder="Key"
            aria-label="Property name"
            aria-describedby=${errorId}
            @change=${onKeyChange}
            @input=${onKeyInput}>
          <span id=${errorId} class="block text-sm text-red-500 mt-1" role="alert"></span>
        </div>
        <div class="flex-grow">
          ${typeof propData === 'object' && propData !== null
            ? this._renderField(inferSchema(propData), propData, (val: any) => {
                onChange({ ...data, [key]: val });
              }, rootSchema)
            : this._renderExtraValueInput(key, propData, data, onChange)
          }
        </div>
        <button type="button" class="text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50 self-start mt-0.5"
          title="Remove field"
          aria-label="Remove field ${key}"
          @click=${onRemove}>&times;</button>
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
        @input=${onInput}
        @blur=${onBlur}>
    `;
  }

  // ─── array ──────────────────────────────────────────────────────────────

  private _renderArray(schema: JSONSchema, data: any, onChange: (val: any) => void, rootSchema: JSONSchema, parentPath?: string): TemplateResult {
    if (!Array.isArray(data)) {
      data = [];
      queueMicrotask(() => onChange(data));
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
      onChange([...data, getDefaultValue(schema.items || { type: 'string' }, rootSchema)]);
    };

    return html`
      <div class="space-y-2 border-l-2 border-indigo-200 pl-4 py-2 my-2">
        ${schema.title ? html`<h4 class="font-bold text-gray-900 text-sm">${schema.title}</h4>` : nothing}

        <div class="space-y-2">
          ${data.map((item: any, index: number) => {
            const itemPath = parentPath ? `${parentPath}.${index}` : `${index}`;
            return html`
            <div class="flex gap-2 items-start">
              <div class="flex-grow">
                ${this._renderField(schema.items || inferSchema(item), item, (val: any) => {
                  const updated = [...data];
                  updated[index] = val;
                  onChange(updated);
                }, rootSchema, undefined, itemPath)}
              </div>
              <button type="button"
                title="Remove item"
                aria-label="Remove item ${index + 1}"
                class=${canRemove
                  ? 'text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50'
                  : 'text-gray-400 font-bold px-2 py-1 border rounded cursor-not-allowed opacity-50'}
                ?disabled=${!canRemove}
                @click=${() => {
                  if (!canRemove) return;
                  onChange(data.filter((_: any, i: number) => i !== index));
                }}>&times;</button>
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
            @click=${onAddItem}>Add Item</button>
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
          @change=${(e: Event) => onChange((e.target as HTMLInputElement).checked)}>
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
      const errorSpan = errorId
        ? (input.closest('div')?.querySelector(`#${errorId}`) as HTMLElement | null)
        : (input.parentElement?.querySelector('.schema-form-error') as HTMLElement | null);
      if (val === '' || val === undefined || val === null) {
        if (errorSpan) errorSpan.textContent = '';
        input.classList.remove('border-red-500');
        input.classList.add('border-gray-300');
        input.removeAttribute('aria-invalid');
        return;
      }
      const num = parseFloat(val);
      let error = '';
      if (!isNaN(num)) {
        if (schema.exclusiveMinimum !== undefined && num <= schema.exclusiveMinimum) {
          error = `Must be greater than ${schema.exclusiveMinimum}`;
        } else if (schema.exclusiveMaximum !== undefined && num >= schema.exclusiveMaximum) {
          error = `Must be less than ${schema.exclusiveMaximum}`;
        } else if (schema.minimum !== undefined && num < schema.minimum) {
          error = `Must be at least ${schema.minimum}`;
        } else if (schema.maximum !== undefined && num > schema.maximum) {
          error = `Must be at most ${schema.maximum}`;
        }
      }
      if (errorSpan) {
        errorSpan.textContent = error;
      }
      if (error) {
        input.classList.add('border-red-500');
        input.classList.remove('border-gray-300');
        input.setAttribute('aria-invalid', 'true');
      } else {
        input.classList.remove('border-red-500');
        input.classList.add('border-gray-300');
        input.removeAttribute('aria-invalid');
      }
    };

    const ariaDescParts = [describedBy, errorId].filter(Boolean).join(' ');

    return html`
      <div>
        <input type="number"
          id=${fieldId || nothing}
          step=${type === 'integer' ? '1' : 'any'}
          min=${schema.minimum !== undefined ? schema.minimum : nothing}
          max=${schema.maximum !== undefined ? schema.maximum : nothing}
          .value=${data !== undefined && data !== null ? String(data) : ''}
          class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
          aria-describedby=${ariaDescParts || nothing}
          ?required=${!!isRequired}
          aria-required=${isRequired ? 'true' : nothing}
          @input=${onInput}
          @blur=${onBlur}>
        ${constraints.length > 0 ? html`<span class="text-xs text-gray-400 block mt-1">${constraints.join(', ')}</span>` : nothing}
        <span class="schema-form-error block text-sm text-red-500 mt-1" id=${errorId || nothing} role="alert"></span>
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
      const errorSpan = errorId
        ? (input.closest('div')?.querySelector(`#${errorId}`) as HTMLElement | null)
        : (input.parentElement?.querySelector('.schema-form-error') as HTMLElement | null);
      let error = '';
      if (schema.minLength !== undefined && val.length < schema.minLength) {
        error = `Must be at least ${schema.minLength} characters`;
      } else if (schema.maxLength !== undefined && val.length > schema.maxLength) {
        error = `Must be at most ${schema.maxLength} characters`;
      }
      if (errorSpan) {
        errorSpan.textContent = error;
      }
      if (error) {
        input.classList.add('border-red-500');
        input.classList.remove('border-gray-300');
        input.setAttribute('aria-invalid', 'true');
      } else {
        input.classList.remove('border-red-500');
        input.classList.add('border-gray-300');
        input.removeAttribute('aria-invalid');
      }
    };

    const ariaDescParts = [describedBy, errorId].filter(Boolean).join(' ');

    return html`
      <div>
        <input type=${inputType}
          id=${fieldId || nothing}
          .value=${data || ''}
          pattern=${schema.pattern || nothing}
          minlength=${schema.minLength !== undefined ? schema.minLength : nothing}
          maxlength=${schema.maxLength !== undefined ? schema.maxLength : nothing}
          class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1"
          aria-describedby=${ariaDescParts || nothing}
          ?required=${!!isRequired}
          aria-required=${isRequired ? 'true' : nothing}
          @input=${onInput}
          @blur=${onBlur}>
        ${hintText ? html`<span class="text-xs text-gray-400 block mt-1">${hintText}</span>` : nothing}
        <span class="schema-form-error block text-sm text-red-500 mt-1" id=${errorId || nothing} role="alert"></span>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-form-mode': SchemaFormMode;
  }
}

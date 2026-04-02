import { isUndef, isNumeric } from '../index.js';

export function freeFields({ fields, name, url, jsonOutput, id, title, fromJSON }) {
  return {
    fields,
    name,
    url,
    jsonOutput,
    id,
    title,
    fromJSON,
    remoteFields: [],
    jsonText: "",

    async init() {
      // Listen for schema fields claiming MetaQuery paths — must be registered
      // before any async work to avoid missing events from schemaSearchFields init().
      // Only search-sidebar freeFields (name="MetaQuery") should respond; bulk-edit
      // and create/edit forms (name="Meta", jsonOutput=true) must not be affected.
      this._removedBySchema = [];
      window.addEventListener('schema-fields-claimed', (e) => {
        if (this.name !== 'MetaQuery') return;
        const claimed = new Set(e.detail.paths || []);
        if (!this.fields) return;

        // Restore previously removed entries back into the current list
        if (this._removedBySchema.length > 0) {
          this.fields = this.fields.concat(this._removedBySchema);
          this._removedBySchema = [];
        }

        // Remove newly claimed entries from the current list
        if (claimed.size > 0) {
          const kept = [];
          for (const f of this.fields) {
            if (claimed.has(f.name)) {
              this._removedBySchema.push(f);
            } else {
              kept.push(f);
            }
          }
          this.fields = kept;
        }
      });

      if (this.jsonOutput) {
        window.Alpine.effect(() => {
          this.jsonText = JSON.stringify(
            Object.fromEntries(
              this.fields
                .filter((x) => x.name !== "")
                .map((field) => [
                  field.name,
                  getJSONOrObjValue(field.value),
                ])
            )
          );
          // Sync edits back to parent so currentMeta stays current
          // when switching between freeFields and schema-form-mode.
          try {
            this.$el.dispatchEvent(new CustomEvent('value-change', {
              detail: { value: JSON.parse(this.jsonText) },
              bubbles: true,
            }));
          } catch { /* ignore parse errors from empty/partial state */ }
        });
      }

      if (this.fields) {
        this.fields = this.fields.map((field) => {
          try {
            field.value = JSON.stringify(JSON.parse(field.value));
          } catch (e) {
            // no op
          }

          return field;
        });
      }

      // Prefer dynamically-passed currentMeta (from parent wrapper's data attribute)
      // over the static server-rendered fromJSON, so that edits made in
      // schema-form-mode are preserved when switching to freeFields.
      // Prefer dynamically-passed currentMeta over the static server-rendered
      // fromJSON.  An empty string means "no edits yet" (use fromJSON); a
      // non-empty string (including "{}") means the user has edited and this
      // value is intentional — even an empty object.
      const currentMetaEl = this.$el.closest('[data-current-meta]');
      const currentMetaAttr = currentMetaEl?.dataset.currentMeta;
      let initSource = this.fromJSON;
      if (currentMetaAttr) {
        try {
          const parsed = JSON.parse(currentMetaAttr);
          if (parsed && typeof parsed === 'object') {
            initSource = parsed;
          }
        } catch { /* fall through to fromJSON */ }
      }

      if (initSource) {
        try {
          this.fields = Object.entries(initSource).map((x) => ({
            name: x[0],
            value: JSON.stringify(x[1]),
          }));
        } catch (e) {
          console.error(e);
          // do not care, you get no prefill
        }
      }

      if (this.url) {
        try {
          this.remoteFields = await fetch(this.url).then((x) => x.json());
        } catch (err) {
          console.error('Failed to fetch remote fields:', err);
        }
      }

    },

    inputEvents: {},
  };
}

export function generateParamNameForMeta({ name, value, operation } = {}) {
  if (isUndef(name) || isUndef(value)) {
    return "";
  }

  const realValue = getJSONValue(value);
  const valueStr =
    typeof realValue === "string"
      ? `"${realValue}"`
      : realValue == null
      ? "null"
      : realValue.toString();

  if (!operation) {
    return `${name}:EQ:${valueStr}`;
  }

  return `${name}:${operation}:${valueStr}`;
}

/**
 * Get the JSON value for string
 *
 * @param {string} x
 * @returns {string|boolean|number|null}
 */
export function getJSONValue(x) {
  if (typeof x !== "string") {
    return x;
  }

  if (x.match(/^\d\d\d\d-\d\d?-\d\d?$/)) {
    const dateCast = new Date(x);

    if (!isNaN(dateCast.getFullYear())) {
      return dateCast.toISOString().split("T")[0];
    }
  }

  if (isNumeric(x)) {
    return parseFloat(x);
  }

  if (x === "true" || x === "false") {
    return x === "true";
  }

  if (typeof x === "string" && x.toLowerCase() === "null") {
    return null;
  }

  if (x.startsWith('"') && x.endsWith('"')) {
    return x.substring(1, x.length - 1);
  }

  return x;
}

/**
 * Get the JSON value for string
 *
 * @param {string} x
 * @returns {string|boolean|number|Object|null}
 */
export function getJSONOrObjValue(x) {
  if (x === "null") {
    return null;
  }

  const value = getJSONValue(x);

  if (typeof value !== "string") {
    return value;
  }

  try {
    return JSON.parse(x);
  } catch (e) {
    return value;
  }
}

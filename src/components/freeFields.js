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

      if (this.fromJSON) {
        try {
          this.fields = Object.entries(this.fromJSON).map((x) => ({
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

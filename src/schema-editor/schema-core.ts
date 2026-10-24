// ─── Type definitions ────────────────────────────────────────────────────────

export interface FlatField {
  path: string;
  label: string;
  type: string;
  enum: string[] | null;
  /** Optional labels for enum values (parallel array with `enum`). null if no labels. */
  enumLabels: string[] | null;
}

export type JSONSchema = Record<string, any>;

// ─── JSON Pointer escaping (RFC 6901) ────────────────────────────────────────

export function escapeJsonPointer(token: string): string {
  return token.replace(/~/g, '~0').replace(/\//g, '~1');
}

export function unescapeJsonPointer(token: string): string {
  return token.replace(/~1/g, '/').replace(/~0/g, '~');
}

// ─── Ref resolution ──────────────────────────────────────────────────────────

// ─── Own-property access ─────────────────────────────────────────────────────
// Schema and data keys are arbitrary strings. obj[key] reads inherited members
// ("constructor", "toString"), and assigning obj["__proto__"] sets the
// prototype instead of a property. Use these wherever a key comes from a
// schema or from stored data.

/** Whether value can be a subschema: an object or a boolean (true/false schemas). */
export function isSchemaValue(value: unknown): boolean {
  return typeof value === 'boolean' || (value !== null && typeof value === 'object' && !Array.isArray(value));
}

/** Whether key is obj's own property. */
export function hasOwn(obj: unknown, key: string): boolean {
  return obj != null && typeof obj === 'object' && Object.prototype.hasOwnProperty.call(obj, key);
}

/** obj[key] when key is obj's own property, else undefined. */
export function ownValue(obj: unknown, key: string): any {
  return hasOwn(obj, key) ? (obj as Record<string, unknown>)[key] : undefined;
}

/** Set key as an own data property of obj, "__proto__" included. */
export function setOwn(obj: object, key: string, value: unknown): void {
  Object.defineProperty(obj, key, { value, writable: true, enumerable: true, configurable: true });
}

export function resolveRef(ref: unknown, root: JSONSchema): JSONSchema | null {
  if (typeof ref !== 'string' || !ref.startsWith('#/')) return null;
  const parts = ref.split('/').slice(1);
  let current: any = root;
  for (const part of parts) {
    const unescaped = unescapeJsonPointer(part);
    if (hasOwn(current, unescaped)) {
      current = current[unescaped];
    } else {
      return null;
    }
  }
  return current;
}

// ─── Schema merging ──────────────────────────────────────────────────────────

export function mergeSchemas(base: JSONSchema, extension: JSONSchema, mode: 'intersect' | 'union' = 'intersect'): JSONSchema {
  countSchemaWork();
  const merged: JSONSchema = { ...base };
  for (const key in extension) {
    // A keyword value of a type no draft allows (an x-display of 7, a title
    // of {}) never erases the valid value the base already has.
    if (!keywordValueValid(key, extension[key])) continue;
    if (key === 'properties') {
      const baseProps = base.properties || {};
      const extProps = extension.properties || {};
      merged.properties = { ...baseProps };
      for (const propKey in extProps) {
        const baseProp = ownValue(baseProps, propKey);
        const extProp = extProps[propKey];
        if (baseProp && extProp) {

          // Deep merge when both sides have nested properties or required
          if ((baseProp.properties && extProp.properties) ||
              (baseProp.required && extProp.required)) {
            setOwn(merged.properties, propKey, mergeSchemas(baseProp, extProp, mode));
          } else {
            // Shallow merge with type/enum conflict resolution
            const baseType = baseProp.type;
            const extType = extProp.type;
            const numericTypes = new Set(['number', 'integer']);
            // The same filter as the top level: an invalid value in extProp
            // never erases a valid one in baseProp.
            const mergedProp = { ...baseProp, ...validKeywords(extProp) };
            // Resolve type conflicts
            if (baseType && extType && baseType !== extType) {
              mergedProp.type = (numericTypes.has(baseType) && numericTypes.has(extType))
                ? 'number'
                : 'string';
            }
            // Merge enum values: intersect for allOf (default), union for oneOf/anyOf
            const baseEnum = baseProp.enum;
            const extEnum = extProp.enum;
            if (baseEnum && extEnum) {
              if (mode === 'union') {
                const combined = [...baseEnum];
                for (const v of extEnum) {
                  if (!combined.some((existing: any) => existing === v && typeof existing === typeof v)) {
                    combined.push(v);
                  }
                }
                mergedProp.enum = combined;
              } else {
                // Intersect: keep only values present in both (allOf semantics)
                const intersected = baseEnum.filter((v: any) =>
                  extEnum.some((ev: any) => ev === v && typeof ev === typeof v)
                );
                // Preserve the intersection even if empty — an empty enum
                // means the constraint is unsatisfiable, which is the correct
                // allOf semantic for disjoint enums.
                mergedProp.enum = intersected;
              }
            }
            // When only one side has enum, the spread already put whichever
            // exists on mergedProp. This is correct for allOf (the constraint
            // applies). For oneOf/anyOf, resolveSchema handles enum dropping
            // after merging all branches.
            setOwn(merged.properties, propKey, mergedProp);
          }
        } else {
          setOwn(merged.properties, propKey, extProp);
        }
      }
    } else if (key === 'required') {
      merged.required = [...new Set([...(base.required || []), ...(extension.required || [])])];
    } else if (key === '$ref') {
      // $ref is never copied — it must be resolved before merging
    } else if (['allOf', 'anyOf', 'oneOf'].includes(key)) {
      // Composition keywords: if base already has the same keyword, concat.
      // Otherwise copy from extension. This preserves nested constraints
      // like spec.allOf:[{required:['height']}] during deep property merge.
      if (merged[key] && Array.isArray(merged[key])) {
        // Copying a branch costs far less than visiting a node, so it counts
        // as 1/64 of a visit: a doubling list still reaches the budget in
        // about a tenth of a second instead of copying for seconds first.
        countSchemaWork(Math.ceil((merged[key].length + extension[key].length) / 64));
        merged[key] = [...merged[key], ...extension[key]];
      } else {
        merged[key] = extension[key];
      }
    } else {
      setOwn(merged, key, extension[key]);
    }
  }
  return merged;
}

// ─── Schema resolution (composition keywords) ───────────────────────────────

// ─── Reference cycles ────────────────────────────────────────────────────────
// A $ref cycle (a definition that refers to itself, directly, through another
// definition or through allOf) is valid JSON Schema.
//
// Which references can recur at all is decided once per root, statically: a
// reference is recursive when its target reaches it again through the schema's
// $ref graph. A walker treats a reference as a cycle only when it is recursive
// and already being expanded on the current path; that reference is then left
// unresolved (resolveSchema: null, the default builders: undefined,
// evaluateCondition: true, isLeafSchema: false, the form: its
// unresolved-reference message). A schema with no recursive reference is
// therefore walked exactly as before cycle detection existed, siblings
// included. A walker that descends into a stored value (null included) starts
// a fresh path there, because the data's own depth bounds that recursion.

let refPath = new Set<unknown>();

const recursiveRefsByRoot = new WeakMap<object, Set<unknown>>();

// Keywords whose values are subschemas. Annotation and data keywords
// (examples, default, const, enum, unknown x-* keywords) are never followed: a
// "$ref" key inside them is data, not a reference.
const SUBSCHEMA_KEYWORDS = ['additionalProperties', 'items', 'contains', 'not', 'if', 'then', 'else', 'contentSchema',
  'propertyNames', 'additionalItems', 'unevaluatedItems', 'unevaluatedProperties'];
const SUBSCHEMA_LIST_KEYWORDS = ['allOf', 'anyOf', 'oneOf', 'prefixItems', 'items'];
// dependencies (draft-04 to 07) maps a property either to a schema or to an
// array of property names; only the schema values are followed.
const SUBSCHEMA_MAP_KEYWORDS = ['properties', 'patternProperties', 'dependentSchemas', 'dependencies', '$defs', 'definitions'];

/**
 * Call visit on schema and on every subschema it holds, at schema-bearing
 * keywords only, depth first in keyword order. Iterative, so a deeply nested
 * schema cannot overflow the stack.
 */
export function visitSubschemas(schema: unknown, visit: (subschema: JSONSchema) => void): void {
  const pending: unknown[] = [schema];
  while (pending.length) {
    const next = pending.pop();
    if (next === null || typeof next !== 'object' || Array.isArray(next)) continue;
    const s = next as JSONSchema;
    visit(s);
    const children: unknown[] = [];
    for (const key of SUBSCHEMA_KEYWORDS) children.push(ownValue(s, key));
    for (const key of SUBSCHEMA_LIST_KEYWORDS) {
      const list = ownValue(s, key);
      if (Array.isArray(list)) children.push(...list);
    }
    for (const key of SUBSCHEMA_MAP_KEYWORDS) {
      const map = ownValue(s, key);
      if (map !== null && typeof map === 'object' && !Array.isArray(map)) children.push(...Object.values(map));
    }
    for (let i = children.length - 1; i >= 0; i--) pending.push(children[i]);
  }
}

/** Every $ref of schema and of the subschemas it holds; each subschema visited counts as work. */
function refsWithin(schema: unknown, into: Set<string>): Set<string> {
  visitSubschemas(schema, s => {
    countSchemaWork();
    if (typeof s.$ref === 'string') into.add(s.$ref);
  });
  return into;
}

/**
 * The references of root that can reach themselves through its $ref graph: a
 * reference with an edge to itself, or one in a strongly connected component
 * of more than one reference. One iterative Tarjan pass, linear in the graph
 * and charged to the work budget per reference and per edge.
 */
function recursiveRefs(root: JSONSchema): Set<unknown> {
  const cached = recursiveRefsByRoot.get(root);
  if (cached) return cached;
  const edges = new Map<string, Set<string>>();
  const pending = [...refsWithin(root, new Set())];
  while (pending.length) {
    const ref = pending.pop()!;
    if (edges.has(ref)) continue;
    const next = refsWithin(resolveRef(ref, root), new Set());
    countSchemaWork(1 + next.size);
    edges.set(ref, next);
    pending.push(...next);
  }

  const recursive = new Set<unknown>();
  const index = new Map<string, number>();
  const low = new Map<string, number>();
  const onStack = new Set<string>();
  const component: string[] = [];
  let counter = 0;
  const enter = (ref: string, frames: Array<[string, Iterator<string>]>) => {
    index.set(ref, counter);
    low.set(ref, counter);
    counter++;
    component.push(ref);
    onStack.add(ref);
    frames.push([ref, edges.get(ref)!.values()]);
  };
  for (const start of edges.keys()) {
    if (index.has(start)) continue;
    const frames: Array<[string, Iterator<string>]> = [];
    enter(start, frames);
    while (frames.length) {
      const [ref, targets] = frames[frames.length - 1];
      const step = targets.next();
      if (!step.done) {
        const target = step.value;
        if (target === ref) recursive.add(ref);
        if (!index.has(target)) enter(target, frames);
        else if (onStack.has(target)) low.set(ref, Math.min(low.get(ref)!, index.get(target)!));
        continue;
      }
      frames.pop();
      if (frames.length) {
        const parent = frames[frames.length - 1][0];
        low.set(parent, Math.min(low.get(parent)!, low.get(ref)!));
      }
      if (low.get(ref) !== index.get(ref)) continue;
      const members: string[] = [];
      let member: string;
      do {
        member = component.pop()!;
        onStack.delete(member);
        members.push(member);
      } while (member !== ref);
      if (members.length > 1) members.forEach(m => recursive.add(m));
    }
  }
  recursiveRefsByRoot.set(root, recursive);
  return recursive;
}

/** Whether ref, a reference of root, is a cycle here: recursive and already on the path. */
export function onRefPath(ref: unknown, root: JSONSchema | null | undefined): boolean {
  if (!refPath.has(ref) || !root || typeof root !== 'object') return false;
  return recursiveRefs(root).has(ref);
}

/** Run walk with refs added to the current path. */
export function followingRefs<T>(refs: readonly unknown[], walk: () => T): T {
  const added = refs.filter(ref => !refPath.has(ref));
  added.forEach(ref => refPath.add(ref));
  try {
    return walk();
  } finally {
    added.forEach(ref => refPath.delete(ref));
  }
}

/** Run walk on a fresh path: for a walker descending into a stored value. */
export function onFreshRefPath<T>(walk: () => T): T {
  const saved = refPath;
  refPath = new Set();
  try {
    return walk();
  } finally {
    refPath = saved;
  }
}

/**
 * schema with its allOf branches merged in, each branch's $ref resolved one
 * level. A branch whose $ref is on the current path is a cycle, so only its
 * sibling keywords are merged. The refs followed go on the path for the
 * walker's recursion into the result: followingRefs(followed, ...).
 */
export function mergeAllOf(schema: JSONSchema, rootSchema?: JSONSchema): { merged: JSONSchema; followed: unknown[] } {
  let merged: JSONSchema = { ...schema };
  delete merged.allOf;
  const followed: unknown[] = [];
  for (const sub of schema.allOf) {
    countSchemaWork();
    let resolved: JSONSchema;
    if (sub.$ref) {
      const refResult = onRefPath(sub.$ref, rootSchema) ? null : resolveRef(sub.$ref, rootSchema as JSONSchema);
      const siblings: JSONSchema = { ...sub };
      delete siblings.$ref;
      resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
      if (refResult) followed.push(sub.$ref);
    } else {
      resolved = sub;
    }
    if (resolved) merged = mergeSchemas(merged, resolved);
  }
  return { merged, followed };
}

// A last resort against a stack overflow on pathological acyclic input (a
// thousand nested allOf layers); real schemas never come close.
const MAX_SCHEMA_DEPTH = 1000;
let schemaDepth = 0;

export function withinDepth<T>(atLimit: T, walk: () => T): T {
  if (schemaDepth >= MAX_SCHEMA_DEPTH) return atLimit;
  const outermost = schemaDepth === 0;
  schemaDepth++;
  try {
    if (outermost) {
      walkMemo = new Map();
      if (budgetScopes === 0) workDone = 0;
    }
    countSchemaWork();
    return walk();
  } finally {
    schemaDepth--;
    if (schemaDepth === 0) walkMemo = null;
  }
}

// ─── Work budget ─────────────────────────────────────────────────────────────
// References let a schema describe work far larger than its written size: a
// chain like aN = anyOf [ref aN+1, ref aN+1], or a diamond of references to
// the same next level, doubles at every level. Rather than special-case each
// shape (every special case changed some result), the walkers count the work
// they do (one unit per node visited), and past a generous budget they throw
// SchemaTooComplexError. A doubling chain is cut at around 13 to 14 levels
// (thousands of merged branches), within a fraction of a second, while every
// normal schema, including 10,002 properties, stays far below it. The
// budget covers one top-level operation: one render of the display panel,
// form, search fields, tree editor or a [meta] element (withSchemaBudget), or
// one top-level walker call outside those. Every entry point catches the error
// and shows the raw value with "Schema too complex to display": a visible
// failure, never a silently truncated result. Real schemas stay far below it.

export const SCHEMA_WORK_BUDGET = 1_000_000;

export class SchemaTooComplexError extends Error {
  constructor() {
    super(`Schema too complex: more than ${SCHEMA_WORK_BUDGET} schema nodes visited`);
    this.name = 'SchemaTooComplexError';
  }
}

let workDone = 0;
let budgetScopes = 0;

/** Run one top-level operation under a fresh work budget. */
export function withSchemaBudget<T>(operation: () => T): T {
  const outermost = budgetScopes === 0;
  // The walk state as the operation found it: restored afterwards whatever
  // happened, so a budget error thrown mid-walk leaves no depth, memo or
  // reference path behind for the next render.
  const savedDepth = schemaDepth, savedMemo = walkMemo, savedPath = outermost ? new Set(refPath) : refPath;
  if (outermost) workDone = 0;
  budgetScopes++;
  try {
    return operation();
  } finally {
    budgetScopes--;
    if (outermost) {
      schemaDepth = savedDepth;
      walkMemo = savedMemo;
      refPath = savedPath;
    }
  }
}

/** The work counted in the current (or last) top-level operation. */
export function schemaWorkDone(): number {
  return workDone;
}

/** Count amount units of walker work; throws past the budget. Outside any operation it does nothing. */
export function countSchemaWork(amount = 1): void {
  if (budgetScopes === 0 && schemaDepth === 0) return;
  workDone += amount;
  if (workDone > SCHEMA_WORK_BUDGET) throw new SchemaTooComplexError();
}

// ─── Shared subtrees ─────────────────────────────────────────────────────────
// Two walkers avoid repeating identical work without changing any result:
// scoring takes a best score, which byte-identical branches cannot change, and
// evaluateCondition evaluates a referenced subtree once per value within one
// top-level walk (memoized; skipped wherever the reference path matters).

/** branches without byte-identical repeats. */
export function distinctBranches<T>(branches: readonly T[]): T[] {
  countSchemaWork(branches.length);
  const seen = new Set<string>();
  return branches.filter(branch => {
    const key = JSON.stringify(branch);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

let walkMemo: Map<string, Map<object, Map<unknown, unknown>>> | null = null;

/**
 * compute's result for (kind, target, data), computed once per top-level walk.
 * target is a resolved $ref target. Only used when the result cannot depend
 * on the reference path: neither ref nor any reference on the path is
 * recursive (see onRefPath).
 */
function memoized<T>(kind: string, target: object, data: unknown, ref: unknown, root: JSONSchema, compute: () => T): T {
  if (!walkMemo) return compute();
  const recursive = recursiveRefs(root);
  if (recursive.has(ref)) return compute();
  for (const onPath of refPath) if (recursive.has(onPath)) return compute();
  let byTarget = walkMemo.get(kind);
  if (!byTarget) walkMemo.set(kind, (byTarget = new Map()));
  let byData = byTarget.get(target);
  if (!byData) byTarget.set(target, (byData = new Map()));
  if (byData.has(data)) return byData.get(data) as T;
  const result = compute();
  byData.set(data, result);
  return result;
}

export function resolveSchema(schema: JSONSchema | null, rootSchema: JSONSchema): JSONSchema | null {
  return withinDepth(null, () => resolveSchemaUnguarded(schema, rootSchema));
}

function resolveSchemaUnguarded(schema: JSONSchema | null, rootSchema: JSONSchema): JSONSchema | null {
  if (!schema) return schema;

  if (schema.$ref) {
    if (onRefPath(schema.$ref, rootSchema)) return null;
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      // Use mergeSchemas to properly combine ref target with sibling keywords,
      // especially properties which need to be merged, not overwritten.
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      const merged = mergeSchemas(resolved, siblings);
      return followingRefs([schema.$ref], () => resolveSchema(merged, rootSchema));
    }
    return null;
  }

  for (const keyword of ['allOf', 'oneOf', 'anyOf'] as const) {
    if (schema[keyword] && Array.isArray(schema[keyword])) {
      // A labeled enum is the oneOf itself: merging its alternatives would
      // leave only the last const and title, and lose every label. Keep it
      // whole, but still resolve every sibling keyword (anyOf, annotations
      // such as x-display, title, description) around it.
      if (keyword === 'oneOf' && isLabeledEnum(schema)) {
        const rest: JSONSchema = { ...schema };
        delete rest.oneOf;
        const resolvedRest = resolveSchema(rest, rootSchema) ?? rest;
        return { ...resolvedRest, oneOf: schema.oneOf };
      }
      let merged: JSONSchema = { ...schema };
      delete merged[keyword];
      const followed: unknown[] = [];
      for (const sub of schema[keyword]) {
        countSchemaWork();
        let resolved: JSONSchema | null;
        if (sub.$ref) {
          const refResult = onRefPath(sub.$ref, rootSchema) ? null : resolveRef(sub.$ref, rootSchema);
          const siblings: JSONSchema = { ...sub };
          delete siblings.$ref;
          resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
          if (refResult) followed.push(sub.$ref);
        } else {
          resolved = sub;
        }
        if (resolved) {
          // For oneOf/anyOf, branches are alternatives — union their enums.
          // For allOf, branches are constraints — intersect (the default).
          const branchMode = keyword === 'allOf' ? 'intersect' : 'union' as const;
          merged = mergeSchemas(merged, resolved, branchMode);
        }
      }

      // For oneOf/anyOf (alternatives), drop enum on properties where any
      // branch DECLARES the property WITHOUT an enum — that branch allows
      // arbitrary values for that field. Branches that omit the property
      // entirely (or have additionalProperties: false) are not saying
      // "any value is fine" — they're saying "this field doesn't exist
      // in this variant," which doesn't weaken the enum constraint from
      // branches that do declare it.
      if (keyword !== 'allOf' && merged.properties) {
        for (const [propKey, propSchema] of Object.entries(merged.properties)) {
          if (!(propSchema as JSONSchema).enum) continue;
          const anyBranchDeclaresWithoutEnum = schema[keyword].some((sub: JSONSchema) => {
            let resolved = sub;
            if (sub.$ref) {
              const r = resolveRef(sub.$ref, rootSchema);
              if (r) {
                // Properly merge ref target with sibling keywords
                const siblings: JSONSchema = { ...sub };
                delete siblings.$ref;
                resolved = mergeSchemas(r, siblings);
              }
            }
            // Branch declares the property but without an enum → unrestricted
            const prop = resolved.properties?.[propKey];
            return prop && !prop.enum;
          });
          if (anyBranchDeclaresWithoutEnum) {
            delete (propSchema as JSONSchema).enum;
          }
        }
      }

      // An alternative's title names that alternative, not the property.
      if (keyword !== 'allOf') {
        if (schema.title === undefined) delete merged.title;
        else merged.title = schema.title;
      }

      return followingRefs(followed, () => resolveSchema(merged, rootSchema));
    }
  }

  return schema;
}

// ─── Type inference ──────────────────────────────────────────────────────────

export function inferType(val: unknown): string {
  if (Array.isArray(val)) return 'array';
  if (val === null) return 'null';
  const t = typeof val;
  if (t === 'number') {
    return Number.isInteger(val) ? 'integer' : 'number';
  }
  return t;
}

export function inferSchema(val: unknown): JSONSchema {
  const type = inferType(val);
  if (type === 'object') return { type: 'object', properties: {} };
  if (type === 'array') {
    const arr = val as unknown[];
    return { type: 'array', items: arr.length ? inferSchema(arr[0]) : { type: 'string' } };
  }
  return { type };
}

// ─── Condition evaluation ────────────────────────────────────────────────────

export function evaluateCondition(conditionSchema: JSONSchema | null | undefined, data: any, rootSchema?: JSONSchema): boolean {
  return withinDepth(true, () => evaluateConditionUnguarded(conditionSchema, data, rootSchema));
}

function evaluateConditionUnguarded(conditionSchema: JSONSchema | null | undefined, data: any, rootSchema?: JSONSchema): boolean {
  if (!conditionSchema) return true;

  // ── Resolve $ref before any checks ────────────────────────────────────
  if (conditionSchema.$ref && rootSchema) {
    if (onRefPath(conditionSchema.$ref, rootSchema)) return true;
    const resolved = resolveRef(conditionSchema.$ref, rootSchema);
    if (resolved) {
      const siblings: JSONSchema = { ...conditionSchema };
      delete siblings.$ref;
      const merged = mergeSchemas(resolved, siblings);
      const ref = conditionSchema.$ref;
      const evaluate = () => followingRefs([ref], () => evaluateCondition(merged, data, rootSchema));
      return Object.keys(siblings).length ? evaluate() : memoized('condition', resolved, data, ref, rootSchema, evaluate);
    }
  }

  // ── Composition keywords (allOf, anyOf, oneOf) ────────────────────────
  // Resolve these recursively, then continue to check any remaining direct
  // keywords (properties, required, etc.) on the same condition schema.

  if (conditionSchema.allOf && Array.isArray(conditionSchema.allOf)) {
    for (const sub of conditionSchema.allOf) {
      if (!evaluateCondition(sub as JSONSchema, data, rootSchema)) return false;
    }
  }

  if (conditionSchema.anyOf && Array.isArray(conditionSchema.anyOf)) {
    if (!conditionSchema.anyOf.some((sub: JSONSchema) => evaluateCondition(sub, data, rootSchema))) return false;
  }

  if (conditionSchema.oneOf && Array.isArray(conditionSchema.oneOf)) {
    if (conditionSchema.oneOf.filter((sub: JSONSchema) => evaluateCondition(sub, data, rootSchema)).length !== 1) return false;
  }

  if (conditionSchema.not) {
    // not: the sub-schema must NOT match — if it evaluates true, the condition fails
    if (evaluateCondition(conditionSchema.not as JSONSchema, data, rootSchema)) return false;
  }

  // ── Top-level keyword checks (constrain the data value itself) ──────────

  // Top-level const — data itself must equal the value
  if (conditionSchema.const !== undefined && data !== conditionSchema.const) return false;

  // Top-level enum — data itself must be in the list
  if (conditionSchema.enum && !conditionSchema.enum.includes(data)) return false;

  // Top-level type check — always validate, including when properties exist.
  // A condition like { type: "object", properties: { x: { const: "a" } } }
  // should fail if data is a string, not silently pass into the properties loop.
  if (conditionSchema.type) {
    const actualType = inferType(data);
    const expected = conditionSchema.type;
    if (typeof expected === 'string') {
      if (expected !== actualType) {
        if (!(expected === 'number' && actualType === 'integer')) return false;
      }
    } else if (Array.isArray(expected)) {
      if (!expected.includes(actualType) &&
          !(actualType === 'integer' && expected.includes('number'))) return false;
    }
  }

  // Top-level numeric constraints
  if (conditionSchema.minimum !== undefined && (typeof data !== 'number' || data < conditionSchema.minimum)) return false;
  if (conditionSchema.maximum !== undefined && (typeof data !== 'number' || data > conditionSchema.maximum)) return false;
  if (conditionSchema.exclusiveMinimum !== undefined && (typeof data !== 'number' || data <= conditionSchema.exclusiveMinimum)) return false;
  if (conditionSchema.exclusiveMaximum !== undefined && (typeof data !== 'number' || data >= conditionSchema.exclusiveMaximum)) return false;

  // Top-level string constraints
  if (conditionSchema.minLength !== undefined && (typeof data !== 'string' || data.length < conditionSchema.minLength)) return false;
  if (conditionSchema.maxLength !== undefined && (typeof data !== 'string' || data.length > conditionSchema.maxLength)) return false;
  if (conditionSchema.pattern) {
    if (typeof data === 'string') {
      try { if (!new RegExp(conditionSchema.pattern).test(data)) return false; } catch { /* invalid regex — skip */ }
    }
  }

  // Top-level array constraints
  if (conditionSchema.minItems !== undefined && (!Array.isArray(data) || data.length < conditionSchema.minItems)) return false;
  if (conditionSchema.maxItems !== undefined && (!Array.isArray(data) || data.length > conditionSchema.maxItems)) return false;

  // Check required — all listed keys must be present and non-undefined
  if (conditionSchema.required && Array.isArray(conditionSchema.required)) {
    for (const key of conditionSchema.required) {
      if (ownValue(data, key) === undefined) return false;
    }
  }

  // Check properties constraints
  // Per JSON Schema spec, property constraints in `if` only apply to properties
  // that are PRESENT in the data. Absent properties match vacuously — use
  // `required` to demand their presence.
  if (conditionSchema.properties) {
    for (const key in conditionSchema.properties) {
      let propSchema = conditionSchema.properties[key];
      const value = ownValue(data, key);

      // Resolve property-level $ref
      if (propSchema.$ref && rootSchema) {
        const resolved = resolveRef(propSchema.$ref, rootSchema);
        if (resolved) {
          const siblings: JSONSchema = { ...propSchema };
          delete siblings.$ref;
          propSchema = mergeSchemas(resolved, siblings);
        }
      }

      // Skip absent properties — they match vacuously per JSON Schema spec.
      if (value === undefined) continue;

      // Recurse into nested object properties or composition schemas
      if (propSchema.properties || propSchema.allOf || propSchema.anyOf || propSchema.oneOf || propSchema.not) {
        // Descending into the stored value: a fresh reference path.
        if (!onFreshRefPath(() => evaluateCondition(propSchema, value, rootSchema))) return false;
        continue;
      }

      // const match
      if (propSchema.const !== undefined && value !== propSchema.const) return false;

      // enum match
      if (propSchema.enum && !propSchema.enum.includes(value)) return false;

      // type match
      if (propSchema.type) {
        const actualType = inferType(value);
        const expectedType = propSchema.type;
        if (typeof expectedType === 'string') {
          if (expectedType !== actualType) {
            if (!(expectedType === 'number' && actualType === 'integer')) return false;
          }
        } else if (Array.isArray(expectedType)) {
          if (!expectedType.includes(actualType) &&
              !(actualType === 'integer' && expectedType.includes('number'))) return false;
        }
      }

      // minimum/maximum for numbers
      if (propSchema.minimum !== undefined && (typeof value !== 'number' || value < propSchema.minimum)) return false;
      if (propSchema.maximum !== undefined && (typeof value !== 'number' || value > propSchema.maximum)) return false;
      if (propSchema.exclusiveMinimum !== undefined && (typeof value !== 'number' || value <= propSchema.exclusiveMinimum)) return false;
      if (propSchema.exclusiveMaximum !== undefined && (typeof value !== 'number' || value >= propSchema.exclusiveMaximum)) return false;

      // minLength/maxLength for strings
      if (propSchema.minLength !== undefined && (typeof value !== 'string' || value.length < propSchema.minLength)) return false;
      if (propSchema.maxLength !== undefined && (typeof value !== 'string' || value.length > propSchema.maxLength)) return false;

      // pattern for strings
      if (propSchema.pattern && typeof value === 'string') {
        try { if (!new RegExp(propSchema.pattern).test(value)) return false; } catch { /* invalid regex — skip */ }
      }

      // minItems/maxItems for arrays
      if (propSchema.minItems !== undefined && (!Array.isArray(value) || value.length < propSchema.minItems)) return false;
      if (propSchema.maxItems !== undefined && (!Array.isArray(value) || value.length > propSchema.maxItems)) return false;
    }
  }

  return true;
}

// ─── Schema match scoring ────────────────────────────────────────────────────

export function scoreSchemaMatch(schema: JSONSchema, data: unknown, rootSchema: JSONSchema): number {
  return withinDepth(0, () => scoreSchemaMatchUnguarded(schema, data, rootSchema));
}

function scoreSchemaMatchUnguarded(schema: JSONSchema, data: unknown, rootSchema: JSONSchema): number {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema);
    if (resolved) {
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      schema = mergeSchemas(resolved, siblings);
    }
  }

  // Resolve composition so we score against the merged schema.
  // allOf: merge all branches (all constraints apply simultaneously).
  // oneOf/anyOf: score each branch independently, return best score.
  if (schema.allOf && Array.isArray(schema.allOf)) {
    let merged: JSONSchema = { ...schema };
    delete merged.allOf;
    for (const sub of schema.allOf) {
      const resolved = sub.$ref ? resolveRef(sub.$ref, rootSchema) : sub;
      if (resolved) {
        const siblings = sub.$ref ? (() => { const s = {...sub}; delete s.$ref; return s; })() : {};
        merged = mergeSchemas(merged, sub.$ref ? mergeSchemas(resolved, siblings) : resolved);
      }
    }
    schema = merged;
  } else {
    for (const kw of ['oneOf', 'anyOf'] as const) {
      if (schema[kw] && Array.isArray(schema[kw])) {
        // Collect sibling properties (declared alongside oneOf/anyOf)
        const siblings: JSONSchema = { ...schema };
        delete siblings[kw];

        let bestScore = -1;
        for (const sub of distinctBranches(schema[kw] as any[])) {
          // A branch referring back to a reference on the path is a cycle.
          if (sub.$ref && onRefPath(sub.$ref, rootSchema)) continue;
          const resolved = sub.$ref ? (() => {
            const r = resolveRef(sub.$ref, rootSchema);
            const s: JSONSchema = { ...sub }; delete s.$ref;
            return r ? mergeSchemas(r, s) : s;
          })() : sub;
          // Merge branch with sibling properties for scoring
          const branchSchema = mergeSchemas(siblings, resolved);
          const branchScore = followingRefs(sub.$ref ? [sub.$ref] : [], () => scoreSchemaMatch(branchSchema, data, rootSchema));
          if (branchScore > bestScore) bestScore = branchScore;
        }
        return bestScore >= 0 ? bestScore : 0;
      }
    }
  }

  if (schema.const !== undefined) return sameValue(schema.const, data) ? 100 : 0;

  const dataType = inferType(data);
  let schemaType = schema.type;

  if (Array.isArray(schemaType)) {
    if (schemaType.includes(dataType)) return 10;
    if (dataType === 'integer' && schemaType.includes('number')) return 9;
    if (dataType === 'null' && (schemaType.includes('string') || schemaType.includes('number'))) return 5;
    return 0;
  }

  if (schemaType && schemaType !== dataType) {
    if (schemaType === 'number' && dataType === 'integer') return 9;
    return 0;
  }

  if (dataType === 'object' && schema.properties) {
    const dataKeys = Object.keys(data as object);
    const schemaKeys = Object.keys(schema.properties);
    const matchCount = dataKeys.filter(k => schemaKeys.includes(k)).length;
    let score = matchCount + 10;

    // Boost/penalize based on const/enum discriminator matches,
    // recursing into nested objects for deep discriminators.
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      const ps = propSchema as Record<string, any>;
      const val = ownValue(data, key);
      if (val === undefined) continue;

      // const match: strong signal for discriminated unions
      if (ps.const !== undefined) {
        if (sameValue(val, ps.const)) score += 50;
        else return 0; // Definite mismatch — this variant cannot match
      }

      // enum match
      if (ps.enum && Array.isArray(ps.enum)) {
        if (ps.enum.some((member: unknown) => sameValue(member, val))) score += 20;
        else return 0; // Value not in allowed enum — cannot match
      }

      // Resolve the property schema before checking for nested discriminators
      let resolvedProp = ps;
      if (ps.$ref) {
        const r = resolveRef(ps.$ref, rootSchema);
        if (r) {
          const siblings: Record<string, any> = { ...ps };
          delete siblings.$ref;
          resolvedProp = mergeSchemas(r, siblings);
        }
      }

      // Handle composition on the property schema
      if (resolvedProp.allOf && Array.isArray(resolvedProp.allOf)) {
        // allOf: merge all (all constraints apply simultaneously)
        let merged: JSONSchema = { ...resolvedProp };
        delete merged.allOf;
        for (const sub of resolvedProp.allOf) {
          const r = sub.$ref ? resolveRef(sub.$ref, rootSchema) : sub;
          if (r) {
            const siblings = sub.$ref ? (() => { const s = {...sub}; delete s.$ref; return s; })() : {};
            merged = mergeSchemas(merged, sub.$ref ? mergeSchemas(r, siblings) : r);
          }
        }
        resolvedProp = merged;
      } else if (typeof val === 'object' && val !== null && !Array.isArray(val)) {
        // oneOf/anyOf: score each branch independently, take best
        // Also check sibling constraints (properties alongside the composition keyword)
        for (const kw of ['oneOf', 'anyOf'] as const) {
          if (resolvedProp[kw] && Array.isArray(resolvedProp[kw])) {
            // Score sibling constraints (properties declared alongside oneOf/anyOf)
            const siblingSchema: JSONSchema = { ...resolvedProp };
            delete siblingSchema[kw];

            // Check if sibling has any meaningful constraints (not just metadata)
            const metadataOnly = ['title', 'description', 'examples', 'deprecated', 'readOnly', 'writeOnly', 'default', '$comment'];
            const hasSiblingConstraints = Object.keys(siblingSchema).some(k => !metadataOnly.includes(k));
            if (hasSiblingConstraints) {
              const siblingScore = onFreshRefPath(() => scoreSchemaMatch(siblingSchema, val, rootSchema));
              if (siblingScore === 0) return 0; // Sibling constraint mismatch
              score += siblingScore - 10; // Subtract base to avoid double-counting
            }

            // Score composition branches independently
            let bestBranchScore = -1;
            for (const sub of resolvedProp[kw]) {
              const branchScore = onFreshRefPath(() => scoreSchemaMatch(sub, val, rootSchema));
              if (branchScore > bestBranchScore) bestBranchScore = branchScore;
            }
            if (bestBranchScore === 0) return 0;
            if (bestBranchScore > 0) score += bestBranchScore - 10;
            resolvedProp = {}; // Both siblings and branches are scored
            break;
          }
        }
      }

      // Recurse into nested objects for deep discriminators
      if (resolvedProp.properties && typeof val === 'object' && val !== null && !Array.isArray(val)) {
        const nestedScore = onFreshRefPath(() => scoreSchemaMatch(resolvedProp, val, rootSchema));
        if (nestedScore === 0) return 0; // Nested discriminator mismatch
        score += nestedScore - 10; // Subtract base score to avoid double-counting
      }
    }

    // Penalize missing required fields
    if (schema.required && Array.isArray(schema.required)) {
      for (const reqKey of schema.required) {
        if ((data as Record<string, any>)[reqKey] === undefined) {
          score -= 5;
        }
      }
    }

    return score;
  }

  // Handle object schemas with required but no properties (e.g., sibling schemas)
  if (dataType === 'object' && schema.required && Array.isArray(schema.required)) {
    let score = 10;
    for (const reqKey of schema.required) {
      if ((data as Record<string, any>)[reqKey] === undefined) {
        score -= 5;
      }
    }
    return score;
  }

  return 10;
}

// ─── Default values ──────────────────────────────────────────────────────────

export function getDefaultValue(schema: JSONSchema, rootSchema?: JSONSchema): any {
  return withinDepth(undefined, () => getDefaultValueUnguarded(schema, rootSchema));
}

function getDefaultValueUnguarded(schema: JSONSchema, rootSchema?: JSONSchema): any {
  if (schema.$ref) {
    if (onRefPath(schema.$ref, rootSchema || schema)) return undefined;
    const resolved = resolveRef(schema.$ref, rootSchema || schema);
    if (resolved) {
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      const merged = mergeSchemas(resolved, siblings);
      return followingRefs([schema.$ref], () => getDefaultValue(merged, rootSchema));
    }
  }

  if (schema.allOf && Array.isArray(schema.allOf)) {
    const { merged, followed } = mergeAllOf(schema, rootSchema || schema);
    return followingRefs(followed, () => getDefaultValue(merged, rootSchema));
  }

  if (schema.if) {
    const baseSchema: JSONSchema = { ...schema };
    delete baseSchema.if;
    delete baseSchema.then;
    delete baseSchema.else;
    // Evaluate the condition against the base defaults to pick the right branch
    const baseDefault = getDefaultValue(baseSchema, rootSchema);
    const conditionMet = evaluateCondition(schema.if, baseDefault, rootSchema);
    const branch = conditionMet ? (schema.then || {}) : (schema.else || {});
    const merged = mergeSchemas(baseSchema, branch);
    return getDefaultValue(merged, rootSchema);
  }

  if (schema.default !== undefined) return schema.default;
  if (schema.const !== undefined) return schema.const;
  if (schema.enum && Array.isArray(schema.enum) && schema.enum.length > 0) {
    return schema.enum[0];
  }
  if (schema.type === 'object') {
    if (!schema.properties) return {};
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      setOwn(obj, key, getDefaultValue(propSchema as JSONSchema, rootSchema));
    }
    return obj;
  }
  if (schema.type === 'array') return [];
  if (schema.type === 'boolean') return false;
  if (schema.type === 'number' || schema.type === 'integer') return 0;
  if (schema.type === 'null') return null;

  if (Array.isArray(schema.type)) {
    if (schema.type.includes('string')) return '';
    if (schema.type.includes('number') || schema.type.includes('integer')) return 0;
    if (schema.type.includes('boolean')) return false;
    if (schema.type.includes('object')) return {};
    if (schema.type.includes('array')) return [];
    if (schema.type.includes('null')) return null;
  }

  if (schema.oneOf && schema.oneOf.length > 0) {
    if (isLabeledEnum(schema)) return schema.oneOf[0].const;
    return getDefaultValue(schema.oneOf[0], rootSchema);
  }
  if (schema.anyOf && schema.anyOf.length > 0) return getDefaultValue(schema.anyOf[0], rootSchema);

  // Schemas with properties but no explicit type are implicitly objects
  if (schema.properties) {
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      setOwn(obj, key, getDefaultValue(propSchema as JSONSchema, rootSchema));
    }
    return obj;
  }

  return '';
}

// ─── Preview-specific defaults ──────────────────────────────────────────────

/**
 * BH-010: getPreviewDefaultValue mirrors getDefaultValue but returns `undefined`
 * for number/integer/string when no explicit `default` is declared. This is
 * the right semantics for the Preview Form tab — the user should see an empty
 * input, not `0` (or an empty string) that then triggers a range-error onBlur.
 *
 * Booleans still default to `false` (they can't be "empty"). Objects still
 * recurse so nested numeric props flow the preview semantics. Arrays default
 * to [] (no items to seed). enum/const/$ref still resolve their explicit value.
 */
export function getPreviewDefaultValue(schema: JSONSchema, rootSchema?: JSONSchema): any {
  return withinDepth(undefined, () => getPreviewDefaultValueUnguarded(schema, rootSchema));
}

function getPreviewDefaultValueUnguarded(schema: JSONSchema, rootSchema?: JSONSchema): any {
  if (schema.$ref) {
    if (onRefPath(schema.$ref, rootSchema || schema)) return undefined;
    const resolved = resolveRef(schema.$ref, rootSchema || schema);
    if (resolved) {
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      const merged = mergeSchemas(resolved, siblings);
      return followingRefs([schema.$ref], () => getPreviewDefaultValue(merged, rootSchema));
    }
  }

  if (schema.allOf && Array.isArray(schema.allOf)) {
    const { merged, followed } = mergeAllOf(schema, rootSchema || schema);
    return followingRefs(followed, () => getPreviewDefaultValue(merged, rootSchema));
  }

  if (schema.if) {
    const baseSchema: JSONSchema = { ...schema };
    delete baseSchema.if;
    delete baseSchema.then;
    delete baseSchema.else;
    const baseDefault = getPreviewDefaultValue(baseSchema, rootSchema);
    const conditionMet = evaluateCondition(schema.if, baseDefault, rootSchema);
    const branch = conditionMet ? (schema.then || {}) : (schema.else || {});
    const merged = mergeSchemas(baseSchema, branch);
    return getPreviewDefaultValue(merged, rootSchema);
  }

  if (schema.default !== undefined) return schema.default;
  if (schema.const !== undefined) return schema.const;
  if (schema.enum && Array.isArray(schema.enum) && schema.enum.length > 0) {
    return schema.enum[0];
  }

  if (schema.type === 'object') {
    if (!schema.properties) return {};
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      setOwn(obj, key, getPreviewDefaultValue(propSchema as JSONSchema, rootSchema));
    }
    return obj;
  }
  if (schema.type === 'array') return [];
  if (schema.type === 'boolean') return false;
  if (schema.type === 'number' || schema.type === 'integer') return undefined;
  if (schema.type === 'string') return undefined;
  if (schema.type === 'null') return null;

  if (Array.isArray(schema.type)) {
    // Preview: prefer the least-surprising empty state — null > boolean > object/array > undefined.
    if (schema.type.includes('null')) return null;
    if (schema.type.includes('boolean')) return false;
    if (schema.type.includes('object')) return {};
    if (schema.type.includes('array')) return [];
    // string/number/integer → undefined (inputs render empty)
    return undefined;
  }

  if (schema.oneOf && schema.oneOf.length > 0) {
    if (isLabeledEnum(schema)) return schema.oneOf[0].const;
    return getPreviewDefaultValue(schema.oneOf[0], rootSchema);
  }
  if (schema.anyOf && schema.anyOf.length > 0) return getPreviewDefaultValue(schema.anyOf[0], rootSchema);

  // Schemas with properties but no explicit type are implicitly objects
  if (schema.properties) {
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      setOwn(obj, key, getPreviewDefaultValue(propSchema as JSONSchema, rootSchema));
    }
    return obj;
  }

  return undefined;
}

// ─── Title case ──────────────────────────────────────────────────────────────

export function titleCase(key: string): string {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
}

// ─── Labeled enum detection ─────────────────────────────────────────────────

/**
 * Returns true when a schema represents a "labeled enum" — a `oneOf` array
 * where every entry is a simple `{ const, title?, description? }` object
 * with no complex subschema keywords.
 */
export function isLabeledEnum(schema: JSONSchema): boolean {
  if (!schema.oneOf || !Array.isArray(schema.oneOf) || schema.oneOf.length === 0) return false;
  const complexKeys = new Set(['type', 'properties', 'items', 'oneOf', 'anyOf', 'allOf', 'if', '$ref', 'enum']);
  return schema.oneOf.every((entry: JSONSchema) => {
    if (!entry || typeof entry !== 'object' || entry.const === undefined) return false;
    return !Object.keys(entry).some(k => complexKeys.has(k));
  });
}

/**
 * Extracts the enum values and labels from a labeled-enum schema.
 * Returns an array of { value, label } objects.
 */
export function getLabeledEnumEntries(schema: JSONSchema): Array<{ value: any; label: string; color?: string }> {
  if (!schema.oneOf) return [];
  return schema.oneOf.map((entry: JSONSchema) => ({
    value: entry.const,
    label: (typeof entry.title === 'string' && entry.title) || valueText(entry.const),
    ...(safeColor(entry['x-color']) ? { color: entry['x-color'] } : {}),
  }));
}

/**
 * A schema const, enum entry, default or stored value as text. Primitives keep
 * their String() form; a non-primitive goes through JSON, because String() and
 * Lit's attribute and .value coercion throw on a parsed {"toString":null} and
 * turn every other object into the same "[object Object]".
 */
export function valueText(value: unknown): string {
  return value !== null && typeof value === 'object' ? JSON.stringify(value) : String(value);
}

const isNumber = (v: unknown) => typeof v === 'number';
const isString = (v: unknown) => typeof v === 'string';

/**
 * The value type each checked keyword allows, as the union across draft-04,
 * draft-07 and 2020-12 (where the drafts differ, both forms are allowed).
 * Keywords not listed are never checked.
 */
const KEYWORD_TYPES: Record<string, (value: unknown) => boolean> = {
  minimum: isNumber,
  maximum: isNumber,
  multipleOf: isNumber,
  // A number from draft-06 on; a boolean modifier of minimum/maximum in draft-04.
  exclusiveMinimum: v => isNumber(v) || typeof v === 'boolean',
  exclusiveMaximum: v => isNumber(v) || typeof v === 'boolean',
  minLength: isNumber,
  maxLength: isNumber,
  minItems: isNumber,
  maxItems: isNumber,
  minProperties: isNumber,
  maxProperties: isNumber,
  title: isString,
  description: isString,
  pattern: isString,
  format: isString,
  type: v => isString(v) || (Array.isArray(v) && v.every(isString)),
  'x-display': isString,
  'x-color': v => safeColor(v) !== undefined,
};

/** False only for a checked keyword holding a value of a type no draft allows. */
export function keywordValueValid(key: string, value: unknown): boolean {
  return !hasOwn(KEYWORD_TYPES, key) || KEYWORD_TYPES[key](value);
}

/** schema without the keywords keywordValueValid rejects. */
function validKeywords(schema: JSONSchema): JSONSchema {
  const out: JSONSchema = {};
  for (const [key, value] of Object.entries(schema)) {
    if (keywordValueValid(key, value)) setOwn(out, key, value);
  }
  return out;
}

/**
 * A copy of schema that renderers can read safely.
 *
 * - Keywords holding a value of a type no draft allows are dropped: a
 *   non-number minimum or maxLength, a non-string title, pattern, format or
 *   x-display, a type that is not a string or an array of strings. A
 *   hand-written schema can hold any JSON there, and a renderer would coerce
 *   it into text, an attribute, a comparison or a RegExp.
 * - A draft-04 boolean exclusiveMinimum/exclusiveMaximum is normalised to the
 *   draft-06+ form: true with a numeric minimum becomes exclusiveMinimum set to
 *   that bound; false, or true with no bound, leaves the inclusive bound alone.
 *
 * Renderers read a property schema through this; the tree editor, which edits
 * the schema itself, shows the raw values through inputText instead.
 */
export function withTypedKeywords(schema: JSONSchema): JSONSchema {
  if (!schema || typeof schema !== 'object') return schema;
  let copy: JSONSchema | null = null;
  const edit = (): JSONSchema => (copy ??= { ...schema });
  for (const key of Object.keys(KEYWORD_TYPES)) {
    if (hasOwn(schema, key) && !keywordValueValid(key, schema[key])) delete edit()[key];
  }
  for (const [flag, bound] of [['exclusiveMinimum', 'minimum'], ['exclusiveMaximum', 'maximum']] as const) {
    const current = copy ?? schema;
    if (typeof current[flag] !== 'boolean') continue;
    const target = edit();
    if (current[flag] && typeof current[bound] === 'number') {
      target[flag] = current[bound];
      delete target[bound];
    } else {
      delete target[flag];
    }
  }
  return copy ?? schema;
}

/** valueText for an input's .value: an absent value (undefined or null) is the empty string. */
export function inputText(value: unknown): string {
  return value === undefined || value === null ? '' : valueText(value);
}

/**
 * A type-tagged identity for a JSON value. Equal keys mean equal values, and
 * object key order does not matter ({b:2,a:1} equals {a:1,b:2}), while values
 * that print alike stay apart: {"a":1} and the string '{"a":1}', or 1 and "1".
 * Compare, sort and dedupe raw values through this, never directly.
 */
export function valueKey(value: unknown): string {
  const kind = value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value;
  return `${kind}:${canonicalJSON(value)}`;
}

/** JSON with every object's keys sorted, recursively; array order is kept. */
function canonicalJSON(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`;
  if (value !== null && typeof value === 'object') {
    const entries = Object.keys(value).sort()
      .map(key => `${JSON.stringify(key)}:${canonicalJSON((value as Record<string, unknown>)[key])}`);
    return `{${entries.join(',')}}`;
  }
  return JSON.stringify(value) ?? String(value);
}

/** Deep, type-aware equality of two JSON values. */
export function sameValue(a: unknown, b: unknown): boolean {
  return a === b || valueKey(a) === valueKey(b);
}

// ─── Enum choices ────────────────────────────────────────────────────────────
// A choice's identity is its position in the enum (or labeled oneOf), never
// its text: two choices can print the same. Controls key their options with
// choiceKey and map a key back with choiceIndex; labels are display only.

/** The key of the choice at index, for an <option> or checkbox value. */
export function choiceKey(index: number): string {
  return String(index);
}

/** The index a choice key names, or -1 when it names none of count choices. */
export function choiceIndex(key: string, count: number): number {
  if (!/^\d+$/.test(key)) return -1;
  const index = Number(key);
  return index < count ? index : -1;
}

/** The index of the first choice equal to value (deep, type-aware), or -1. */
export function findChoice(choices: readonly unknown[], value: unknown): number {
  return choices.findIndex(choice => sameValue(choice, value));
}

// Characters a CSS color can contain; anything else (";", ":", braces,
// quotes) could break out of the declaration it is interpolated into.
const SAFE_COLOR_CHARS = /^[#\w\s(),.%/+-]+$/;

/**
 * An x-color value when it is a string CSS color made of safe characters,
 * else undefined. Hand-written schemas can hold any JSON there.
 */
export function safeColor(value: unknown): string | undefined {
  if (typeof value !== 'string' || !SAFE_COLOR_CHARS.test(value)) return undefined;
  if (typeof CSS !== 'undefined' && typeof CSS.supports === 'function' && !CSS.supports('color', value)) {
    return undefined;
  }
  return value;
}

// ─── Schema flattening (for search mode) ─────────────────────────────────────

export function flattenSchema(
  schema: JSONSchema,
  prefix = '',
  labelPrefix = '',
  depth = 0,
  rootSchema: JSONSchema | null = null,
): FlatField[] {
  return withinDepth([], () => flattenSchemaUnguarded(schema, prefix, labelPrefix, depth, rootSchema));
}

function flattenSchemaUnguarded(
  schema: JSONSchema,
  prefix: string,
  labelPrefix: string,
  depth: number,
  rootSchema: JSONSchema | null,
): FlatField[] {
  if (depth > 10 || !schema) return [];

  const root = rootSchema || schema;
  const resolved = resolveSchema(schema, root);
  if (!resolved || !resolved.properties) return [];

  const fields: FlatField[] = [];

  for (const [key, rawPropIn] of Object.entries(resolved.properties) as [string, JSONSchema][]) {
    if (!isSchemaValue(rawPropIn)) continue;
    const path = prefix ? `${prefix}.${key}` : key;
    const rawProp = withTypedKeywords(rawPropIn);
    const prop = withTypedKeywords(resolveSchema(rawProp, root) || rawProp);
    const rawLabel = prop.title || titleCase(key);
    const label = labelPrefix ? `${labelPrefix} › ${rawLabel}` : rawLabel;

    // Labeled enum: oneOf with const+title entries
    if (isLabeledEnum(prop)) {
      const entries = getLabeledEnumEntries(prop);
      let fieldType = prop.type || 'string';
      if (Array.isArray(fieldType)) {
        fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
      }
      // Infer type from const values if no explicit type
      if (!prop.type && entries.length > 0) {
        const representative = entries.find(e => e.value !== null)?.value ?? entries[0].value;
        if (representative === null) fieldType = 'null';
        else if (typeof representative === 'number') fieldType = Number.isInteger(representative) ? 'integer' : 'number';
        else if (typeof representative === 'boolean') fieldType = 'boolean';
      }
      fields.push({
        path,
        label,
        type: fieldType,
        enum: entries.map(e => e.value),
        enumLabels: entries.map(e => e.label),
      });
      continue;
    }

    if (prop.properties) {
      fields.push(...flattenSchema(prop, path, label, depth + 1, root));
    } else if (prop.type === 'array') {
      continue;
    } else {
      // Normalize nullable type arrays (e.g. ["string", "null"]) to their
      // base scalar type so search-mode operators and serialization work.
      let fieldType = prop.type || 'string';
      if (Array.isArray(fieldType)) {
        fieldType = fieldType.find((t: string) => t !== 'null') || 'string';
      }
      // If type is defaulted (no explicit type) and enum exists, infer from enum values
      if (!prop.type && Array.isArray(prop.enum) && prop.enum.length > 0) {
        // Find the first non-null value to infer type, since null alone isn't searchable
        const representative = prop.enum.find((v: any) => v !== null) ?? prop.enum[0];
        if (representative === null) {
          fieldType = 'null';
        } else if (typeof representative === 'number') {
          fieldType = Number.isInteger(representative) ? 'integer' : 'number';
        } else if (typeof representative === 'boolean') {
          fieldType = 'boolean';
        }
        // string is already the default
      }
      fields.push({
        path,
        label,
        type: fieldType,
        enum: Array.isArray(prop.enum) ? prop.enum : null,
        enumLabels: null,
      });
    }
  }

  return fields;
}

// ─── Field intersection (multi-schema search) ────────────────────────────────

export function intersectFields(fieldLists: FlatField[][]): FlatField[] {
  if (fieldLists.length === 0) return [];
  if (fieldLists.length === 1) return fieldLists[0];

  const base = new Map(fieldLists[0].map(f => [f.path, { ...f }]));

  for (let i = 1; i < fieldLists.length; i++) {
    const currentPaths = new Set(fieldLists[i].map(f => f.path));

    for (const path of base.keys()) {
      if (!currentPaths.has(path)) {
        base.delete(path);
      }
    }

    for (const field of fieldLists[i]) {
      const existing = base.get(field.path);
      if (!existing) continue;

      const numericTypes = new Set(['number', 'integer']);
      if (existing.type !== field.type) {
        if (numericTypes.has(existing.type) && numericTypes.has(field.type)) {
          existing.type = 'number';
        } else {
          existing.type = 'string';
          existing.enum = null;
          existing.enumLabels = null;
          continue;
        }
      }
      if (existing.enum && field.enum) {
        // Compare as sets of typed keys: sorting raw values coerces them.
        const a = existing.enum.map(valueKey).sort();
        const b = field.enum.map(valueKey).sort();
        if (JSON.stringify(a) !== JSON.stringify(b)) {
          existing.enum = null;
          existing.enumLabels = null;
        } else if (existing.enumLabels || field.enumLabels) {
          // Enums match but labels may differ — drop labels if they don't match
          if (JSON.stringify(existing.enumLabels) !== JSON.stringify(field.enumLabels)) {
            existing.enumLabels = null;
          }
        }
      } else if (existing.enum !== field.enum) {
        existing.enum = null;
        existing.enumLabels = null;
      }
    }
  }

  return Array.from(base.values());
}

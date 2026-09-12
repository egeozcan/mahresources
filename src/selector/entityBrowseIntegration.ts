import { encodeBrowseParameters, type BrowseValue, type EntityBrowseMetadata, type EntityBrowseSource } from './entityBrowseTypes';

export interface EntityBrowseOrigin {
 metadata: EntityBrowseMetadata;
 getValues(): readonly BrowseValue[];
 isAvailable(): boolean;
 replace(values: readonly BrowseValue[]): void;
}

/** Eligibility is re-read before the one ordinary, non-silent selector command.
 * No association writes belong here: the origin's existing observer owns them.
 */
export function createEntityBrowseConfirmation(origin: EntityBrowseOrigin, source: EntityBrowseSource) {
 let destroyed = false, controller: AbortController | null = null;
 return Object.freeze({
  confirm: async (values: readonly BrowseValue[]): Promise<boolean> => {
   if (destroyed || controller || !origin.isAvailable()) return false;
   const metadata = origin.metadata, entity = metadata.entity, multiple = metadata.multiple;
   const ids = [...new Set(values.map(value => Number(value.ID)))];
   if (!ids.length) return multiple;
   if (ids.some(id => !Number.isSafeInteger(id) || id < 1) || (!multiple && ids.length !== 1)) throw new Error('Invalid selection');
   const constraints = encodeBrowseParameters(metadata.parameters());
   controller = new AbortController();
   const signal = controller.signal;
   try {
    const resolved = await source.resolve({ entity, constraints, ids }, signal);
    if (destroyed || signal.aborted || !origin.isAvailable()) return false;
    if (entity !== metadata.entity || multiple !== metadata.multiple || constraints !== encodeBrowseParameters(metadata.parameters())) {
     throw new Error('This field’s filters changed. Refresh the results and try again.');
    }
    const excluded = new Set(metadata.excludedKeys().map(String));
    const byID = new Map(resolved.map(value => [String(value.ID), value]));
    if (ids.some(id => !byID.has(String(id)) || excluded.has(String(id)))) {
     throw new Error('A selected item is no longer available. Refresh the results and try again.');
    }
    const current = origin.getValues();
    const next = new Map(multiple ? current.map(value => [String(value.ID), value]) : []);
    for (const id of ids) if (!next.has(String(id))) next.set(String(id), byID.get(String(id))!);
    if (multiple && metadata.maximum !== undefined && next.size > metadata.maximum) {
     throw new Error('The selection exceeds this field’s remaining capacity.');
    }
    const nextKeys = [...next.keys()];
    const same = current.length === next.size && current.every((value, index) => String(value.ID) === nextKeys[index]);
    if (!same) origin.replace([...next.values()]);
    return true;
   } finally { controller = null; }
  },
  destroy() {destroyed = true;controller?.abort();},
 });
}

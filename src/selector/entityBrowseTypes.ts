import type { EntityProfileName } from './entityFieldProfiles';
import type { SelectorHttpParameter } from './httpSelectorSource';

export interface BrowseValue { ID: number; Name: string; [key: string]: unknown }
export interface BrowseResult { value: BrowseValue; html: string }
export interface BrowseStyle { key: string; css: string }
export interface BrowsePage { items: BrowseResult[]; page: number; hasNext: boolean; styles: BrowseStyle[]; warnings: string[] }
export interface EntityBrowseSource {
 search(input: { entity: EntityProfileName; filter: string; constraints: string; page: number }, signal: AbortSignal): Promise<BrowsePage>;
 resolve(input: { entity: EntityProfileName; constraints: string; ids: number[] }, signal: AbortSignal): Promise<BrowseValue[]>;
}
export interface EntityBrowseMetadata {
 readonly entity: EntityProfileName;
 readonly multiple: boolean;
 readonly maximum?: number;
 readonly parameters: () => Readonly<Record<string, SelectorHttpParameter>>;
 readonly excludedKeys: () => readonly (string | number)[];
}

export function encodeBrowseParameters(parameters: Readonly<Record<string, SelectorHttpParameter>>): string {
 const query = new URLSearchParams();
 for (const [name, value] of Object.entries(parameters)) {
  for (const item of Array.isArray(value) ? value : [value]) {
   if (item !== undefined && item !== null) query.append(name, String(item));
  }
 }
 return query.toString();
}

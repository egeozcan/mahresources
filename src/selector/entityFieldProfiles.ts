import { createDebouncedSelectorSource } from './debouncedSelectorSource';
import {
    createHttpSelectorSource,
    type SelectorHttpParameter,
} from './httpSelectorSource';
import { createSelector } from './selectorCore';
import type { SelectorHandle, SelectorKey, SelectorOption } from './types';

const PROFILE_SEARCH_DELAY_MS = 200;

const entityEndpointCatalog = {
    category: '/v1/categories',
    group: '/v1/groups',
    note: '/v1/notes',
    noteType: '/v1/note/noteTypes',
    query: '/v1/queries',
    relationType: '/v1/relationTypes',
    resource: '/v1/resources',
    resourceCategory: '/v1/resourceCategories',
    series: '/v1/seriesList',
    tag: '/v1/tags',
} as const;

export type EntityProfileName = keyof typeof entityEndpointCatalog;

export interface SelectorEntityValue {
    readonly ID: SelectorKey;
    readonly Name: string;
    readonly [key: string]: unknown;
}

export interface EntityFieldFormInput {
    readonly name: string;
    readonly minimum?: number;
}

export interface EntityFieldFormMetadata {
    readonly name: string;
    readonly minimum: number;
    /** Render the enabled empty hidden control that allows a relationship to be cleared. */
    readonly includeEmptyControl: true;
}

export interface EntityFieldInteractionMetadata {
    readonly tokenDelimiters: readonly [','];
    readonly commitOnSpace: false;
}

export interface EntityFieldPresentationMetadata {
    /** Optional secondary category-name decoration; the selector label remains the entity Name. */
    readonly decoration: 'category-name' | null;
}

export interface EntityFieldProfile<TRaw extends SelectorEntityValue> {
    readonly selector: SelectorHandle<TRaw>;
    readonly form: EntityFieldFormMetadata | null;
    readonly interaction: EntityFieldInteractionMetadata;
    readonly presentation: EntityFieldPresentationMetadata;
}

export interface EntityFieldProfileOptions<TRaw extends SelectorEntityValue> {
    readonly entity: EntityProfileName;
    readonly selected?: readonly TRaw[];
    readonly form?: EntityFieldFormInput;
    readonly parameters?: () => Readonly<Record<string, SelectorHttpParameter>>;
    readonly categoryDecoration?: boolean;
}

export interface MultiEntityFieldProfileOptions<TRaw extends SelectorEntityValue>
    extends EntityFieldProfileOptions<TRaw> {
    readonly maximum?: number;
}

function mapEntityOption<TRaw extends SelectorEntityValue>(raw: TRaw): SelectorOption<TRaw> {
    return { key: raw.ID, label: raw.Name, raw };
}

function formMetadata(form: EntityFieldFormInput | undefined): EntityFieldFormMetadata | null {
    if (!form) return null;
    return Object.freeze({
        name: form.name,
        minimum: Math.max(0, Math.floor(form.minimum ?? 0)),
        includeEmptyControl: true as const,
    });
}

function interactionMetadata(): EntityFieldInteractionMetadata {
    return Object.freeze({
        tokenDelimiters: Object.freeze([','] as [',']),
        commitOnSpace: false as const,
    });
}

function presentationMetadata(categoryDecoration: boolean): EntityFieldPresentationMetadata {
    return Object.freeze({
        decoration: categoryDecoration ? 'category-name' as const : null,
    });
}

interface BuildEntityFieldProfileOptions<TRaw extends SelectorEntityValue>
    extends EntityFieldProfileOptions<TRaw> {
    readonly multiple: boolean;
    readonly maximum?: number;
}

function buildEntityFieldProfile<TRaw extends SelectorEntityValue>(
    options: BuildEntityFieldProfileOptions<TRaw>,
): EntityFieldProfile<TRaw> {
    const source = createDebouncedSelectorSource(createHttpSelectorSource({
        searchUrl: entityEndpointCatalog[options.entity],
        parameters: options.parameters,
        mapOption: mapEntityOption<TRaw>,
    }), PROFILE_SEARCH_DELAY_MS);
    const selector = createSelector({
        source,
        selected: options.selected?.map(mapEntityOption),
        multiple: options.multiple,
        maxSelected: options.multiple ? options.maximum : undefined,
    });

    return Object.freeze({
        selector,
        form: formMetadata(options.form),
        interaction: interactionMetadata(),
        presentation: presentationMetadata(options.categoryDecoration ?? false),
    });
}

/** Creates a zero-or-one entity selector without exposing its source or core configuration. */
export function createSingleEntityFieldProfile<TRaw extends SelectorEntityValue>(
    options: EntityFieldProfileOptions<TRaw>,
): EntityFieldProfile<TRaw> {
    return buildEntityFieldProfile({ ...options, multiple: false });
}

/** Creates an ordinary non-creatable multi-entity selector. */
export function createMultiEntityFieldProfile<TRaw extends SelectorEntityValue>(
    options: MultiEntityFieldProfileOptions<TRaw>,
): EntityFieldProfile<TRaw> {
    return buildEntityFieldProfile({ ...options, multiple: true });
}

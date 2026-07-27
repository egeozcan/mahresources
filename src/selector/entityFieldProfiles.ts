import { createDebouncedSelectorSource } from './debouncedSelectorSource';
import {
    createHttpSelectorSource,
    type SelectorHttpParameter,
} from './httpSelectorSource';
import { createSelector } from './selectorCore';
import type { SelectorHandle, SelectorKey, SelectorOption } from './types';

const PROFILE_SEARCH_DELAY_MS = 200;

const entityEndpointCatalog = {
    category: { search: '/v1/categories', create: '/v1/category' },
    group: { search: '/v1/groups' },
    note: { search: '/v1/notes' },
    noteType: { search: '/v1/note/noteTypes' },
    query: { search: '/v1/queries' },
    relationType: { search: '/v1/relationTypes' },
    resource: { search: '/v1/resources' },
    resourceCategory: { search: '/v1/resourceCategories' },
    series: { search: '/v1/seriesList', create: '/v1/series/create' },
    tag: { search: '/v1/tags', create: '/v1/tag', suggestions: '/v1/tags/suggest' },
} as const;

export type EntityProfileName = keyof typeof entityEndpointCatalog;
export type CreatableEntityProfileName = 'category' | 'series';
export type TagUsageEntity = 'group' | 'note' | 'resource';

export interface SelectorEntityValue {
    readonly ID: SelectorKey;
    readonly Name: string;
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

export interface TagSuggestionSourceInput {
    /** Rank the lean tag suggestions by associations with this owning entity type. */
    readonly usage: TagUsageEntity;
}

export interface MultiEntityFieldProfileOptions<TRaw extends SelectorEntityValue>
    extends EntityFieldProfileOptions<TRaw> {
    readonly maximum?: number;
    /** Selects the private lean tag source without enabling tag creation. */
    readonly tagSuggestions?: TagSuggestionSourceInput;
}

export interface CreatableEntityFieldProfileOptions<TRaw extends SelectorEntityValue>
    extends Omit<EntityFieldProfileOptions<TRaw>, 'entity'> {
    readonly entity: CreatableEntityProfileName;
}

export interface TagFieldProfileOptions<TRaw extends SelectorEntityValue>
    extends Omit<EntityFieldProfileOptions<TRaw>, 'entity' | 'categoryDecoration'> {
    readonly usage: TagUsageEntity;
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
    readonly create?: boolean;
    readonly tagSuggestions?: TagSuggestionSourceInput;
}

function buildEntityFieldProfile<TRaw extends SelectorEntityValue>(
    options: BuildEntityFieldProfileOptions<TRaw>,
): EntityFieldProfile<TRaw> {
    if (options.tagSuggestions && options.entity !== 'tag') {
        throw new Error('Lean tag suggestions can only be used with the tag entity profile');
    }
    const catalogEntry = entityEndpointCatalog[options.entity];
    const tagCatalogEntry = options.entity === 'tag' ? entityEndpointCatalog.tag : null;
    const searchUrl = options.tagSuggestions
        ? tagCatalogEntry!.suggestions
        : catalogEntry.search;
    const createUrl = options.create && 'create' in catalogEntry
        ? catalogEntry.create
        : undefined;
    const parameters = options.parameters || options.tagSuggestions
        ? () => ({
            ...options.parameters?.(),
            ...(options.tagSuggestions
                ? { SortBy: `most_used_${options.tagSuggestions.usage}` }
                : {}),
        })
        : undefined;
    const source = createDebouncedSelectorSource(createHttpSelectorSource({
        searchUrl,
        createUrl,
        parameters,
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

/** Creates a zero-or-one form field that may create an entity before selecting it. */
export function createCreatableEntityFieldProfile<TRaw extends SelectorEntityValue>(
    options: CreatableEntityFieldProfileOptions<TRaw>,
): EntityFieldProfile<TRaw> {
    return buildEntityFieldProfile({ ...options, multiple: false, create: true });
}

/** Creates the creatable multi-tag preset with lean, usage-ranked suggestions. */
export function createTagFieldProfile<TRaw extends SelectorEntityValue>(
    options: TagFieldProfileOptions<TRaw>,
): EntityFieldProfile<TRaw> {
    const { usage, ...fieldOptions } = options;
    return buildEntityFieldProfile({
        ...fieldOptions,
        entity: 'tag',
        multiple: true,
        create: true,
        tagSuggestions: { usage },
    });
}

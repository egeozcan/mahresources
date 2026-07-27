import type { SelectorOption } from '../selector/types';

export interface LegacyAutocompleterRawOption {
    readonly ID: string | number;
    readonly Name: string;
    readonly [key: string]: unknown;
}

export interface LegacyAutocompleterArguments<
    TRaw extends LegacyAutocompleterRawOption = LegacyAutocompleterRawOption,
> {
    readonly selectedResults?: TRaw[] | string | null;
    readonly max?: string | number | null;
    readonly min?: string | number | null;
    readonly ownerId?: string | number | null;
    readonly url: string;
    readonly sortBy?: string;
    readonly elName?: string;
    readonly addUrl?: string;
    readonly extraInfo?: string;
    readonly onSelect?: ((item: TRaw) => unknown) | null;
    readonly onRemove?: ((item: TRaw) => unknown) | null;
    readonly standalone?: boolean;
    readonly commitOnSpace?: boolean;
}

export interface LegacySelectorSourceConfiguration<
    TRaw extends LegacyAutocompleterRawOption = LegacyAutocompleterRawOption,
> {
    readonly searchUrl: string;
    readonly createUrl: string;
    readonly ownerId: number;
    readonly sortBy?: string;
    readonly mapOption: (raw: TRaw) => SelectorOption<TRaw>;
}

export interface LegacySelectorSelectionConfiguration<
    TRaw extends LegacyAutocompleterRawOption = LegacyAutocompleterRawOption,
> {
    readonly initialValues: TRaw[];
    readonly minimum: number;
    readonly maximum: number;
}

export interface LegacySelectorRenderingConfiguration<
    TRaw extends LegacyAutocompleterRawOption = LegacyAutocompleterRawOption,
> {
    readonly fieldName?: string;
    readonly extraInfo: string;
    readonly standalone: boolean;
    readonly commitOnSpace: boolean;
    readonly onSelect: ((item: TRaw) => unknown) | null;
    readonly onRemove: ((item: TRaw) => unknown) | null;
}

export interface NormalizedLegacyAutocompleterConfiguration<
    TRaw extends LegacyAutocompleterRawOption = LegacyAutocompleterRawOption,
> {
    readonly source: LegacySelectorSourceConfiguration<TRaw>;
    readonly selection: LegacySelectorSelectionConfiguration<TRaw>;
    readonly rendering: LegacySelectorRenderingConfiguration<TRaw>;
}

function parseSelectedValues<TRaw extends LegacyAutocompleterRawOption>(
    selectedResults: TRaw[] | string | null | undefined,
): TRaw[] {
    return typeof selectedResults === 'string'
        ? JSON.parse(selectedResults) as TRaw[]
        : selectedResults || [];
}

function parseLegacyInteger(value: string | number | null | undefined): number {
    return parseInt(value as string) || 0;
}

/**
 * Translates the public legacy Alpine arguments without creating a selector core.
 * The compatibility adapter remains solely responsible for behavior in this phase.
 */
export function normalizeLegacyAutocompleterConfig<
    TRaw extends LegacyAutocompleterRawOption = LegacyAutocompleterRawOption,
>(
    arguments_: LegacyAutocompleterArguments<TRaw>,
): NormalizedLegacyAutocompleterConfiguration<TRaw> {
    const {
        selectedResults,
        max,
        min,
        ownerId,
        url,
        sortBy,
        elName,
        addUrl = '',
        extraInfo = '',
        onSelect = null,
        onRemove = null,
        standalone = false,
        commitOnSpace = false,
    } = arguments_;

    return {
        source: {
            searchUrl: url,
            createUrl: addUrl,
            ownerId: parseLegacyInteger(ownerId),
            sortBy,
            mapOption: (raw) => ({ key: raw.ID, label: raw.Name, raw }),
        },
        selection: {
            initialValues: parseSelectedValues(selectedResults),
            minimum: parseLegacyInteger(min),
            maximum: parseLegacyInteger(max),
        },
        rendering: {
            fieldName: elName,
            extraInfo,
            standalone,
            commitOnSpace,
            onSelect,
            onRemove,
        },
    };
}

import type { SelectorOption, SelectorSource } from './types';

type SelectorHttpScalar = string | number | boolean | null | undefined;
export type SelectorHttpParameter = SelectorHttpScalar | readonly SelectorHttpScalar[];

export interface HttpSelectorSourceConfig<TRaw> {
    readonly searchUrl: string;
    readonly createUrl?: string;
    readonly parameters?: () => Readonly<Record<string, SelectorHttpParameter>>;
    readonly mapOption: (raw: TRaw) => SelectorOption<TRaw>;
    readonly fetch?: typeof globalThis.fetch;
}

export class HttpSelectorSourceError extends Error {
    readonly operation: 'search' | 'create';
    readonly status: number;

    constructor(operation: 'search' | 'create', status: number) {
        super(`Selector ${operation} request failed with status ${status}`);
        this.name = 'HttpSelectorSourceError';
        this.operation = operation;
        this.status = status;
    }
}

function requestUrl(
    endpoint: string,
    query: string,
    parameters: Readonly<Record<string, SelectorHttpParameter>>,
): string {
    const searchParameters = new URLSearchParams();
    for (const [key, value] of Object.entries(parameters)) {
        for (const item of Array.isArray(value) ? value : [value]) {
            if (item !== null && item !== undefined) searchParameters.append(key, String(item));
        }
    }
    searchParameters.set('name', query);
    const separator = endpoint.includes('?') ? '&' : '?';
    return `${endpoint}${separator}${searchParameters.toString()}`;
}

async function responseJson(
    response: Response,
    operation: 'search' | 'create',
): Promise<unknown> {
    if (!response.ok) throw new HttpSelectorSourceError(operation, response.status);
    return response.json();
}

function isSingleResponseValue(value: unknown): boolean {
    return value !== null && value !== undefined && !Array.isArray(value);
}

/**
 * A browser-fetch source adapter. Entity-specific response mapping remains supplied by its profile.
 */
export function createHttpSelectorSource<TRaw>(
    config: HttpSelectorSourceConfig<TRaw>,
): SelectorSource<TRaw> {
    const fetch = config.fetch ?? globalThis.fetch;
    const parameters = () => config.parameters?.() ?? {};

    const search = async (
        query: string,
        signal: AbortSignal,
    ): Promise<readonly SelectorOption<TRaw>[]> => {
        const response = await fetch(requestUrl(config.searchUrl, query, parameters()), { signal });
        const payload = await responseJson(response, 'search');
        if (!Array.isArray(payload)) throw new Error('Expected search response to be a list');
        return payload.map((raw) => config.mapOption(raw as TRaw));
    };

    const source: SelectorSource<TRaw> = { search };
    if (config.createUrl) {
        source.create = async (label: string, signal: AbortSignal): Promise<SelectorOption<TRaw>> => {
            const response = await fetch(config.createUrl!, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ...parameters(), Name: label }),
                signal,
            });
            const payload = await responseJson(response, 'create');
            if (!isSingleResponseValue(payload)) {
                throw new Error('Expected create response to be one value');
            }
            return config.mapOption(payload as TRaw);
        };
    }
    return source;
}

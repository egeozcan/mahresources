import type { SelectorOption, SelectorSource } from './types';

export interface InMemoryDeferred<T> {
    resolve(value: T): void;
    reject(error: unknown): void;
}

type Fixture<T> =
    | { readonly kind: 'success'; readonly value: T }
    | { readonly kind: 'failure'; readonly error: unknown }
    | { readonly kind: 'deferred'; readonly promise: Promise<T> };

function deferredFixture<T>(): { fixture: Fixture<T>; controller: InMemoryDeferred<T> } {
    let resolvePromise!: (value: T) => void;
    let rejectPromise!: (error: unknown) => void;
    const promise = new Promise<T>((resolve, reject) => {
        resolvePromise = resolve;
        rejectPromise = reject;
    });
    return {
        fixture: { kind: 'deferred', promise },
        controller: {
            resolve: resolvePromise,
            reject: rejectPromise,
        },
    };
}

function abortError(): DOMException {
    return new DOMException('The operation was aborted', 'AbortError');
}

function fixturePromise<T>(fixture: Fixture<T>, signal: AbortSignal): Promise<T> {
    if (signal.aborted) return Promise.reject(abortError());

    let operation: Promise<T>;
    switch (fixture.kind) {
        case 'success':
            operation = Promise.resolve(fixture.value);
            break;
        case 'failure':
            operation = Promise.reject(fixture.error);
            break;
        case 'deferred':
            operation = fixture.promise;
            break;
    }

    return new Promise<T>((resolve, reject) => {
        const onAbort = () => {
            cleanup();
            reject(abortError());
        };
        const cleanup = () => signal.removeEventListener('abort', onAbort);
        signal.addEventListener('abort', onAbort, { once: true });
        operation.then(
            (value) => {
                cleanup();
                resolve(value);
            },
            (error: unknown) => {
                cleanup();
                reject(error);
            },
        );
    });
}

/**
 * A deterministic selector source for unit tests and non-network consumers.
 * Fixtures are keyed by the exact query or creation label supplied by the caller.
 */
export class InMemorySelectorSource<TRaw = unknown> implements SelectorSource<TRaw> {
    private readonly searches = new Map<string, Fixture<readonly SelectorOption<TRaw>[]>>();
    private readonly creations = new Map<string, Fixture<SelectorOption<TRaw>>>();

    setSearchResult(query: string, options: readonly SelectorOption<TRaw>[]): void {
        this.searches.set(query, { kind: 'success', value: [...options] });
    }

    setSearchFailure(query: string, error: unknown): void {
        this.searches.set(query, { kind: 'failure', error });
    }

    deferSearch(query: string): InMemoryDeferred<readonly SelectorOption<TRaw>[]> {
        const { fixture, controller } = deferredFixture<readonly SelectorOption<TRaw>[]>();
        this.searches.set(query, fixture);
        return controller;
    }

    setCreateResult(label: string, option: SelectorOption<TRaw>): void {
        this.creations.set(label, { kind: 'success', value: option });
    }

    setCreateFailure(label: string, error: unknown): void {
        this.creations.set(label, { kind: 'failure', error });
    }

    deferCreate(label: string): InMemoryDeferred<SelectorOption<TRaw>> {
        const { fixture, controller } = deferredFixture<SelectorOption<TRaw>>();
        this.creations.set(label, fixture);
        return controller;
    }

    search(query: string, signal: AbortSignal): Promise<readonly SelectorOption<TRaw>[]> {
        const fixture = this.searches.get(query) ?? {
            kind: 'success' as const,
            value: [] as readonly SelectorOption<TRaw>[],
        };
        return fixturePromise(fixture, signal);
    }

    create(label: string, signal: AbortSignal): Promise<SelectorOption<TRaw>> {
        const fixture = this.creations.get(label);
        if (!fixture) {
            return Promise.reject(new Error(`No in-memory creation fixture for ${JSON.stringify(label)}`));
        }
        return fixturePromise(fixture, signal);
    }
}

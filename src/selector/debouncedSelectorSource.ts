import type { SelectorOption, SelectorSource } from './types';

interface ScheduledSearch<TRaw> {
    readonly timer: ReturnType<typeof setTimeout>;
    readonly signal: AbortSignal;
    readonly onAbort: () => void;
    readonly reject: (error: unknown) => void;
}

function abortError(): DOMException {
    return new DOMException('The operation was aborted', 'AbortError');
}

/**
 * Delays source lookups while preserving the selector core's immediate query state.
 */
export function createDebouncedSelectorSource<TRaw>(
    source: SelectorSource<TRaw>,
    delayMs: number,
): SelectorSource<TRaw> {
    let scheduled: ScheduledSearch<TRaw> | null = null;

    const cancelScheduled = (candidate: ScheduledSearch<TRaw>) => {
        clearTimeout(candidate.timer);
        candidate.signal.removeEventListener('abort', candidate.onAbort);
        if (scheduled === candidate) scheduled = null;
        candidate.reject(abortError());
    };

    const search = (query: string, signal: AbortSignal): Promise<readonly SelectorOption<TRaw>[]> => {
        if (scheduled) cancelScheduled(scheduled);

        return new Promise((resolve, reject) => {
            if (signal.aborted) {
                reject(abortError());
                return;
            }

            let candidate!: ScheduledSearch<TRaw>;
            const onAbort = () => cancelScheduled(candidate);
            const timer = setTimeout(() => {
                signal.removeEventListener('abort', onAbort);
                if (scheduled === candidate) scheduled = null;
                try {
                    Promise.resolve(source.search(query, signal)).then(resolve, reject);
                } catch (error) {
                    reject(error);
                }
            }, Math.max(0, delayMs));
            candidate = { timer, signal, onAbort, reject };
            scheduled = candidate;
            signal.addEventListener('abort', onAbort, { once: true });
        });
    };

    const debounced: SelectorSource<TRaw> = { search };
    if (source.create) {
        const create = source.create;
        debounced.create = (label, signal) => create(label, signal);
    }
    return debounced;
}

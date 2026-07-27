import type {
    SelectorChange,
    SelectorCommand,
    SelectorCommandResult,
    SelectorConfig,
    SelectorHandle,
    SelectorOption,
    SelectorState,
    SelectorSubscriber,
} from './types';

function freezeOption<TRaw>(option: SelectorOption<TRaw>): SelectorOption<TRaw> {
    return Object.freeze({
        key: option.key,
        label: option.label,
        raw: option.raw,
    });
}

function freezeOptions<TRaw>(
    options: readonly SelectorOption<TRaw>[],
): readonly SelectorOption<TRaw>[] {
    return Object.freeze(options.map(freezeOption));
}

function initialSnapshot<TRaw>(config: SelectorConfig<TRaw>): SelectorState<TRaw> {
    return Object.freeze({
        query: '',
        options: freezeOptions(config.options ?? []),
        selected: freezeOptions(config.selected ?? []),
        activeOptionIndex: null,
        isOpen: false,
        searchStatus: 'idle',
        creationStatus: 'idle',
        createCandidate: null,
        createConfirmationCandidate: null,
        error: null,
        destroyed: false,
    });
}

function withState<TRaw>(
    current: SelectorState<TRaw>,
    update: Partial<SelectorState<TRaw>>,
): SelectorState<TRaw> {
    return Object.freeze({ ...current, ...update });
}

class SelectorCore<TRaw> implements SelectorHandle<TRaw> {
    private snapshot: SelectorState<TRaw>;
    private readonly subscribers = new Set<SelectorSubscriber<TRaw>>();

    constructor(config: SelectorConfig<TRaw>) {
        this.snapshot = initialSnapshot(config);
    }

    getSnapshot(): SelectorState<TRaw> {
        return this.snapshot;
    }

    subscribe(subscriber: SelectorSubscriber<TRaw>): () => void {
        if (this.snapshot.destroyed) return () => undefined;
        this.subscribers.add(subscriber);
        return () => {
            this.subscribers.delete(subscriber);
        };
    }

    dispatch(_command: SelectorCommand<TRaw>): SelectorCommandResult<TRaw> {
        if (this.snapshot.destroyed) {
            return {
                ok: false,
                error: {
                    code: 'destroyed',
                    message: 'Selector has been destroyed',
                },
            };
        }
        return {
            ok: false,
            error: {
                code: 'unsupported-command',
                message: 'Selector command is not implemented',
            },
        };
    }

    destroy(): void {
        if (this.snapshot.destroyed) return;
        this.publish(withState(this.snapshot, { destroyed: true }));
        this.subscribers.clear();
    }

    private publish(snapshot: SelectorState<TRaw>, change?: SelectorChange<TRaw>): void {
        this.snapshot = snapshot;
        for (const subscriber of [...this.subscribers]) subscriber(snapshot, change);
    }
}

export function createSelector<TRaw = unknown>(
    config: SelectorConfig<TRaw>,
): SelectorHandle<TRaw> {
    return new SelectorCore(config);
}

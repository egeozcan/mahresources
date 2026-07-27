import type {
    SelectorChange,
    SelectorChangeReason,
    SelectorCommand,
    SelectorCommandResult,
    SelectorConfig,
    SelectorHandle,
    SelectorKey,
    SelectorOption,
    SelectorState,
    SelectorSubscriber,
} from './types';

function canonicalKey(key: SelectorKey): string {
    return String(key);
}

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

function normalizeSelection<TRaw>(
    options: readonly SelectorOption<TRaw>[],
    limit: number | null,
): readonly SelectorOption<TRaw>[] {
    const seen = new Set<string>();
    const unique: SelectorOption<TRaw>[] = [];
    for (const option of options) {
        const key = canonicalKey(option.key);
        if (seen.has(key)) continue;
        seen.add(key);
        unique.push(freezeOption(option));
    }
    if (limit === null) return Object.freeze(unique);
    if (limit === 0) return Object.freeze([]);
    return Object.freeze(unique.slice(-limit));
}

function initialSnapshot<TRaw>(
    config: SelectorConfig<TRaw>,
    selectionLimit: number | null,
): SelectorState<TRaw> {
    return Object.freeze({
        query: '',
        options: freezeOptions(config.options ?? []),
        selected: normalizeSelection(config.selected ?? [], selectionLimit),
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

function sameOptions<TRaw>(
    left: readonly SelectorOption<TRaw>[],
    right: readonly SelectorOption<TRaw>[],
): boolean {
    return left.length === right.length && left.every((option, index) => {
        const candidate = right[index];
        return canonicalKey(option.key) === canonicalKey(candidate.key)
            && option.label === candidate.label
            && option.raw === candidate.raw;
    });
}

function selectionChange<TRaw>(
    previous: readonly SelectorOption<TRaw>[],
    current: readonly SelectorOption<TRaw>[],
    reason: SelectorChangeReason,
): SelectorChange<TRaw> | undefined {
    const previousKeys = new Set(previous.map((option) => canonicalKey(option.key)));
    const currentKeys = new Set(current.map((option) => canonicalKey(option.key)));
    const added = Object.freeze(current.filter((option) => !previousKeys.has(canonicalKey(option.key))));
    const removed = Object.freeze(previous.filter((option) => !currentKeys.has(canonicalKey(option.key))));
    if (added.length === 0 && removed.length === 0) return undefined;
    return Object.freeze({ previous, current, added, removed, reason });
}

class SelectorCore<TRaw> implements SelectorHandle<TRaw> {
    private snapshot: SelectorState<TRaw>;
    private readonly subscribers = new Set<SelectorSubscriber<TRaw>>();
    private readonly selectionLimit: number | null;

    constructor(config: SelectorConfig<TRaw>) {
        this.selectionLimit = config.multiple === false
            ? 1
            : config.maxSelected === undefined
                ? null
                : Math.max(0, Math.floor(config.maxSelected));
        this.snapshot = initialSnapshot(config, this.selectionLimit);
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

    dispatch(command: SelectorCommand<TRaw>): SelectorCommandResult<TRaw> {
        if (this.snapshot.destroyed) {
            return {
                ok: false,
                error: {
                    code: 'destroyed',
                    message: 'Selector has been destroyed',
                },
            };
        }

        switch (command.type) {
            case 'open':
                return this.open();
            case 'close':
                return this.close();
            case 'move-active':
                return this.moveActive(command.direction);
            case 'select-option':
                return this.select(command.option);
            case 'remove-option':
                return this.remove(command.key);
            case 'replace-selection':
                return this.replace(
                    command.options,
                    command.reason ?? 'replace',
                    command.silent ?? false,
                );
            default:
                return {
                    ok: false,
                    error: {
                        code: 'unsupported-command',
                        message: 'Selector command is not implemented',
                    },
                };
        }
    }

    destroy(): void {
        if (this.snapshot.destroyed) return;
        this.publish(withState(this.snapshot, { destroyed: true }));
        this.subscribers.clear();
    }

    private open(): SelectorCommandResult<TRaw> {
        if (this.snapshot.isOpen) return { ok: true };
        this.publish(withState(this.snapshot, { isOpen: true }));
        return { ok: true };
    }

    private close(): SelectorCommandResult<TRaw> {
        if (!this.snapshot.isOpen && this.snapshot.activeOptionIndex === null) return { ok: true };
        this.publish(withState(this.snapshot, {
            isOpen: false,
            activeOptionIndex: null,
        }));
        return { ok: true };
    }

    private moveActive(direction: 'next' | 'previous'): SelectorCommandResult<TRaw> {
        const optionCount = this.snapshot.options.length;
        let activeOptionIndex: number | null = null;
        if (optionCount > 0) {
            const current = this.snapshot.activeOptionIndex;
            if (current === null) {
                activeOptionIndex = direction === 'next' ? 0 : optionCount - 1;
            } else if (direction === 'next') {
                activeOptionIndex = (current + 1) % optionCount;
            } else {
                activeOptionIndex = (current - 1 + optionCount) % optionCount;
            }
        }

        if (this.snapshot.isOpen && this.snapshot.activeOptionIndex === activeOptionIndex) {
            return { ok: true };
        }
        this.publish(withState(this.snapshot, { isOpen: true, activeOptionIndex }));
        return { ok: true };
    }

    private select(option: SelectorOption<TRaw>): SelectorCommandResult<TRaw> {
        const key = canonicalKey(option.key);
        if (this.snapshot.selected.some((selected) => canonicalKey(selected.key) === key)) {
            return { ok: true };
        }
        const current = normalizeSelection(
            [...this.snapshot.selected, option],
            this.selectionLimit,
        );
        return this.transitionSelection(current, 'select');
    }

    private remove(key: SelectorKey): SelectorCommandResult<TRaw> {
        const canonical = canonicalKey(key);
        if (!this.snapshot.selected.some((option) => canonicalKey(option.key) === canonical)) {
            return { ok: true };
        }
        const current = Object.freeze(
            this.snapshot.selected.filter((option) => canonicalKey(option.key) !== canonical),
        );
        return this.transitionSelection(current, 'remove');
    }

    private replace(
        options: readonly SelectorOption<TRaw>[],
        reason: 'replace' | 'reset',
        silent: boolean,
    ): SelectorCommandResult<TRaw> {
        const current = normalizeSelection(options, this.selectionLimit);
        if (sameOptions(this.snapshot.selected, current)) return { ok: true };
        return this.transitionSelection(current, reason, silent);
    }

    private transitionSelection(
        current: readonly SelectorOption<TRaw>[],
        reason: SelectorChangeReason,
        silent = false,
    ): SelectorCommandResult<TRaw> {
        const previous = this.snapshot.selected;
        const nextSnapshot = withState(this.snapshot, { selected: current });
        if (silent) {
            this.publish(nextSnapshot);
            return { ok: true };
        }
        const change = selectionChange(previous, current, reason);
        this.publish(nextSnapshot, change);
        return change ? { ok: true, change } : { ok: true };
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

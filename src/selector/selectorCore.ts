import type {
    SelectorChange,
    SelectorChangeReason,
    SelectorCommand,
    SelectorCommandResult,
    SelectorConfig,
    SelectorCreationError,
    SelectorCreationOutcome,
    SelectorCreationRequest,
    SelectorHandle,
    SelectorKey,
    SelectorOption,
    SelectorState,
    SelectorSubscriber,
    SelectorSource,
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
        searchError: null,
        creationError: null,
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
        return Object.is(option.key, candidate.key)
            && option.label === candidate.label
            && option.raw === candidate.raw;
    });
}

function isAbortError(error: unknown): boolean {
    return typeof error === 'object'
        && error !== null
        && 'name' in error
        && error.name === 'AbortError';
}

function errorMessage(error: unknown, fallback: string): string {
    return error instanceof Error && error.message ? error.message : fallback;
}

interface CreationQueueEntry<TRaw> {
    readonly label: string;
    readonly request: SelectorCreationRequest<TRaw>;
    readonly resolve: (outcome: SelectorCreationOutcome<TRaw>) => void;
}

function creationQueueEntry<TRaw>(label: string): CreationQueueEntry<TRaw> {
    let resolve!: (outcome: SelectorCreationOutcome<TRaw>) => void;
    const outcome = new Promise<SelectorCreationOutcome<TRaw>>((resolvePromise) => {
        resolve = resolvePromise;
    });
    return {
        label,
        request: Object.freeze({ label, outcome }),
        resolve,
    };
}

function selectionChange<TRaw>(
    previous: readonly SelectorOption<TRaw>[],
    current: readonly SelectorOption<TRaw>[],
    reason: SelectorChangeReason,
): SelectorChange<TRaw> {
    const previousKeys = new Set(previous.map((option) => canonicalKey(option.key)));
    const currentKeys = new Set(current.map((option) => canonicalKey(option.key)));
    const added = Object.freeze(current.filter((option) => !previousKeys.has(canonicalKey(option.key))));
    const removed = Object.freeze(previous.filter((option) => !currentKeys.has(canonicalKey(option.key))));
    return Object.freeze({ previous, current, added, removed, reason });
}

class SelectorCore<TRaw> implements SelectorHandle<TRaw> {
    private snapshot: SelectorState<TRaw>;
    private readonly subscribers = new Set<SelectorSubscriber<TRaw>>();
    private readonly selectionLimit: number | null;
    private readonly source: SelectorSource<TRaw>;
    private searchController: AbortController | null = null;
    private searchGeneration = 0;
    private creationController: AbortController | null = null;
    private currentCreationEntry: CreationQueueEntry<TRaw> | null = null;
    private readonly creationQueue: CreationQueueEntry<TRaw>[] = [];
    private readonly completedCreationLabels = new Set<string>();

    constructor(config: SelectorConfig<TRaw>) {
        this.source = config.source;
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
            case 'set-query':
                return this.setQuery(command.query);
            case 'open':
                return this.open();
            case 'close':
                return this.close();
            case 'move-active':
                return this.moveActive(command.direction);
            case 'set-active':
                return this.setActive(command.index);
            case 'commit-active':
                return this.commitActive();
            case 'commit-token':
                return this.commitToken(command.token);
            case 'request-create-confirmation':
                return this.requestCreateConfirmation(command.label);
            case 'confirm-create':
                return this.confirmCreate();
            case 'cancel-create-confirmation':
                return this.cancelCreateConfirmation();
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
        this.searchGeneration += 1;
        this.searchController?.abort();
        this.searchController = null;

        const activeCreation = this.currentCreationEntry;
        const discardedCreations = this.creationQueue.splice(0);
        this.currentCreationEntry = null;
        this.creationController?.abort();
        this.creationController = null;
        activeCreation?.resolve(Object.freeze({
            status: 'cancelled',
            label: activeCreation.label,
        }));
        for (const entry of discardedCreations) {
            entry.resolve(Object.freeze({ status: 'discarded', label: entry.label }));
        }

        this.publish(withState(this.snapshot, {
            destroyed: true,
            creationStatus: 'idle',
            createCandidate: null,
            createConfirmationCandidate: null,
        }));
        this.subscribers.clear();
    }

    private setQuery(query: string): SelectorCommandResult<TRaw> {
        this.searchGeneration += 1;
        const generation = this.searchGeneration;
        this.searchController?.abort();
        const controller = new AbortController();
        this.searchController = controller;
        this.publish(withState(this.snapshot, {
            query,
            activeOptionIndex: null,
            searchStatus: 'loading',
            createCandidate: null,
            searchError: null,
        }));

        let pending: Promise<readonly SelectorOption<TRaw>[]>;
        try {
            pending = this.source.search(query, controller.signal);
        } catch (error) {
            pending = Promise.reject(error);
        }
        void pending.then(
            (options) => this.applySearchSuccess(generation, query, controller, options),
            (error: unknown) => this.applySearchFailure(generation, query, controller, error),
        );
        return { ok: true };
    }

    private applySearchSuccess(
        generation: number,
        query: string,
        controller: AbortController,
        results: readonly SelectorOption<TRaw>[],
    ): void {
        if (!this.isCurrentSearch(generation, query, controller)) return;
        const selectedKeys = new Set(
            this.snapshot.selected.map((option) => canonicalKey(option.key)),
        );
        const options = freezeOptions(
            results.filter((option) => !selectedKeys.has(canonicalKey(option.key))),
        );
        const trimmedQuery = query.trim();
        const hasExactMatch = results.some((option) => option.label === trimmedQuery)
            || this.snapshot.selected.some((option) => option.label === trimmedQuery);
        const createCandidate = this.source.create && trimmedQuery && !hasExactMatch
            ? Object.freeze({ label: trimmedQuery })
            : null;
        this.publish(withState(this.snapshot, {
            options,
            activeOptionIndex: null,
            searchStatus: 'success',
            createCandidate,
            searchError: null,
        }));
    }

    private applySearchFailure(
        generation: number,
        query: string,
        controller: AbortController,
        error: unknown,
    ): void {
        if (!this.isCurrentSearch(generation, query, controller)) return;
        if (controller.signal.aborted || isAbortError(error)) {
            this.publish(withState(this.snapshot, { searchStatus: 'idle', searchError: null }));
            return;
        }
        this.publish(withState(this.snapshot, {
            searchStatus: 'error',
            searchError: Object.freeze({
                operation: 'search',
                message: errorMessage(error, 'Search failed'),
                cause: error,
            }),
        }));
    }

    private isCurrentSearch(
        generation: number,
        query: string,
        controller: AbortController,
    ): boolean {
        return !this.snapshot.destroyed
            && this.searchGeneration === generation
            && this.snapshot.query === query
            && this.searchController === controller;
    }

    private commitToken(token: string): SelectorCommandResult<TRaw> {
        const label = token.trim();
        if (!label) return { ok: true, consumed: false };
        if (this.snapshot.selected.some((option) => option.label === label)) {
            return { ok: true, consumed: true };
        }
        const available = this.snapshot.options.find((option) => option.label === label);
        if (available) {
            const result = this.select(available);
            return result.ok ? { ...result, consumed: true } : result;
        }
        if (!this.source.create) return { ok: true, consumed: false };
        return this.enqueueCreation(label);
    }

    private enqueueCreation(label: string): SelectorCommandResult<TRaw> {
        const canonical = label.trim();
        if (!canonical || !this.source.create) return { ok: true, consumed: false };
        if (this.snapshot.selected.some((option) => option.label.trim() === canonical)
            || this.completedCreationLabels.has(canonical)) {
            return { ok: true, consumed: true };
        }

        const existing = this.currentCreationEntry?.label === canonical
            ? this.currentCreationEntry
            : this.creationQueue.find((queued) => queued.label === canonical);
        if (existing) {
            return { ok: true, consumed: true, creation: existing.request };
        }

        const entry = creationQueueEntry<TRaw>(canonical);
        this.creationQueue.push(entry);
        this.consumeQueryForCreation();
        this.processCreationQueue();
        return { ok: true, consumed: true, creation: entry.request };
    }

    private consumeQueryForCreation(): void {
        this.searchGeneration += 1;
        this.searchController?.abort();
        this.searchController = null;
        this.publish(withState(this.snapshot, {
            query: '',
            activeOptionIndex: null,
            searchStatus: this.snapshot.searchStatus === 'loading'
                ? 'idle'
                : this.snapshot.searchStatus,
            creationStatus: 'loading',
            createCandidate: null,
            creationError: null,
        }));
    }

    private processCreationQueue(): void {
        if (this.creationController || this.snapshot.destroyed) return;
        const entry = this.creationQueue.shift();
        if (!entry || !this.source.create) {
            if (!this.snapshot.destroyed && this.snapshot.creationStatus === 'loading') {
                this.publish(withState(this.snapshot, { creationStatus: 'idle' }));
            }
            return;
        }

        const controller = new AbortController();
        this.creationController = controller;
        this.currentCreationEntry = entry;
        if (this.snapshot.creationStatus !== 'loading' || this.snapshot.creationError) {
            this.publish(withState(this.snapshot, {
                creationStatus: 'loading',
                creationError: null,
            }));
        }
        let pending: Promise<SelectorOption<TRaw>>;
        try {
            pending = this.source.create(entry.label, controller.signal);
        } catch (error) {
            pending = Promise.reject(error);
        }
        void pending.then(
            (option) => this.applyCreateSuccess(entry, controller, option),
            (error: unknown) => this.applyCreateFailure(entry, controller, error),
        );
    }

    private applyCreateSuccess(
        entry: CreationQueueEntry<TRaw>,
        controller: AbortController,
        option: SelectorOption<TRaw>,
    ): void {
        if (!this.isCurrentCreation(entry, controller)) return;
        this.creationController = null;
        this.currentCreationEntry = null;
        this.completedCreationLabels.add(entry.label);
        const createdOption = freezeOption(option);
        const current = normalizeSelection(
            [...this.snapshot.selected, createdOption],
            this.selectionLimit,
        );
        const hasSelectionChange = !sameOptions(this.snapshot.selected, current);
        if (hasSelectionChange) {
            this.transitionSelection(current, 'create', false, {
                creationStatus: this.creationQueue.length ? 'loading' : 'idle',
                createConfirmationCandidate: null,
                creationError: null,
            });
        } else {
            this.publish(withState(this.snapshot, {
                creationStatus: this.creationQueue.length ? 'loading' : 'idle',
                createConfirmationCandidate: null,
                creationError: null,
            }));
        }
        entry.resolve(Object.freeze({
            status: 'success',
            label: entry.label,
            option: createdOption,
        }));
        this.processCreationQueue();
    }

    private applyCreateFailure(
        entry: CreationQueueEntry<TRaw>,
        controller: AbortController,
        error: unknown,
    ): void {
        if (!this.isCurrentCreation(entry, controller)) return;
        this.creationController = null;
        this.currentCreationEntry = null;
        if (controller.signal.aborted || isAbortError(error)) {
            this.publish(withState(this.snapshot, {
                creationStatus: this.creationQueue.length ? 'loading' : 'idle',
                creationError: null,
            }));
            entry.resolve(Object.freeze({ status: 'cancelled', label: entry.label }));
            this.processCreationQueue();
            return;
        }
        const creationError: SelectorCreationError = Object.freeze({
            operation: 'create',
            message: errorMessage(error, 'Creation failed'),
            cause: error,
        });
        this.publish(withState(this.snapshot, {
            creationStatus: 'error',
            creationError,
        }));
        entry.resolve(Object.freeze({
            status: 'failure',
            label: entry.label,
            error: creationError,
        }));
        this.processCreationQueue();
    }

    private isCurrentCreation(
        entry: CreationQueueEntry<TRaw>,
        controller: AbortController,
    ): boolean {
        return !this.snapshot.destroyed
            && this.creationController === controller
            && this.currentCreationEntry === entry;
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

    private requestCreateConfirmation(label?: string): SelectorCommandResult<TRaw> {
        if (!this.source.create) return { ok: true, consumed: false };
        const candidate = label === undefined
            ? this.snapshot.createCandidate?.label
            : label.trim();
        if (!candidate) return { ok: true, consumed: false };
        this.publish(withState(this.snapshot, {
            createConfirmationCandidate: Object.freeze({ label: candidate }),
        }));
        return { ok: true, consumed: true };
    }

    private confirmCreate(): SelectorCommandResult<TRaw> {
        const candidate = this.snapshot.createConfirmationCandidate;
        if (!candidate) return { ok: true, consumed: false };
        return this.enqueueCreation(candidate.label);
    }

    private cancelCreateConfirmation(): SelectorCommandResult<TRaw> {
        if (!this.snapshot.createConfirmationCandidate) return { ok: true, consumed: false };
        this.publish(withState(this.snapshot, { createConfirmationCandidate: null }));
        return { ok: true, consumed: true };
    }

    private commitActive(): SelectorCommandResult<TRaw> {
        const active = this.snapshot.activeOptionIndex;
        if (active === null) return { ok: true, consumed: false };
        const option = this.snapshot.options[active];
        if (option) {
            const result = this.select(option);
            return result.ok ? { ...result, consumed: true } : result;
        }
        if (active === this.snapshot.options.length && this.snapshot.createCandidate) {
            return this.enqueueCreation(this.snapshot.createCandidate.label);
        }
        return { ok: true, consumed: false };
    }

    private moveActive(direction: 'next' | 'previous'): SelectorCommandResult<TRaw> {
        const optionCount = this.optionCount();
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
        return this.setActive(activeOptionIndex);
    }

    private setActive(index: number | null): SelectorCommandResult<TRaw> {
        const optionCount = this.optionCount();
        const activeOptionIndex = index !== null && index >= 0 && index < optionCount
            ? index
            : null;
        if (this.snapshot.isOpen && this.snapshot.activeOptionIndex === activeOptionIndex) {
            return { ok: true };
        }
        this.publish(withState(this.snapshot, { isOpen: true, activeOptionIndex }));
        return { ok: true };
    }

    private optionCount(): number {
        return this.snapshot.options.length + (this.snapshot.createCandidate ? 1 : 0);
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
        const state = this.snapshot.createConfirmationCandidate
            ? { createConfirmationCandidate: null }
            : {};
        if (sameOptions(this.snapshot.selected, current)) {
            if (this.snapshot.createConfirmationCandidate) {
                this.publish(withState(this.snapshot, state));
            }
            return { ok: true };
        }
        return this.transitionSelection(current, reason, silent, state);
    }

    private transitionSelection(
        current: readonly SelectorOption<TRaw>[],
        reason: SelectorChangeReason,
        silent = false,
        state: Partial<SelectorState<TRaw>> = {},
    ): SelectorCommandResult<TRaw> {
        const previous = this.snapshot.selected;
        if (sameOptions(previous, current)) return { ok: true };
        const candidateWasSelected = this.snapshot.createCandidate
            && current.some((option) => option.label === this.snapshot.createCandidate?.label);
        const nextSnapshot = withState(this.snapshot, {
            ...state,
            ...(candidateWasSelected && !('createCandidate' in state)
                ? { createCandidate: null }
                : {}),
            selected: current,
        });
        if (silent) {
            this.publish(nextSnapshot);
            return { ok: true };
        }
        const change = selectionChange(previous, current, reason);
        this.publish(nextSnapshot, change);
        return { ok: true, change };
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

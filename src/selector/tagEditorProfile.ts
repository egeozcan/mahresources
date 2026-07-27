import {
    createTagFieldProfile,
    type EntityFieldProfile,
    type SelectorEntityValue,
    type TagFieldProfileOptions,
} from './entityFieldProfiles';
import type {
    SelectorChange,
    SelectorCommand,
    SelectorCommandResult,
    SelectorHandle,
    SelectorKey,
    SelectorOption,
    SelectorState,
    SelectorSubscriber,
} from './types';

const DEFAULT_FAILED_KEY_DURATION_MS = 400;

type AssociationAction = 'add' | 'remove';

/**
 * Persistence belongs to the owning domain module. The profile only coordinates optimistic
 * selection state; an adapter normally closes over the current resource, note, or group.
 */
export interface TagAssociationPersistenceAdapter<TRaw extends SelectorEntityValue> {
    add(tag: TRaw, signal: AbortSignal): Promise<void>;
    remove(tag: TRaw, signal: AbortSignal): Promise<void>;
}

export interface TagEditorProfileOptions<TRaw extends SelectorEntityValue>
    extends TagFieldProfileOptions<TRaw> {
    readonly association: TagAssociationPersistenceAdapter<TRaw>;
    /** How long a failed association remains available for chip-level presentation. */
    readonly failedKeyDurationMs?: number;
}

export interface TagEditorProfileState {
    /** Canonical string keys whose optimistic association writes are in flight. */
    readonly pendingKeys: readonly string[];
    /** Canonical string keys whose most recent association write failed. */
    readonly failedKeys: readonly string[];
    readonly destroyed: boolean;
}

export type TagEditorProfileSubscriber = (snapshot: TagEditorProfileState) => void;

export interface TagEditorProfile<TRaw extends SelectorEntityValue>
    extends EntityFieldProfile<TRaw> {
    getSnapshot(): TagEditorProfileState;
    subscribe(subscriber: TagEditorProfileSubscriber): () => void;
    /**
     * Reconciles the selection with associations the owning domain changed elsewhere on the
     * SAME entity (another panel applied a tag, an optimistic cache write landed). Unlike a
     * `replace-selection` reset, which assumes the underlying entity changed and therefore
     * invalidates every in-flight write, this only invalidates the keys it actually moves —
     * so a concurrent write to an untouched key keeps its pending presentation and rollback.
     */
    syncAssociations(options: readonly SelectorOption<TRaw>[]): void;
    /** Aborts profile-owned writes, clears transient presentation state, and destroys the selector. */
    destroy(): void;
}

interface AssociationOperation<TRaw extends SelectorEntityValue> {
    readonly key: string;
    readonly version: number;
    readonly action: AssociationAction;
    readonly option: SelectorOption<TRaw>;
    readonly controller: AbortController;
}

function canonicalKey(key: SelectorKey): string {
    return String(key);
}

function selectedByKey<TRaw extends SelectorEntityValue>(
    selected: readonly SelectorOption<TRaw>[],
): Map<string, SelectorOption<TRaw>> {
    return new Map(selected.map((option) => [canonicalKey(option.key), option]));
}

function failureDuration(value: number | undefined): number {
    if (value === undefined) return DEFAULT_FAILED_KEY_DURATION_MS;
    return Math.max(0, Math.floor(value));
}

class TagEditorProfileImpl<TRaw extends SelectorEntityValue> implements TagEditorProfile<TRaw> {
    readonly selector: EntityFieldProfile<TRaw>['selector'];
    readonly form: EntityFieldProfile<TRaw>['form'];
    readonly interaction: EntityFieldProfile<TRaw>['interaction'];
    readonly presentation: EntityFieldProfile<TRaw>['presentation'];

    private readonly coreSelector: SelectorHandle<TRaw>;
    private readonly association: TagAssociationPersistenceAdapter<TRaw>;
    private readonly failedKeyDurationMs: number;
    private readonly subscribers = new Set<TagEditorProfileSubscriber>();
    private readonly pendingKeys = new Set<string>();
    private readonly failedKeys = new Set<string>();
    private readonly failureTimers = new Map<string, ReturnType<typeof setTimeout>>();
    private readonly operations = new Map<string, AssociationOperation<TRaw>>();
    private readonly versions = new Map<string, number>();
    private selected: readonly SelectorOption<TRaw>[];
    private unsubscribeSelector: (() => void) | null = null;
    private destroyed = false;

    constructor(options: TagEditorProfileOptions<TRaw>) {
        const { association, failedKeyDurationMs, ...tagFieldOptions } = options;
        const field = createTagFieldProfile(tagFieldOptions);
        this.coreSelector = field.selector;
        this.selector = Object.freeze({
            getSnapshot: () => this.coreSelector.getSnapshot(),
            subscribe: (subscriber: SelectorSubscriber<TRaw>) =>
                this.coreSelector.subscribe(subscriber),
            dispatch: (command: SelectorCommand<TRaw>) => this.dispatchSelectorCommand(command),
            destroy: () => this.coreSelector.destroy(),
        });
        this.form = field.form;
        this.interaction = field.interaction;
        this.presentation = field.presentation;
        this.association = association;
        this.failedKeyDurationMs = failureDuration(failedKeyDurationMs);
        this.selected = this.coreSelector.getSnapshot().selected;
        this.unsubscribeSelector = this.coreSelector.subscribe((snapshot, change) => {
            this.handleSelectorSnapshot(snapshot, change);
        });
    }

    getSnapshot(): TagEditorProfileState {
        return this.profileSnapshot();
    }

    subscribe(subscriber: TagEditorProfileSubscriber): () => void {
        if (this.destroyed) return () => undefined;
        this.subscribers.add(subscriber);
        return () => this.subscribers.delete(subscriber);
    }

    destroy(): void {
        this.dispose(true);
    }

    syncAssociations(options: readonly SelectorOption<TRaw>[]): void {
        if (this.destroyed) return;
        // Bypasses dispatchSelectorCommand deliberately: the per-key membership diff in
        // handleSelectorSnapshot invalidates exactly the keys this moves, and a silent
        // transition carries no change, so nothing is persisted back.
        this.coreSelector.dispatch({
            type: 'replace-selection',
            options,
            reason: 'reset',
            silent: true,
        });
    }

    private dispatchSelectorCommand(
        command: SelectorCommand<TRaw>,
    ): SelectorCommandResult<TRaw> {
        if (command.type === 'replace-selection') {
            for (const key of [...this.operations.keys()]) {
                this.advanceVersion(key);
                this.invalidateOperation(key);
            }
        }
        return this.coreSelector.dispatch(command);
    }

    private handleSelectorSnapshot(
        snapshot: SelectorState<TRaw>,
        change?: SelectorChange<TRaw>,
    ): void {
        if (this.destroyed) return;
        if (snapshot.destroyed) {
            this.dispose(false);
            return;
        }

        const previous = selectedByKey(this.selected);
        const current = selectedByKey(snapshot.selected);
        const changedKeys = new Set([...previous.keys(), ...current.keys()]);
        for (const key of changedKeys) {
            if (previous.has(key) === current.has(key)) continue;
            this.advanceVersion(key);
            this.invalidateOperation(key);
        }
        this.selected = snapshot.selected;

        if (!change) return;
        for (const option of change.added) this.persist('add', option);
        for (const option of change.removed) this.persist('remove', option);
    }

    private persist(action: AssociationAction, option: SelectorOption<TRaw>): void {
        const key = canonicalKey(option.key);
        const existing = this.operations.get(key);
        if (existing?.action === action) return;
        this.invalidateOperation(key);
        this.clearFailure(key);

        const operation: AssociationOperation<TRaw> = {
            key,
            version: this.versions.get(key) ?? 0,
            action,
            option,
            controller: new AbortController(),
        };
        this.operations.set(key, operation);
        this.setPending(key, true);

        let request: Promise<void>;
        try {
            request = this.association[action](option.raw, operation.controller.signal);
        } catch (error) {
            request = Promise.reject(error);
        }
        void Promise.resolve(request).then(
            () => this.completeSuccess(operation),
            (error: unknown) => this.completeFailure(operation, error),
        );
    }

    private completeSuccess(operation: AssociationOperation<TRaw>): void {
        if (!this.isCurrent(operation)) return;
        this.operations.delete(operation.key);
        this.setPending(operation.key, false);
        this.clearFailure(operation.key);
    }

    private completeFailure(operation: AssociationOperation<TRaw>, _error: unknown): void {
        if (!this.isCurrent(operation)) return;
        this.operations.delete(operation.key);
        this.setPending(operation.key, false);
        this.markFailed(operation.key);
        this.rollback(operation);
    }

    private rollback(operation: AssociationOperation<TRaw>): void {
        if (this.destroyed || this.versions.get(operation.key) !== operation.version) return;
        const current = this.coreSelector.getSnapshot().selected;
        const hasKey = current.some((option) => canonicalKey(option.key) === operation.key);
        const options = operation.action === 'add'
            ? current.filter((option) => canonicalKey(option.key) !== operation.key)
            : hasKey
                ? current
                : [...current, operation.option];
        if (options === current || (operation.action === 'add' && options.length === current.length)) {
            return;
        }
        this.coreSelector.dispatch({
            type: 'replace-selection',
            options,
            reason: 'replace',
            silent: true,
        });
    }

    private isCurrent(operation: AssociationOperation<TRaw>): boolean {
        return !this.destroyed
            && this.operations.get(operation.key) === operation
            && this.versions.get(operation.key) === operation.version;
    }

    private invalidateOperation(key: string): void {
        const operation = this.operations.get(key);
        if (!operation) return;
        this.operations.delete(key);
        operation.controller.abort();
        this.setPending(key, false);
    }

    private advanceVersion(key: string): void {
        this.versions.set(key, (this.versions.get(key) ?? 0) + 1);
    }

    private setPending(key: string, pending: boolean): void {
        const changed = pending ? !this.pendingKeys.has(key) : this.pendingKeys.delete(key);
        if (!changed) return;
        if (pending) this.pendingKeys.add(key);
        this.publish();
    }

    private markFailed(key: string): void {
        this.failedKeys.add(key);
        const existingTimer = this.failureTimers.get(key);
        if (existingTimer) clearTimeout(existingTimer);
        const timer = setTimeout(() => {
            if (this.failureTimers.get(key) !== timer || this.destroyed) return;
            this.failureTimers.delete(key);
            if (this.failedKeys.delete(key)) this.publish();
        }, this.failedKeyDurationMs);
        this.failureTimers.set(key, timer);
        this.publish();
    }

    private clearFailure(key: string): void {
        const timer = this.failureTimers.get(key);
        if (timer) clearTimeout(timer);
        this.failureTimers.delete(key);
        if (this.failedKeys.delete(key)) this.publish();
    }

    private profileSnapshot(): TagEditorProfileState {
        return Object.freeze({
            pendingKeys: Object.freeze([...this.pendingKeys].sort()),
            failedKeys: Object.freeze([...this.failedKeys].sort()),
            destroyed: this.destroyed,
        });
    }

    private publish(): void {
        const snapshot = this.profileSnapshot();
        for (const subscriber of [...this.subscribers]) subscriber(snapshot);
    }

    private dispose(destroySelector: boolean): void {
        if (this.destroyed) return;
        this.destroyed = true;
        this.unsubscribeSelector?.();
        this.unsubscribeSelector = null;
        for (const operation of this.operations.values()) operation.controller.abort();
        this.operations.clear();
        this.pendingKeys.clear();
        for (const timer of this.failureTimers.values()) clearTimeout(timer);
        this.failureTimers.clear();
        this.failedKeys.clear();
        this.publish();
        this.subscribers.clear();
        if (destroySelector) this.coreSelector.destroy();
    }
}

/**
 * Creates a creatable tag field whose normal selection commands optimistically persist tag
 * associations. The owning domain supplies the association adapter; source/core details stay
 * encapsulated by the tag-field profile.
 */
export function createTagEditorProfile<TRaw extends SelectorEntityValue>(
    options: TagEditorProfileOptions<TRaw>,
): TagEditorProfile<TRaw> {
    return new TagEditorProfileImpl(options);
}

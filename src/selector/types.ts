export type SelectorKey = string | number;

export interface SelectorOption<TRaw = unknown> {
    readonly key: SelectorKey;
    readonly label: string;
    readonly raw: TRaw;
}

export type SelectorSearchStatus = 'idle' | 'loading' | 'success' | 'error';
export type SelectorCreationStatus = 'idle' | 'loading' | 'error';

export interface SelectorCreateCandidate {
    readonly label: string;
}

export interface SelectorError {
    readonly operation: 'search' | 'create';
    readonly message: string;
    readonly cause?: unknown;
}

export interface SelectorState<TRaw = unknown> {
    readonly query: string;
    readonly options: readonly SelectorOption<TRaw>[];
    readonly selected: readonly SelectorOption<TRaw>[];
    readonly activeOptionIndex: number | null;
    readonly isOpen: boolean;
    readonly searchStatus: SelectorSearchStatus;
    readonly creationStatus: SelectorCreationStatus;
    readonly createCandidate: SelectorCreateCandidate | null;
    readonly createConfirmationCandidate: SelectorCreateCandidate | null;
    readonly error: SelectorError | null;
    readonly destroyed: boolean;
}

export type SelectorChangeReason = 'select' | 'remove' | 'create' | 'replace' | 'reset';

export interface SelectorChange<TRaw = unknown> {
    readonly previous: readonly SelectorOption<TRaw>[];
    readonly current: readonly SelectorOption<TRaw>[];
    readonly added: readonly SelectorOption<TRaw>[];
    readonly removed: readonly SelectorOption<TRaw>[];
    readonly reason: SelectorChangeReason;
}

export type SelectorCommand<TRaw = unknown> =
    | { readonly type: 'set-query'; readonly query: string }
    | { readonly type: 'open' }
    | { readonly type: 'close' }
    | { readonly type: 'move-active'; readonly direction: 'next' | 'previous' }
    | { readonly type: 'commit-active' }
    | { readonly type: 'commit-token'; readonly token: string }
    | { readonly type: 'select-option'; readonly option: SelectorOption<TRaw> }
    | { readonly type: 'remove-option'; readonly key: SelectorKey }
    | {
        readonly type: 'replace-selection';
        readonly options: readonly SelectorOption<TRaw>[];
        readonly silent?: boolean;
        readonly reason?: 'replace' | 'reset';
    }
    | { readonly type: 'request-create-confirmation'; readonly label?: string }
    | { readonly type: 'confirm-create' }
    | { readonly type: 'cancel-create-confirmation' };

export interface SelectorCommandError {
    readonly code: 'destroyed' | 'unsupported-command';
    readonly message: string;
}

export type SelectorCommandResult<TRaw = unknown> =
    | { readonly ok: true; readonly change?: SelectorChange<TRaw> }
    | { readonly ok: false; readonly error: SelectorCommandError };

export interface SelectorSource<TRaw = unknown> {
    search(query: string, signal: AbortSignal): Promise<readonly SelectorOption<TRaw>[]>;
    create?(label: string, signal: AbortSignal): Promise<SelectorOption<TRaw>>;
}

export interface SelectorConfig<TRaw = unknown> {
    readonly source: SelectorSource<TRaw>;
    readonly options?: readonly SelectorOption<TRaw>[];
    readonly selected?: readonly SelectorOption<TRaw>[];
    readonly multiple?: boolean;
    readonly maxSelected?: number;
}

export type SelectorSubscriber<TRaw = unknown> = (
    snapshot: SelectorState<TRaw>,
    change?: SelectorChange<TRaw>,
) => void;

export interface SelectorHandle<TRaw = unknown> {
    getSnapshot(): SelectorState<TRaw>;
    subscribe(subscriber: SelectorSubscriber<TRaw>): () => void;
    dispatch(command: SelectorCommand<TRaw>): SelectorCommandResult<TRaw>;
    destroy(): void;
}

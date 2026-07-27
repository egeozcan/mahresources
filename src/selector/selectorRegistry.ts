export type SelectorKey = string | number;

export interface SelectorValue {
    ID: SelectorKey;
    Name?: string;
    [property: string]: unknown;
}

export interface SelectorReplacementOptions {
    silent?: boolean;
}

/**
 * The narrow integration surface used by callers that must synchronize with a selector
 * without depending on its Alpine state or DOM declaration.
 */
export interface SelectorIntegrationHandle {
    getRawValues(): readonly SelectorValue[];
    replaceRawValues(
        values: readonly SelectorValue[],
        options?: SelectorReplacementOptions,
    ): void;
    replaceByKeys(
        keys: readonly SelectorKey[],
        options?: SelectorReplacementOptions,
    ): void;
    resolveExactLabels(
        labels: readonly string[],
        options?: SelectorReplacementOptions,
    ): Promise<boolean>;
}

/**
 * One atomic selection change, as seen by another module in the same form. `values` is the
 * resulting selection; `added`/`removed` describe only what moved, so a replacement reports
 * its removal and its addition in a single notification rather than as two events.
 */
export interface SelectorFieldChange {
    readonly values: readonly SelectorValue[];
    readonly added: readonly SelectorValue[];
    readonly removed: readonly SelectorValue[];
}

export type SelectorFieldChangeListener = (change: SelectorFieldChange) => void;

/**
 * Stores selector handles under the identity of their owning form and field name.
 * Weak form keys allow detached forms to be collected even if their Alpine cleanup did
 * not run, while the returned cleanup function handles the normal lifecycle explicitly.
 */
export class SelectorRegistry {
    private readonly forms = new WeakMap<HTMLFormElement, Map<string, SelectorIntegrationHandle>>();

    private readonly observers = new WeakMap<
        HTMLFormElement,
        Map<string, Set<SelectorFieldChangeListener>>
    >();

    register(
        form: HTMLFormElement,
        fieldName: string,
        handle: SelectorIntegrationHandle,
    ): () => void {
        let selectors = this.forms.get(form);
        if (!selectors) {
            selectors = new Map();
            this.forms.set(form, selectors);
        }
        selectors.set(fieldName, handle);

        return () => {
            const current = this.forms.get(form);
            if (current?.get(fieldName) !== handle) return;
            current.delete(fieldName);
            if (current.size === 0) this.forms.delete(form);
        };
    }

    get(form: HTMLFormElement, fieldName: string): SelectorIntegrationHandle | undefined {
        return this.forms.get(form)?.get(fieldName);
    }

    /**
     * Subscribes to atomic changes of one named field within one form. Observation is
     * independent of registration, so a consumer rendered before or after its selector
     * gets the same result and no module has to reason about Alpine initialization order.
     */
    observe(
        form: HTMLFormElement,
        fieldName: string,
        listener: SelectorFieldChangeListener,
    ): () => void {
        let fields = this.observers.get(form);
        if (!fields) {
            fields = new Map();
            this.observers.set(form, fields);
        }
        let listeners = fields.get(fieldName);
        if (!listeners) {
            listeners = new Set();
            fields.set(fieldName, listeners);
        }
        listeners.add(listener);

        return () => {
            const currentFields = this.observers.get(form);
            const current = currentFields?.get(fieldName);
            if (!current?.delete(listener)) return;
            if (current.size === 0) currentFields!.delete(fieldName);
            if (currentFields!.size === 0) this.observers.delete(form);
        };
    }

    /**
     * Publishes one atomic change to the field's observers. Delivery is synchronous and
     * isolated: a throwing observer is reported and skipped so the rest still see the change.
     */
    notifyChange(form: HTMLFormElement, fieldName: string, change: SelectorFieldChange): void {
        const listeners = this.observers.get(form)?.get(fieldName);
        if (!listeners?.size) return;
        for (const listener of [...listeners]) {
            try {
                listener(change);
            } catch (error) {
                console.error(`Selector field observer for "${fieldName}" failed`, error);
            }
        }
    }
}

export const selectorRegistry = new SelectorRegistry();

/**
 * Observes a named selector field from a DOM consumer that shares its form. Callers name the
 * field they depend on instead of listening for a document-wide event, so two forms on the
 * same page never cross-wire even when they use identical field names.
 */
export function observeSelectorField(
    element: Element,
    fieldName: string,
    listener: SelectorFieldChangeListener,
): () => void {
    const form = element.closest('form');
    if (!form) return () => {};
    return selectorRegistry.observe(form as HTMLFormElement, fieldName, listener);
}

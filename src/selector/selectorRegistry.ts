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
 * Stores selector handles under the identity of their owning form and field name.
 * Weak form keys allow detached forms to be collected even if their Alpine cleanup did
 * not run, while the returned cleanup function handles the normal lifecycle explicitly.
 */
export class SelectorRegistry {
    private readonly forms = new WeakMap<HTMLFormElement, Map<string, SelectorIntegrationHandle>>();

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
}

export const selectorRegistry = new SelectorRegistry();

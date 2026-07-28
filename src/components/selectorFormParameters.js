/**
 * Reads dependent search parameters off the surrounding form controls, given a parameter-name
 * -> control-name mapping. Called from inside a selector's `parameters` callback, so every
 * search re-reads the live DOM. Used by the relation form, where the type and the from/to group
 * searches each depend on what the other controls currently hold.
 *
 * The lookup is deliberately unfiltered: every control with that name is visited and the last
 * one wins, disabled or not. A selector field renders its value inputs followed by a control of
 * the same name that is empty and only enabled while nothing is selected, so a field WITH a
 * selection still contributes an empty value and the dependent search stays unnarrowed.
 * Restricting this to `:not(:disabled)` would make these cross-filters bite for the first time,
 * which is a product change -- it can leave uncategorized groups with no selectable relation
 * type -- rather than a refactor. It is tracked separately in docs/todo.md and characterized by
 * `106-autocompleter-behavior-contract`.
 *
 * A control name that matches nothing contributes no key at all, so the parameter is omitted
 * from the request rather than sent empty.
 */
export function selectorFormParameters(mapping) {
    const parameters = {};
    for (const [parameter, controlName] of Object.entries(mapping)) {
        document.querySelectorAll(`input[name=${controlName}]`).forEach((input) => {
            parameters[parameter] = input.value;
        });
    }
    return parameters;
}

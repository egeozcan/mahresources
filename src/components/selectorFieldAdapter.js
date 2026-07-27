import { selectorRegistry } from '../selector/selectorRegistry.ts';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

function mapOption(raw) {
    return { key: raw.ID, label: raw.Name, raw };
}

/**
 * Alpine rendering adapter for a selector profile. It owns only presentation concerns -- the
 * popover, the live region, focus and the DOM buffer -- and forwards every selection decision
 * to the profile's selector core. `profiledAutocompleter.js` supplies the bridge; nothing
 * constructs this from endpoints or loose flags.
 */
export function selectorFieldAdapter({ _profileBridge: profileBridge }) {
    const { profile } = profileBridge;
    // A profile without form metadata is not part of a submitted form: the lightbox tag editor
    // and the entity picker's filters render one, so there is no field name, no minimum to
    // enforce, and Escape returns focus to the surrounding surface instead of the form.
    const formMetadata = profile.form;
    const elName = formMetadata?.name;
    const min = formMetadata?.minimum ?? 0;
    const max = profileBridge.maximum;
    const creatable = Boolean(profileBridge.creatable);
    const decorateWithCategoryName = profile.presentation.decoration === 'category-name';
    // The profile owns its own search source; this endpoint is only read back for out-of-band
    // label resolution through the form registry handle.
    const lookupUrl = profile.lookup.searchUrl;
    // Chip-input: when true, a space also commits the current token. Off by default so
    // multi-word tag names stay typeable in every existing form (comma always commits).
    const { commitOnSpace } = profile.interaction;

    return {
        max,
        min,
        results: [],
        selectedIndex: -1,
        errorMessage: false,
        dropdownActive: false,
        selectedResults: profile.selector.getSnapshot().selected.map((option) => option.raw),
        addModeForTag: false,
        _addModeShown: false,
        createCandidate: '',
        _core: null,
        _unsubscribeCore: null,
        _lastSearchStatus: 'idle',
        // Read-only rendering mirror: the tag editor shows a spinner while a creation is
        // in flight. Every other core search field is read straight off the snapshot.
        loading: false,
        _selecting: false,
        _selectingTimer: null,
        // Read-only rendering mirror of the core query.
        query: '',
        _unregisterSelector: null,
        _destroyed: false,
        _popover: null,
        _popoverMouseDownHandler: null,
        // The form this field publishes atomic changes under. Set only when the field is
        // registered, so an unnamed or form-less selector notifies nobody.
        _registeredForm: null,
        _form: null,
        _formSubmitHandler: null,
        _formResetHandler: null,

        init() {
            this._destroyed = false;
            // Alpine calls init with the raw data object. Core subscriptions run later, so use
            // its public reactive data proxy to keep DOM bindings current.
            const reactive = globalThis.Alpine?.$data?.(this.$el) || this;
            reactive._core = profile.selector;
            reactive._unsubscribeCore = reactive._core.subscribe((snapshot, change) => {
                reactive._syncCoreSnapshot(snapshot, change);
            });
            reactive._syncCoreSnapshot(reactive._core.getSnapshot());

            // Add ARIA live region for announcements
            this._liveRegion = createLiveRegion(this.$el);

            // Popover management: show/hide and reposition
            this.$watch('dropdownActive', () => this.updatePopover());
            this.$watch('results', () => this.updatePopover());
            // The create row can appear with zero search results, so the popover must also
            // react to the typed buffer changing (which drives createCandidate).
            this.$watch('query', () => this.updatePopover());

            // Prevent scrollbar clicks from blurring the input.
            // mousedown.preventDefault() stops the browser from moving focus
            // away from the input, so the blur handler never fires.
            this.$nextTick(() => {
                if (this._destroyed) return;
                const popover = this.$refs?.dropdown;
                if (!popover) return;
                this._popover = popover;
                this._popoverMouseDownHandler = (e) => {
                    // Only prevent default for clicks on the popover itself (scrollbar),
                    // not on option items (which have their own mousedown.prevent)
                    if (e.target === popover) {
                        e.preventDefault();
                    }
                };
                popover.addEventListener('mousedown', this._popoverMouseDownHandler);
            });

            this._repositionHandler = () => {
                if (this.dropdownActive && this.results.length > 0) {
                    this.positionDropdown();
                }
            };
            window.addEventListener('scroll', this._repositionHandler, true);
            window.addEventListener('resize', this._repositionHandler);

            const form = this.$el.closest('form');
            if (form && elName) {
                this._unregisterSelector?.();
                const handle = {
                    getRawValues: () => this.selectedResults,
                    replaceRawValues: (values, { silent = false } = {}) => {
                        this._replaceSelection(values, { silent });
                    },
                    replaceByKeys: (keys, options = {}) => {
                        const existingById = new Map(
                            this.selectedResults.map((item) => [String(item.ID), item]),
                        );
                        const selected = keys.map((key) => {
                            const existing = existingById.get(String(key));
                            if (existing) return existing;
                            const numericKey = Number(key);
                            const ID = Number.isNaN(numericKey) ? key : numericKey;
                            return { ID, Name: `#${key}` };
                        });
                        handle.replaceRawValues(selected, options);
                    },
                    resolveExactLabels: async (labels, options = {}) => {
                        const selected = [];
                        for (const label of labels) {
                            const separator = lookupUrl.includes('?') ? '&' : '?';
                            const response = await fetch(
                                `${lookupUrl}${separator}Name=${encodeURIComponent(label)}`,
                            );
                            if (!response.ok) return false;
                            const results = await response.json();
                            const match = Array.isArray(results)
                                ? results.find((item) =>
                                    String(item.Name).toLowerCase() === label.toLowerCase())
                                : null;
                            if (!match) return false;
                            selected.push(match);
                        }
                        handle.replaceRawValues(selected, options);
                        return true;
                    },
                };
                this._unregisterSelector = selectorRegistry.register(form, elName, handle);
                this._registeredForm = form;
            }

            // Minimum enforcement and reset only apply to a field that is part of a form.
            if (form && formMetadata) {
                this._form = form;
                this._formSubmitHandler = (e) => {
                    if (this.selectedResults.length < this.min) {
                        e.preventDefault();
                        this.errorMessage = 'Please select at least ' + this.min + ' ' + (this.min === 1 ? 'value' : 'values');
                    }
                };
                this._formResetHandler = () => {
                    this._replaceSelection([], { silent: false });
                };
                form.addEventListener('submit', this._formSubmitHandler);
                form.addEventListener('reset', this._formResetHandler);
            }
        },

        _syncCoreSnapshot(snapshot, change) {
            if (this._destroyed) return;
            const searchCompleted = snapshot.searchStatus === 'success'
                && this._lastSearchStatus !== 'success';
            this._lastSearchStatus = snapshot.searchStatus;
            this.results = snapshot.options.map((option) => option.raw);
            this.selectedResults = snapshot.selected.map((option) => option.raw);
            this.dropdownActive = snapshot.isOpen;
            this.selectedIndex = snapshot.activeOptionIndex ?? -1;
            this.query = snapshot.query;
            this.createCandidate = snapshot.createCandidate?.label || '';
            // `false` is a sentinel, not just a falsy value: the shared field autofocuses on
            // `addModeForTag !== false`. It must stay `false` until the "Add X?" confirmation has
            // actually been shown -- otherwise every selector grabs focus during init and leaves
            // an open dropdown over the page -- and become '' once that flow closes, which is
            // what returns focus to the input after Cancel.
            if (snapshot.createConfirmationCandidate) {
                this.addModeForTag = snapshot.createConfirmationCandidate.label;
                this._addModeShown = true;
            } else {
                this.addModeForTag = this._addModeShown ? '' : false;
            }
            this.loading = snapshot.creationStatus === 'loading';

            if (!change) {
                const input = this.$refs?.autocompleter;
                if (searchCompleted && snapshot.isOpen && this.optionCount
                    && snapshot.activeOptionIndex === null) {
                    this._core?.dispatch({ type: 'move-active', direction: 'next' });
                }
                if (searchCompleted && snapshot.isOpen && input && document.activeElement === input) {
                    if (this.results.length) {
                        this._liveRegion?.announce(`${this.results.length} result${this.results.length === 1 ? '' : 's'} available. Use arrow keys to navigate.`);
                    } else if (!this.createCandidate) {
                        this._liveRegion?.announce('No results found.');
                    }
                }
                void this.updatePopover();
                if (snapshot.searchStatus === 'error') this.errorMessage = snapshot.searchError?.message || 'Search failed';
                if (snapshot.creationStatus === 'error') {
                    this.errorMessage = `Could not add ${snapshot.creationError?.message || 'item'}`;
                    setTimeout(() => { this.errorMessage = '' }, 3000);
                }
                return;
            }
            profileBridge.onChange?.(change);
            // Publish the atomic change to whoever named this field in this form. Registry
            // delivery replaces the old document-wide `multiple-input` broadcast: it cannot
            // reach an unrelated form that happens to reuse the field name.
            if (this._registeredForm) {
                selectorRegistry.notifyChange(this._registeredForm, elName, {
                    values: this.selectedResults,
                    added: change.added.map((option) => option.raw),
                    removed: change.removed.map((option) => option.raw),
                });
            }
            if (change.added.length) {
                this._liveRegion?.announce(`Added ${change.added[change.added.length - 1].raw.Name}`);
            } else if (change.removed.length) {
                this._liveRegion?.announce(`Removed item, ${this.selectedResults.length} items remaining`);
            }
        },

        destroy() {
            if (this._destroyed) return;
            this._destroyed = true;
            this._unsubscribeCore?.();
            this._unsubscribeCore = null;
            this._core?.destroy();
            this._core = null;
            this._unregisterSelector?.();
            this._unregisterSelector = null;
            this._registeredForm = null;
            this._popover?.removeEventListener('mousedown', this._popoverMouseDownHandler);
            this._popover = null;
            this._popoverMouseDownHandler = null;
            this._form?.removeEventListener('submit', this._formSubmitHandler);
            this._form?.removeEventListener('reset', this._formResetHandler);
            this._form = null;
            this._formSubmitHandler = null;
            this._formResetHandler = null;
            if (this._repositionHandler) {
                window.removeEventListener('scroll', this._repositionHandler, true);
                window.removeEventListener('resize', this._repositionHandler);
                this._repositionHandler = null;
            }
            this._liveRegion?.destroy();
            this._liveRegion = null;
        },

        async updatePopover() {
            await this.$nextTick();
            const popover = this.$refs?.dropdown || this.$el?.querySelector?.('[popover]');
            if (!popover) return;

            const shouldShow = this.dropdownActive && (this.results.length > 0 || !!this.createCandidate);

            if (shouldShow) {
                if (!popover.matches(':popover-open')) popover.showPopover();
                this.positionDropdown();
            } else {
                // `matches(':popover-open')` can lag a core-driven close by one Alpine tick.
                // hidePopover is idempotent for an already-hidden manual popover.
                try {
                    popover.hidePopover();
                } catch {
                    // A disconnected popover can be removed by an inline editor concurrently.
                }
            }
        },

        positionDropdown() {
            const popover = this.$refs?.dropdown;
            const input = this.$refs?.autocompleter;
            if (!popover || !input) return;

            const inputRect = input.getBoundingClientRect();
            const popoverHeight = popover.offsetHeight;
            const spaceBelow = window.innerHeight - inputRect.bottom;
            const spaceAbove = inputRect.top;
            const gap = 4;

            popover.style.width = inputRect.width + 'px';
            popover.style.left = inputRect.left + 'px';

            if (spaceBelow < popoverHeight && spaceAbove > spaceBelow) {
                popover.style.top = 'auto';
                popover.style.bottom = (window.innerHeight - inputRect.top + gap) + 'px';
            } else {
                popover.style.top = (inputRect.bottom + gap) + 'px';
                popover.style.bottom = 'auto';
            }
        },

        addVal() {
            if (this._core?.getSnapshot().createConfirmationCandidate) this._clearInput();
            this._core?.dispatch({ type: 'confirm-create' });
        },

        // Clear the DOM buffer before creation can re-render it. Creation commands own query
        // consumption themselves, so do not send an input event that invalidates their candidate.
        _clearInput({ notify = true } = {}) {
            const inputEl = this.$refs?.autocompleter;
            if (inputEl) {
                inputEl.value = '';
                if (notify) inputEl.dispatchEvent(new Event('input'));
            }
        },

        commitToken(token) {
            const label = token.trim();
            const snapshot = this._core?.getSnapshot();
            const matchesAvailable = snapshot?.options.some((option) => option.label === label);
            const matchesSelected = snapshot?.selected.some((option) => option.label === label);
            const creates = Boolean(
                label
                && creatable
                && !matchesAvailable
                && !matchesSelected
            );
            // The DOM must clear before a core creation synchronously publishes state and
            // Alpine removes/re-renders this input. Existing-option selection clears afterward
            // so its active option is not invalidated before the command reads it.
            if (creates) this._clearInput({ notify: false });
            const result = this._core?.dispatch({ type: 'commit-token', token });
            if (!creates && result?.ok && result.consumed) this._clearInput();
        },

        exitAdd() {
            this._core?.dispatch({ type: 'cancel-create-confirmation' });
        },

        // Suppresses the blur-close between an option's mousedown and its commit. The timeout is
        // only a safety net for a mousedown that never commits (a scrollbar drag); a commit ends
        // the suppression immediately, otherwise the popover outlives the blur that should have
        // closed it and covers the next field on the form.
        startSelecting() {
            this._selecting = true;
            clearTimeout(this._selectingTimer);
            this._selectingTimer = setTimeout(() => { this._selecting = false; }, 200);
        },

        _endSelecting() {
            clearTimeout(this._selectingTimer);
            this._selectingTimer = null;
            this._selecting = false;
        },

        setActiveIndex(index) {
            this._core?.dispatch({ type: 'set-active', index });
        },

        selectResult(item) {
            this._endSelecting();
            const result = this._core?.dispatch({ type: 'select-option', option: mapOption(item) });
            if (result?.ok) this._clearInput();
        },

        pushVal() {
            this._endSelecting();
            const snapshot = this._core?.getSnapshot();
            if (!snapshot) return;
            if (snapshot.isOpen && snapshot.searchStatus === 'success'
                && snapshot.activeOptionIndex === null && snapshot.options.length) {
                const result = this._core.dispatch({
                    type: 'select-option',
                    option: snapshot.options[0],
                });
                if (result.ok) this._clearInput();
                return;
            }
            if (snapshot.isOpen && snapshot.searchStatus === 'success'
                && snapshot.activeOptionIndex !== null) {
                const active = snapshot.options[snapshot.activeOptionIndex];
                if (active) {
                    this._liveRegion?.announce(`${this.getItemDisplayName(active.raw)} selected. Use arrow keys to navigate and enter to confirm.`);
                }
                const isVirtualCreateRow = !active;
                if (isVirtualCreateRow) this._clearInput({ notify: false });
                const result = this._core.dispatch({ type: 'commit-active' });
                if (!isVirtualCreateRow && result.ok && result.consumed) this._clearInput();
                return;
            }
            // Confirmation is only valid for the candidate produced by the completed core search.
            // Never manufacture one from the input while search is still loading.
            if (snapshot.searchStatus !== 'loading' && snapshot.createCandidate) {
                this._clearInput({ notify: false });
                this._core.dispatch({ type: 'request-create-confirmation' });
            }
        },

        /**
          * Replaces the whole selection with values the surrounding domain owns. Silent by
          * default: a reset describes state the selector did not choose, so it must not be
          * echoed back to the callers listening for user-driven changes.
          */
        _replaceSelection(values, { silent = true } = {}) {
            this._core?.dispatch({
                type: 'replace-selection',
                options: (values || []).map(mapOption),
                reason: 'reset',
                silent,
            });
        },

        /** Discards the current selection. Public because form-less filters clear on close. */
        clearSelection() {
            this._replaceSelection([]);
        },

        removeItem(item) {
            this._core?.dispatch({ type: 'remove-option', key: item.ID });
        },

        getItemDisplayName(item) {
            if (!decorateWithCategoryName || !item.Category?.Name) {
                return item.Name;
            }

            return `${item.Name} (${item.Category.Name})`
        },

        // Total roving options including the virtual "Create X" row when present.
        get optionCount() {
            return this.results.length + (this.createCandidate ? 1 : 0);
        },

        // Announce selected item for screen readers
        announceSelectedItem() {
            const total = this.optionCount;
            if (this.createCandidate && this.selectedIndex === this.results.length) {
                this._liveRegion.announce(`Create "${this.createCandidate}", ${total} of ${total}`);
                return;
            }
            if (this.results[this.selectedIndex]) {
                const name = this.getItemDisplayName(this.results[this.selectedIndex]);
                this._liveRegion.announce(`${name}, ${this.selectedIndex + 1} of ${total}`);
            }
        },

        // scrolls the container to the selected item
        async showSelected() {
            await this.$nextTick();

            const list = this.$refs?.dropdown;

            if (!list) {
                return;
            }

            const selected = list.querySelector('[aria-selected="true"]');

            if (!selected) {
                return;
            }

            // scroll the selected item into view if it's not already
            if (selected.offsetTop < list.scrollTop) {
                list.scrollTop = selected.offsetTop;
            } else if (selected.offsetTop + selected.offsetHeight > list.scrollTop + list.clientHeight) {
                list.scrollTop = selected.offsetTop + selected.offsetHeight - list.clientHeight;
            }
        },

        inputEvents: {
            // Chip-input keys, handled in one generic @keydown so they coexist with the modified
            // handlers below and do not depend on Alpine aliasing comma/backspace. Returns early
            // for every other key, leaving Enter/Escape/arrows/Tab to their own handlers.
            ['@keydown'](e) {
                // Backspace on an empty input removes the last applied chip.
                if (e.key === 'Backspace') {
                    if (e.target.value !== '' || this.addModeForTag || this.selectedResults.length === 0) return;
                    e.preventDefault();
                    const last = this.selectedResults[this.selectedResults.length - 1];
                    this.removeItem(last);
                    this._liveRegion.announce(`Removed ${last.Name}`);
                    return;
                }
                // Comma always commits the current token; space commits only when commitOnSpace
                // is opted in (default off, so multi-word tag names stay typeable).
                const isComma = e.key === ',';
                const isSpace = e.key === ' ';
                if (!isComma && !(isSpace && commitOnSpace)) return;
                const token = (e.target.value || '').trim();
                if (!token) return; // empty buffer: let the key type normally
                // Only intercept when there is something to commit: an exact-match result or an
                // addUrl to create with. Otherwise (e.g. a category picker) let the char type.
                const canCommit = creatable || this.results.some(x => x.Name === token);
                if (!canCommit) return;
                e.preventDefault();
                this.commitToken(token);
            },

            ['@keydown.escape'](e) {
                e.preventDefault();
                e.stopPropagation();

                this._core?.dispatch({ type: 'close' });

                // A form-less selector (lightbox tag editor, picker filter) always blurs on
                // Escape so focus returns to the surrounding surface for keyboard navigation.
                if (!formMetadata) {
                    e.target.blur();
                }
            },

            ['@keydown.arrow-up.prevent']() {
                this._core?.dispatch({ type: 'move-active', direction: 'previous' });
                this.announceSelectedItem();
                this.showSelected();
            },

            ['@keydown.arrow-down.prevent']() {
                this._core?.dispatch({ type: 'move-active', direction: 'next' });
                this.announceSelectedItem();
                this.showSelected();
            },

            ['@keydown.enter.prevent'](e) {
                e.stopPropagation();

                if (e.target.value === '') {
                    const form = e.target.closest('form');
                    if (form && formMetadata && !form.dataset.inlineEditor) {
                        form.requestSubmit();
                        return;
                    }
                }

                this.pushVal(e);

                if (this.selectedResults.length === this.max) {
                    setTimeout(() => this._core?.dispatch({ type: 'close' }), 100);
                }
            },

            ['@keydown.tab']() {
                this._core?.dispatch({ type: 'close' });
            },

            ['@blur'](e) {
                if (document.activeElement === e.target) {
                    return;
                }
                setTimeout(() => {
                    if (this._selecting) return;
                    // Focus can return before this fires -- committing an option blurs the
                    // input and the caller (or the user) refocuses it right after. Closing
                    // then would hide a dropdown that is being used again.
                    if (document.activeElement === this.$refs?.autocompleter) return;
                    this._core?.dispatch({ type: 'close' });
                }, 150);
            },

            ['@focus']() {
                this._core?.dispatch({ type: 'open' });
                this.$event.target.dispatchEvent(new Event('input'));
            },

            ['@input']() {
                const value = this.$event.target.value;
                this._core?.dispatch({ type: 'set-query', query: value });
            }
        },
    }
}

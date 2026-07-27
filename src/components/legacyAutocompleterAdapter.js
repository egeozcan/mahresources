import {
    createDebouncedSelectorSource,
    createHttpSelectorSource,
    createSelector,
} from '../selector/index.ts';
import { selectorRegistry } from '../selector/selectorRegistry.ts';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { normalizeLegacyAutocompleterConfig } from './legacyAutocompleterConfig.ts';

export function legacyAutocompleterAdapter(arguments_) {
    const { source, selection, rendering } = normalizeLegacyAutocompleterConfig(arguments_);
    const {
        searchUrl: url,
        createUrl: addUrl,
        ownerId,
        sortBy,
        dynamicFilters: filterEls,
    } = source;
    const {
        initialValues: selectedResults,
        minimum: min,
        maximum: max,
    } = selection;
    const {
        fieldName: elName,
        extraInfo,
        onSelect,
        onRemove,
        standalone,
        dispatchOnSelect,
        // Chip-input: when true, a space also commits the current token. Off by default so
        // multi-word tag names stay typeable in every existing form (comma always commits).
        commitOnSpace,
    } = rendering;

    return {
        max,
        min,
        ownerId,
        results: [],
        selectedIndex: -1,
        errorMessage: false,
        dropdownActive: false,
        selectedResults,
        selectedIds: new Set(),
        url,
        addUrl,
        extraInfo,
        filterEls,
        sortBy,
        addModeForTag: false,
        createCandidate: '',
        _core: null,
        _unsubscribeCore: null,
        _lastSearchStatus: 'idle',
        loading: false,
        _selecting: false,
        // Read-only rendering mirror of the core query.
        query: '',
        // Optimistic-write tracking for chips whose onSelect callback is async (lightbox).
        // Reactive Sets so chip :class / :data-tag-pending bindings update on add/delete.
        pendingIds: new Set(),
        failedIds: new Set(),
        _unregisterSelector: null,

        // Retained for legacy template consumers; silent core resets need no suppression flag.
        _suppressNextAnnounce: false,

        init() {
            // Alpine calls init with the raw data object. Core subscriptions run later, so use
            // its public reactive data proxy to keep DOM bindings current.
            const reactive = globalThis.Alpine?.$data?.(this.$el) || this;
            reactive._core = createSelector({
                source: createDebouncedSelectorSource(createHttpSelectorSource({
                    searchUrl: url,
                    createUrl: addUrl || undefined,
                    parameters: () => reactive.getAdditionalParams(),
                    mapOption: source.mapOption,
                }), 200),
                selected: selectedResults.map(source.mapOption),
                maxSelected: max === 0 ? undefined : max,
            });
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
                const popover = this.$refs?.dropdown;
                if (popover) {
                    popover.addEventListener('mousedown', (e) => {
                        // Only prevent default for clicks on the popover itself (scrollbar),
                        // not on option items (which have their own mousedown.prevent)
                        if (e.target === popover) {
                            e.preventDefault();
                        }
                    });
                }
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
                        this.resetSelectedResults(values, { silent });
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
                            const separator = url.includes('?') ? '&' : '?';
                            const response = await fetch(
                                `${url}${separator}Name=${encodeURIComponent(label)}`,
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
            }

            // Form handling only when not in standalone mode
            if (form && !standalone) {
                form.addEventListener('submit', (e) => {
                    if (this.selectedResults.length < this.min) {
                        e.preventDefault();
                        this.errorMessage = 'Please select at least ' + this.min + ' ' + (this.min === 1 ? 'value' : 'values');
                    }
                });

                form.addEventListener('reset', () => {
                    this.resetSelectedResults([], { silent: false });
                });
            }
        },

        _syncCoreSnapshot(snapshot, change) {
            const searchCompleted = snapshot.searchStatus === 'success'
                && this._lastSearchStatus !== 'success';
            this._lastSearchStatus = snapshot.searchStatus;
            this.results = snapshot.options.map((option) => option.raw);
            this.selectedResults = snapshot.selected.map((option) => option.raw);
            this.selectedIds = new Set(this.selectedResults.map((value) => value.ID));
            this.dropdownActive = snapshot.isOpen;
            this.selectedIndex = snapshot.activeOptionIndex ?? -1;
            this.query = snapshot.query;
            this.createCandidate = snapshot.createCandidate?.label || '';
            this.addModeForTag = snapshot.createConfirmationCandidate?.label || '';
            this.loading = snapshot.creationStatus === 'loading';

            if (!change) {
                const input = this.$refs?.autocompleter;
                if (searchCompleted && this.optionCount
                    && snapshot.activeOptionIndex === null) {
                    this._core?.dispatch({ type: 'move-active', direction: 'next' });
                }
                if (snapshot.searchStatus === 'success' && input && document.activeElement === input) {
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
            for (const option of change.removed) onRemove?.(option.raw);
            for (const option of change.added) {
                this._trackPending(option.raw, onSelect?.(option.raw));
                if (dispatchOnSelect) {
                    window.dispatchEvent(new CustomEvent(dispatchOnSelect, {
                        detail: { item: option.raw },
                        bubbles: true,
                    }));
                }
            }
            if (!standalone) {
                this.$dispatch('multiple-input', { value: this.selectedResults, name: elName });
            }
            if (change.added.length) {
                this._liveRegion?.announce(`Added ${change.added[change.added.length - 1].raw.Name}`);
            } else if (change.removed.length) {
                this._liveRegion?.announce(`Removed item, ${this.selectedResults.length} items remaining`);
            }
        },

        destroy() {
            this._unsubscribeCore?.();
            this._unsubscribeCore = null;
            this._core?.destroy();
            this._core = null;
            this._unregisterSelector?.();
            this._unregisterSelector = null;
            if (this._repositionHandler) {
                window.removeEventListener('scroll', this._repositionHandler, true);
                window.removeEventListener('resize', this._repositionHandler);
            }
            this._liveRegion?.destroy();
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
            this._core?.dispatch({ type: 'confirm-create' });
        },

        createAndSelectNow(name) {
            this._clearInput();
            this._core?.dispatch({ type: 'commit-token', token: name });
        },

        // Clear the input buffer synchronously; query/search state remains core-owned.
        _clearInput() {
            const inputEl = this.$refs?.autocompleter;
            if (inputEl) {
                inputEl.value = '';
                inputEl.dispatchEvent(new Event('input'));
            }
        },

        // Pending chip presentation remains at the compatibility edge until its dedicated migration.
        _trackPending(item, ret) {
            if (!ret || typeof ret.then !== 'function') return;
            const id = Number(item.ID);
            this.pendingIds.add(id);
            ret.then(() => {
                this.pendingIds.delete(id);
            }).catch(() => {
                this.pendingIds.delete(id);
                this.failedIds.add(id);
                setTimeout(() => this.failedIds.delete(id), 400);
            });
        },

        commitToken(token) {
            const result = this._core?.dispatch({ type: 'commit-token', token });
            if (result?.ok && result.consumed) this._clearInput();
        },

        exitAdd() {
            this._core?.dispatch({ type: 'cancel-create-confirmation' });
        },

        startSelecting() {
            this._selecting = true;
        },

        setActiveIndex(index) {
            this._core?.dispatch({ type: 'set-active', index });
        },

        selectResult(item) {
            const result = this._core?.dispatch({ type: 'select-option', option: source.mapOption(item) });
            if (result?.ok) this._clearInput();
        },

        pushVal() {
            if (this.loading) return;
            this._selecting = false;
            let snapshot = this._core?.getSnapshot();
            if (!snapshot) return;
            if (snapshot.activeOptionIndex === null && snapshot.options.length) {
                const result = this._core.dispatch({
                    type: 'select-option',
                    option: snapshot.options[0],
                });
                if (result.ok) this._clearInput();
                return;
            }
            if (snapshot.activeOptionIndex !== null) {
                const active = snapshot.options[snapshot.activeOptionIndex];
                if (active) {
                    this._liveRegion?.announce(`${this.getItemDisplayName(active.raw)} selected. Use arrow keys to navigate and enter to confirm.`);
                }
                const result = this._core.dispatch({ type: 'commit-active' });
                if (result.ok && result.consumed) this._clearInput();
                return;
            }
            // Confirmation is only valid for the candidate produced by the completed core search.
            // Never manufacture one from the input while search is still loading.
            this._core.dispatch({ type: 'request-create-confirmation' });
        },

        resetSelectedResults(tags, { silent = true } = {}) {
            this._core?.dispatch({
                type: 'replace-selection',
                options: (tags || []).map(source.mapOption),
                reason: 'reset',
                silent,
            });
        },

        ensureMaxItems() {
            this._core?.dispatch({
                type: 'replace-selection',
                options: this._core.getSnapshot().selected,
                reason: 'replace',
            });
        },

        removeItem(item) {
            this._core?.dispatch({ type: 'remove-option', key: item.ID });
        },

        getItemDisplayName(item) {
            if (!this.extraInfo || !item[this.extraInfo]?.Name) {
                return item.Name;
            }

            return `${item.Name} (${item[this.extraInfo].Name})`
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
                const canCommit = this.addUrl || this.results.some(x => x.Name === token);
                if (!canCommit) return;
                e.preventDefault();
                this.commitToken(token);
            },

            ['@keydown.escape'](e) {
                e.preventDefault();
                e.stopPropagation();

                this._core?.dispatch({ type: 'close' });

                // In standalone mode (lightbox), always blur on Escape
                // so focus returns to the lightbox for keyboard navigation
                if (standalone) {
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
                    if (form && !standalone && !form.dataset.inlineEditor) {
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
                    if (!this._selecting) this._core?.dispatch({ type: 'close' });
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

        getAdditionalParams() {
            const params = { };

            if (this.ownerId) {
                params.ownerId = this.ownerId;
            }

            if (this.sortBy) {
                params.SortBy = this.sortBy;
            }

            if (this.filterEls && Array.isArray(this.filterEls)) {
                for (const filter of this.filterEls) {
                    document.querySelectorAll(`input[name=${filter.nameInput}]:not(:disabled)`).forEach((input) => {
                        params[filter.nameGet] = input.value;
                    });
                }
            }

            return params;
        }
    }
}

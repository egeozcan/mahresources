import { abortableFetch } from '../index.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

export function autocompleter({
    selectedResults,
    max,
    min,
    ownerId,
    url,
    sortBy,
    elName,
    filterEls = [],
    addUrl = "",
    extraInfo = "",
    // Callbacks for standalone/lightbox mode
    onSelect = null,
    onRemove = null,
    standalone = false,
    // Custom event to dispatch on selection (for compare view integration)
    dispatchOnSelect = null,
    // Chip-input: when true, a space also commits the current token. Off by default so
    // multi-word tag names stay typeable in every existing form (comma always commits).
    commitOnSpace = false,
}) {
    if (typeof filterEls === "string") {
        try {
            filterEls = JSON.parse(filterEls);
        } catch (e) {
            filterEls = [ filterEls ];
        }
    }

    return {
        max: parseInt(max) || 0,
        min: parseInt(min) || 0,
        ownerId: parseInt(ownerId) || 0,
        results: [],
        selectedIndex: -1,
        errorMessage: false,
        dropdownActive: false,
        selectedResults: typeof selectedResults === "string" ? JSON.parse(selectedResults) : (selectedResults || []),
        selectedIds: new Set(),
        url,
        addUrl,
        extraInfo,
        filterEls,
        sortBy,
        requestAborter: null,
        debounceTimer: null,
        addModeForTag: false,
        loading: false,
        _selecting: false,
        // Reactive mirror of the input's current value, so createCandidate() recomputes as
        // the user types (the component otherwise reads value imperatively).
        query: '',
        // The buffer value the current `results` correspond to. The "Create X" row only shows
        // once the search for the CURRENT buffer has completed (otherwise it would flash before
        // results load and be mistaken for a real option).
        _searchedQuery: null,
        // Optimistic-write tracking for chips whose onSelect callback is async (lightbox).
        // Reactive Sets so chip :class / :data-tag-pending bindings update on add/delete.
        pendingIds: new Set(),
        failedIds: new Set(),

        // Set by resetSelectedResults() right before an external (non-user) wholesale
        // replacement of selectedResults — e.g. the lightbox swapping to a different
        // resource's tags. Consumed once by the selectedResults $watch below so that swap
        // is not mistaken for a user add/remove and announced as one.
        _suppressNextAnnounce: false,
        // Sequencer for createAndSelectNow() so back-to-back new-tag commits queue instead
        // of racing the single `loading` guard in _createAndSelect.
        _createQueue: [],
        _processingCreateQueue: false,

        // The trimmed buffer to offer as a brand-new tag: only when an addUrl exists, the
        // buffer is non-empty, and it is not already an exact result or an applied chip.
        get createCandidate() {
            const token = (this.query || '').trim();
            if (!this.addUrl || !token) return '';
            // Only offer create once the search for THIS buffer has completed, so the row never
            // appears during the debounce window (where it would be taken for a real result).
            if ((this._searchedQuery || '').trim() !== token) return '';
            if (this.results.some(x => x.Name === token)) return '';
            if (this.selectedResults.some(x => x.Name === token)) return '';
            return token;
        },

        init() {
            this.selectedResults.forEach(val => {
                this.selectedIds.add(val.ID);
            });

            // Add ARIA live region for announcements
            this._liveRegion = createLiveRegion(this.$el);

            this.$watch('selectedResults', (values, oldValues) => {
                this.selectedIds.clear();
                values.forEach(val => {
                    this.selectedIds.add(val.ID);
                });
                if (!standalone) {
                    this.$dispatch('multiple-input', { value: this.selectedResults, name: elName });
                }

                // Announce selection changes — but not when this change is an external
                // wholesale swap (e.g. the lightbox displaying a different resource's tags)
                // rather than a real user add/remove on the current selection.
                if (this._suppressNextAnnounce) {
                    this._suppressNextAnnounce = false;
                } else if (values.length > oldValues.length) {
                    this._liveRegion.announce(`Added ${values[values.length-1].Name}`);
                } else if (values.length < oldValues.length) {
                    this._liveRegion.announce(`Removed item, ${values.length} items remaining`);
                }
            });

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

            // Form handling only when not in standalone mode
            const form = this.$el.closest('form');
            if (form && !standalone) {
                form.addEventListener('submit', (e) => {
                    if (this.selectedResults.length < this.min) {
                        e.preventDefault();
                        this.errorMessage = 'Please select at least ' + this.min + ' ' + (this.min === 1 ? 'value' : 'values');
                    }
                });

                form.addEventListener('reset', (e) => {
                    this.selectedResults = [];
                });
            }
        },

        destroy() {
            if (this._repositionHandler) {
                window.removeEventListener('scroll', this._repositionHandler, true);
                window.removeEventListener('resize', this._repositionHandler);
            }
            if (this.debounceTimer) {
                clearTimeout(this.debounceTimer);
            }
            this._liveRegion?.destroy();
        },

        async updatePopover() {
            await this.$nextTick();
            const popover = this.$refs?.dropdown;
            if (!popover) return;

            const shouldShow = this.dropdownActive && (this.results.length > 0 || !!this.createCandidate);

            if (shouldShow) {
                if (!popover.matches(':popover-open')) {
                    popover.showPopover();
                }
                this.positionDropdown();
            } else {
                if (popover.matches(':popover-open')) {
                    popover.hidePopover();
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

        // Confirm-button path ("Add X?"): create the tag named by addModeForTag, then leave
        // add-mode (which re-renders an empty input). Called as a bare Alpine handler, so it
        // takes no positional args.
        async addVal() {
            const name = this.addModeForTag;
            await this._createAndSelect(name);
            this.exitAdd();
        },

        // One-step create used by comma-commit and the "Create X" dropdown row: create the
        // tag and clear the input without flashing the "Add X?" confirm UI (which would steal
        // focus mid-typing). Queued so committing several brand-new tags back-to-back (e.g.
        // typing "a,b,") doesn't silently drop a token whose create POST starts while an
        // earlier one is still in flight — _createAndSelect's single `loading` guard would
        // otherwise no-op it.
        async createAndSelectNow(name) {
            // Clear the buffer up front (optimistic, and while the input ref is still valid —
            // it can go stale across the create await as the dropdown/templates re-render).
            this._clearInput();
            this._createQueue.push(name);
            if (this._processingCreateQueue) return;
            this._processingCreateQueue = true;
            while (this._createQueue.length) {
                const next = this._createQueue.shift();
                // An earlier queued create (or a fast exact-match select) may have already
                // applied this exact name while we waited — skip a redundant duplicate create.
                if (this.selectedResults.some(x => x.Name === next)) continue;
                await this._createAndSelect(next);
            }
            this._processingCreateQueue = false;
        },

        // Shared network add. POSTs to addUrl, applies the returned tag, and tracks the
        // optimistic onSelect thenable (lightbox) for the pending/failure chip visuals.
        async _createAndSelect(name) {
            if (this.loading || !name) {
                return;
            }

            this.loading = true;

            try {
                const response = await fetch(this.addUrl, {
                    method: 'POST',
                    body: JSON.stringify({ Name: name, ...this.getAdditionalParams() }),
                    headers: {
                        "Content-Type": "application/json",
                    },
                });
                if (!response.ok) {
                    const errorData = await response.json().catch(() => ({}));
                    throw new Error(errorData.error || `Server error: ${response.status}`);
                }
                const newVal = await response.json();
                this.selectedResults.push(newVal);
                this.selectedIds.add(newVal.ID);
                this.ensureMaxItems();

                // Call onSelect callback if provided
                if (onSelect) {
                    this._trackPending(newVal, onSelect(newVal));
                }
            } catch (e) {
                this.errorMessage = `Could not add ${name}`
                setTimeout(() => { this.errorMessage = '' }, 3000);
            } finally {
                this.loading = false;
            }
        },

        // Clear the input buffer and re-fire its input event so results/createCandidate reset.
        _clearInput() {
            const inputEl = this.$refs?.autocompleter;
            if (inputEl) {
                inputEl.value = '';
                inputEl.dispatchEvent(new Event('input'));
            }
        },

        // Add the chip's id to pendingIds while an async onSelect is in flight; on rejection,
        // briefly mark it failed (the lightbox store also rolls the chip back).
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

        // Commit a typed token: select an exact-match result, else create it (when addUrl is
        // set). No-op for an already-applied name. Used by comma/space commit.
        commitToken(token) {
            if (!token) return;
            if (this.selectedResults.some(x => x.Name === token)) {
                this._clearInput();
                return;
            }
            const matchIndex = this.results.findIndex(x => x.Name === token);
            if (matchIndex !== -1) {
                this.selectedIndex = matchIndex;
                this.dropdownActive = true;
                this.pushVal();
                return;
            }
            if (this.addUrl) {
                this.createAndSelectNow(token);
            }
        },

        exitAdd() {
            if (this.loading) {
                return;
            }

            this.addModeForTag = '';
        },

        startSelecting() {
            this._selecting = true;
        },

        pushVal($event) {
            if (this.loading) {
                return;
            }
            this._selecting = false;

            // Activating the virtual "Create X" row (roving index just past the results) →
            // one-step create, no "Add X?" confirm flash.
            if (this.dropdownActive && this.createCandidate && this.selectedIndex >= this.results.length) {
                this.createAndSelectNow(this.createCandidate);
                return;
            }

            // Announce selection
            if (this.results[this.selectedIndex]) {
                const selectedName = this.getItemDisplayName(this.results[this.selectedIndex]);
                this._liveRegion.announce(`${selectedName} selected. Use arrow keys to navigate and enter to confirm.`);
            }

            /*
                The dropdown is not open and/or there are no selected results
            */
            if (!this.results[this.selectedIndex] || !this.dropdownActive) {
                if (!this.addUrl) {
                    return;
                }

                const value = this.$refs?.autocompleter?.value;

                /*
                    We have an add url, so maybe try adding the option if it wasn't in the list already
                */
                if (!this.results.find(x => x.Name === value)) {
                    this.addModeForTag = value;
                } else {
                    this.addModeForTag = "";
                    this.dropdownActive = true;
                }

                return;
            }

            const selectedItem = this.results[this.selectedIndex];
            this.selectedResults.push(selectedItem);
            this.selectedIds.add(selectedItem.ID);
            this.ensureMaxItems();

            // Call onSelect callback if provided
            if (onSelect) {
                this._trackPending(selectedItem, onSelect(selectedItem));
            }

            // Dispatch custom event if specified (for compare view integration)
            // Use window.dispatchEvent so it can be caught with .window modifier
            if (dispatchOnSelect) {
                window.dispatchEvent(new CustomEvent(dispatchOnSelect, {
                    detail: { item: selectedItem },
                    bubbles: true
                }));
            }

            // Clear the input and trigger a refresh of results
            // Always use the autocompleter ref since $event.target might be a dropdown item (mousedown)
            const inputEl = this.$refs?.autocompleter;
            if (inputEl) {
                inputEl.value = '';
                inputEl.dispatchEvent(new Event('input'));
            }
        },

        // Wholesale-replace selectedResults from an external source (the lightbox swapping
        // to a different resource's tags) without the selectedResults $watch announcing it
        // as a user add/remove.
        resetSelectedResults(tags) {
            this._suppressNextAnnounce = true;
            this.selectedResults = [...tags];
        },

        ensureMaxItems() {
            while (this.max !== 0 && this.selectedResults.length > Math.max(this.max, 0)) {
                this.selectedResults.splice(0, 1);
            }
        },

        removeItem(item) {
            const index = this.selectedResults.findIndex(r => r.ID === item.ID);
            if (index !== -1) {
                this.selectedResults.splice(index, 1);
                // Call onRemove callback if provided
                if (onRemove) {
                    onRemove(item);
                }
            }
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

                if (this.dropdownActive) {
                    this.dropdownActive = false;
                }

                // In standalone mode (lightbox), always blur on Escape
                // so focus returns to the lightbox for keyboard navigation
                if (standalone) {
                    e.target.blur();
                }
            },

            ['@keydown.arrow-up.prevent']() {
                const total = this.optionCount;
                if (!this.dropdownActive && total > 0) {
                    this.dropdownActive = true;
                }
                if (total === 0) return;
                this.selectedIndex = this.selectedIndex <= 0 ? total - 1 : this.selectedIndex - 1;
                this.announceSelectedItem();
                this.showSelected();
            },

            ['@keydown.arrow-down.prevent']() {
                const total = this.optionCount;
                if (!this.dropdownActive && total > 0) {
                    this.dropdownActive = true;
                }
                if (total === 0) return;
                this.selectedIndex = (this.selectedIndex + 1) % total;
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
                    setTimeout(() => {
                        this.dropdownActive = false;
                    }, 100);
                }
            },

            ['@keydown.tab']() {
                this.dropdownActive = false;
            },

            ['@blur'](e) {
                if (document.activeElement === e.target) {
                    return;
                }
                setTimeout(() => {
                    if (!this._selecting) {
                        this.dropdownActive = false;
                    }
                }, 150);
            },

            ['@focus']() {
                this.dropdownActive = true;
                this.$event.target.dispatchEvent(new Event('input'));
            },

            ['@input']() {
                const target = this.$event.target;
                const value = target.value;

                // Mirror the buffer into reactive state so createCandidate() recomputes.
                this.query = value;

                this.results = this.results.filter(val => !this.selectedIds.has(val.ID));

                if (this.debounceTimer) {
                    clearTimeout(this.debounceTimer);
                }

                if (this.requestAborter) {
                    this.requestAborter();
                    this.requestAborter = null;
                }

                this.debounceTimer = setTimeout(() => {
                    const params = new URLSearchParams({ name: target.value, ...this.getAdditionalParams() })

                    const {
                        abort,
                        ready
                    } = abortableFetch(url + '?' + params.toString(), {})

                    ready.then(x => x.json()).then(values => {
                        if (value !== target.value) {
                            return;
                        }
                        this.results = values.filter(val => !this.selectedIds.has(val.ID));
                        // Mark the search as completed for this buffer so createCandidate may show.
                        this._searchedQuery = value;

                        if (this.results.length && document.activeElement === target) {
                            this.dropdownActive = true;
                            this.selectedIndex = 0;
                            // Announce results for screen readers
                            this._liveRegion.announce(`${this.results.length} result${this.results.length === 1 ? '' : 's'} available. Use arrow keys to navigate.`);
                        } else if (this.createCandidate && document.activeElement === target) {
                            // No matches, but the buffer can be created: open the popover with the
                            // "Create X" row pre-highlighted at the virtual index.
                            this.dropdownActive = true;
                            this.selectedIndex = this.results.length;
                        } else if (this.results.length === 0) {
                            this._liveRegion.announce('No results found.');
                        }
                    }).catch(err => {
                        this.errorMessage = err.toString();
                    });

                    this.requestAborter = abort;
                }, 200);
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
                    document.querySelectorAll(`input[name=${filter.nameInput}]`).forEach((input) => {
                        params[filter.nameGet] = input.value;
                    });
                }
            }

            return params;
        }
    }
}

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

                // Announce selection changes
                if (values.length > oldValues.length) {
                    this._liveRegion.announce(`Added ${values[values.length-1].Name}`);
                } else if (values.length < oldValues.length) {
                    this._liveRegion.announce(`Removed item, ${values.length} items remaining`);
                }
            });

            // Popover management: show/hide and reposition
            this.$watch('dropdownActive', () => this.updatePopover());
            this.$watch('results', () => this.updatePopover());

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

            const shouldShow = this.dropdownActive && this.results.length > 0;

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

        async addVal() {
            if (this.loading) {
                return;
            }

            this.loading = true;

            try {
                const newVal = await fetch(this.addUrl, {
                    method: 'POST',
                    body: JSON.stringify({ Name: this.addModeForTag, ...this.getAdditionalParams() }),
                    headers: {
                        "Content-Type": "application/json",
                    },
                }).then(x => x.json());
                this.selectedResults.push(newVal);
                this.ensureMaxItems();

                // Call onSelect callback if provided
                if (onSelect) {
                    onSelect(newVal);
                }
            } catch (e) {
                this.errorMessage = `Could not add ${this.addModeForTag}`
                setTimeout(() => { this.errorMessage = '' }, 3000);
            } finally {
                this.loading = false;
                this.exitAdd();
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
                if (!this.results.find(x => x.name === value)) {
                    this.addModeForTag = value;
                } else {
                    this.addModeForTag = "";
                    this.dropdownActive = true;
                }

                return;
            }

            const selectedItem = this.results[this.selectedIndex];
            this.selectedResults.push(selectedItem);
            this.ensureMaxItems();

            // Call onSelect callback if provided
            if (onSelect) {
                onSelect(selectedItem);
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

        // Announce selected item for screen readers
        announceSelectedItem() {
            if (this.results[this.selectedIndex]) {
                const name = this.getItemDisplayName(this.results[this.selectedIndex]);
                this._liveRegion.announce(`${name}, ${this.selectedIndex + 1} of ${this.results.length}`);
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
            ['@keydown.escape'](e) {
                if (!this.dropdownActive) {
                    return;
                }

                e.preventDefault();
                e.stopPropagation();
                this.dropdownActive = false;
            },

            ['@keydown.arrow-up.prevent']() {
                if (this.results.length === 0) return;
                this.selectedIndex = this.selectedIndex === 0 ? this.results.length - 1 : this.selectedIndex - 1;
                this.announceSelectedItem();
                this.showSelected();
            },

            ['@keydown.arrow-down.prevent']() {
                if (this.results.length === 0) return;
                this.selectedIndex = (this.selectedIndex + 1) % this.results.length;
                this.announceSelectedItem();
                this.showSelected();
            },

            ['@keydown.enter.prevent'](e) {
                if (e.target.value === '' && !this.dropdownActive) {
                    const form = e.target.closest('form');
                    if (form && !standalone) {
                        form.dispatchEvent(new Event('submit'));
                    }
                    return;
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

                        if (this.results.length && document.activeElement === target) {
                            this.dropdownActive = true;
                            this.selectedIndex = 0;
                            // Announce results for screen readers
                            this._liveRegion.announce(`${this.results.length} result${this.results.length === 1 ? '' : 's'} available. Use arrow keys to navigate.`);
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

document.addEventListener('alpine:init', () => {
    window.Alpine.data('autocompleter', ({
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
    }) => {
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
            addModeForTag: false,
            loading: false,

            init() {
                this.selectedResults.forEach(val => {
                    this.selectedIds.add(val.ID);
                });

                // Add ARIA live region for announcements
                const liveRegion = document.createElement('div');
                liveRegion.setAttribute('aria-live', 'polite');
                liveRegion.setAttribute('aria-atomic', 'true');
                liveRegion.className = 'sr-only';
                this.$el.appendChild(liveRegion);
                this.liveRegion = liveRegion;

                this.$watch('selectedResults', values => {
                    this.selectedIds.clear();
                    values.forEach(val => {
                        this.selectedIds.add(val.ID);
                    });
                    this.$dispatch('multiple-input', { value: selectedResults, name: elName });
                    
                    // Announce selection changes
                    if(values.length > this.selectedResults.length) {
                        this.liveRegion.textContent = `Added ${values[values.length-1].Name}`;
                    } else {
                        this.liveRegion.textContent = `Removed item, ${values.length} items remaining`;
                    }
                });

                this.$el.closest('form').addEventListener('submit', (e) => {
                    if (selectedResults.length < min) {
                        e.preventDefault();
                        this.errorMessage = 'Please select at least ' + min + ' ' + (min === 1 ? 'value' : 'values');
                    }
                });

                this.$el.closest('form').addEventListener('reset', (e) => {
                    this.selectedResults = [];
                });
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
                } catch (e) {
                    this.errorMessage = `Could not add ${this.addModeForTag}`
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

            pushVal($event) {
                if (this.loading) {
                    return;
                }

                // Announce selection
                if (this.results[this.selectedIndex]) {
                    const selectedName = this.getItemDisplayName(this.results[this.selectedIndex]);
                    this.liveRegion.textContent = `${selectedName} selected. Use arrow keys to navigate and enter to confirm.`;
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

                this.selectedResults.push(this.results[this.selectedIndex]);
                this.ensureMaxItems();

                $event.target.value = '';
                if (this.$refs?.autocompleter) {
                    this.$refs.autocompleter.value = '';
                }
                $event.target.dispatchEvent(new Event('input'));
            },

            ensureMaxItems() {
                while (this.max !== 0 && this.selectedResults.length > Math.max(this.max, 0)) {
                    this.selectedResults.splice(0, 1);
                }
            },

            getItemDisplayName(item) {
                if (!this.extraInfo || !item[this.extraInfo]?.Name) {
                    return item.Name;
                }

                return `${item.Name} (${item[this.extraInfo].Name})`
            },

            // scrolls mthe container to the selected item
            async showSelected() {
                await this.$nextTick();

                const list = this.$refs?.list?.closest(".overflow-x-auto");

                if (!list) {
                    return;
                }

                const selected = list.querySelector('.bg-blue-500');

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
                    this.dropdownActive = false;
                },

                ['@keydown.arrow-up.prevent']() {
                    this.selectedIndex = this.selectedIndex === 0 ? this.results.length - 1 : this.selectedIndex - 1;

                    this.showSelected();
                },

                ['@keydown.arrow-down.prevent']() {
                    this.selectedIndex = (this.selectedIndex + 1) % this.results.length;

                    this.showSelected();
                },

                ['@keydown.enter.prevent'](e) {
                    if (e.target.value === '' && !this.dropdownActive) {
                        e.target.closest('form').dispatchEvent(new Event('submit'));
                        return;
                    }

                    this.pushVal(e);

                    if (this.selectedResults.length === max) {
                        setTimeout(() => {
                            this.dropdownActive = false;
                        }, 100);
                    }
                },

                ['@blur'](e) {
                    if (document.activeElement === e.target) {
                        return;
                    }
                    setTimeout(() => {
                        this.dropdownActive = false;
                    }, 10);
                },

                ['@focus']() {
                    this.dropdownActive = true;
                    this.$event.target.dispatchEvent(new Event('input'));
                },

                ['@input']() {
                    const target = this.$event.target;
                    const value = target.value;

                    this.results = this.results.filter(val => !this.selectedIds.has(val.ID));

                    if (this.requestAborter) {
                        this.requestAborter();
                        this.requestAborter = null;
                    }

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
                        }
                    }).catch(err => {
                        this.errorMessage = err.toString();
                    });

                    this.requestAborter = abort;
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
    })
})

import { abortableFetch } from '../index.js';

export default function timeline({ apiUrl, entityType, defaultView }) {
    return {
        // Configuration (from template params)
        apiUrl,
        entityType,
        defaultView,

        // State
        granularity: 'month',
        anchor: new Date().toISOString().slice(0, 10),
        columns: 12,
        buckets: [],
        hasMore: { left: true, right: false },
        selectedBar: null,
        selectedBarType: null,
        previewItems: [],
        previewTitle: '',
        previewTotalCount: 0,
        loading: false,
        error: null,
        maxCount: 0,

        // Internal
        _resizeObserver: null,
        _fetchAborter: null,
        _previewAborter: null,

        init() {
            this.calculateColumns();
            this.fetchBuckets();

            this._resizeObserver = new ResizeObserver(() => {
                const newCols = this.calculateColumns();
                if (newCols !== this.columns) {
                    this.columns = newCols;
                    this.fetchBuckets();
                }
            });
            this._resizeObserver.observe(this.$el);
        },

        destroy() {
            if (this._resizeObserver) {
                this._resizeObserver.disconnect();
                this._resizeObserver = null;
            }
            if (this._fetchAborter) {
                this._fetchAborter();
                this._fetchAborter = null;
            }
            if (this._previewAborter) {
                this._previewAborter();
                this._previewAborter = null;
            }
        },

        calculateColumns() {
            const width = this.$el ? this.$el.clientWidth : 720;
            const cols = Math.floor(width / 60);
            this.columns = Math.max(5, Math.min(30, cols));
            return this.columns;
        },

        get rangeLabel() {
            if (!this.buckets || this.buckets.length === 0) return '';
            const first = this.buckets[0].label;
            const last = this.buckets[this.buckets.length - 1].label;
            if (first === last) return first;
            return first + ' \u2014 ' + last;
        },

        barHeight(count) {
            if (!this.maxCount || this.maxCount === 0) return '0%';
            return Math.max(2, (count / this.maxCount) * 100) + '%';
        },

        async fetchBuckets() {
            if (this._fetchAborter) {
                this._fetchAborter();
                this._fetchAborter = null;
            }

            this.loading = true;
            this.error = null;

            const params = new URLSearchParams(window.location.search);
            // Backend expects 'yearly'/'monthly'/'weekly', frontend uses 'year'/'month'/'week'
            const granularityMap = { year: 'yearly', month: 'monthly', week: 'weekly' };
            params.set('granularity', granularityMap[this.granularity] || 'monthly');
            params.set('anchor', this.anchor);
            params.set('columns', String(this.columns));

            try {
                const { abort, ready } = abortableFetch(this.apiUrl + '?' + params.toString());
                this._fetchAborter = abort;

                const response = await ready;
                if (!response.ok) throw new Error('HTTP ' + response.status);

                const data = await response.json();
                this.buckets = data.buckets || [];
                this.hasMore = data.hasMore || { left: true, right: false };
                this.maxCount = 0;
                for (const b of this.buckets) {
                    const created = b.created || 0;
                    const updated = b.updated || 0;
                    if (created > this.maxCount) this.maxCount = created;
                    if (updated > this.maxCount) this.maxCount = updated;
                }
                this.loading = false;
                this.renderChart();
            } catch (err) {
                if (err.name !== 'AbortError') {
                    this.error = err.message || 'Failed to load timeline data';
                    this.loading = false;
                }
            } finally {
                this._fetchAborter = null;
            }
        },

        setGranularity(g) {
            this.granularity = g;
            this.anchor = new Date().toISOString().slice(0, 10);
            this.closePreview();
            this.fetchBuckets();
        },

        prev() {
            if (this.buckets.length > 0) {
                this.anchor = this.buckets[0].start.slice(0, 10);
            }
            this.closePreview();
            this.fetchBuckets();
        },

        next() {
            if (!this.hasMore.right) return;
            if (this.buckets.length > 0) {
                const lastEnd = this.buckets[this.buckets.length - 1].end.slice(0, 10);
                const today = new Date().toISOString().slice(0, 10);
                this.anchor = lastEnd > today ? today : lastEnd;
            }
            this.closePreview();
            this.fetchBuckets();
        },

        renderChart() {
            const chart = this.$refs.chart;
            if (!chart) return;

            // Clear existing content
            chart.innerHTML = '';

            if (this.buckets.length === 0) {
                chart.innerHTML = '<p class="text-center text-stone-600 py-4 text-sm">No data for this range.</p>';
                return;
            }

            const wrapper = document.createElement('div');
            wrapper.style.display = 'flex';
            wrapper.style.alignItems = 'flex-end';
            wrapper.style.gap = '2px';
            wrapper.style.height = '160px';
            wrapper.style.padding = '0';

            this.buckets.forEach((bucket, index) => {
                const col = document.createElement('div');
                col.style.flex = '1';
                col.style.display = 'flex';
                col.style.flexDirection = 'column';
                col.style.alignItems = 'center';
                col.style.height = '100%';
                col.style.justifyContent = 'flex-end';
                col.style.gap = '1px';
                col.style.minWidth = '0';

                const createdCount = bucket.created || 0;
                const updatedCount = bucket.updated || 0;

                // Updated bar (behind/lighter)
                if (updatedCount > 0) {
                    const updatedBar = document.createElement('button');
                    updatedBar.type = 'button';
                    updatedBar.className = 'timeline-bar timeline-bar-updated' + (this.selectedBar === index && this.selectedBarType === 'updated' ? ' selected' : '');
                    updatedBar.style.height = this.barHeight(updatedCount);
                    updatedBar.style.width = '100%';
                    updatedBar.setAttribute('aria-label', bucket.label + ': ' + updatedCount + ' updated');
                    updatedBar.title = bucket.label + ': ' + updatedCount + ' updated';
                    updatedBar.dataset.index = index;
                    updatedBar.dataset.type = 'updated';
                    updatedBar.addEventListener('click', () => this.selectBar(index, 'updated'));
                    col.appendChild(updatedBar);
                }

                // Created bar (in front/darker)
                if (createdCount > 0) {
                    const createdBar = document.createElement('button');
                    createdBar.type = 'button';
                    createdBar.className = 'timeline-bar timeline-bar-created' + (this.selectedBar === index && this.selectedBarType === 'created' ? ' selected' : '');
                    createdBar.style.height = this.barHeight(createdCount);
                    createdBar.style.width = '100%';
                    createdBar.setAttribute('aria-label', bucket.label + ': ' + createdCount + ' created');
                    createdBar.title = bucket.label + ': ' + createdCount + ' created';
                    createdBar.dataset.index = index;
                    createdBar.dataset.type = 'created';
                    createdBar.addEventListener('click', () => this.selectBar(index, 'created'));
                    col.appendChild(createdBar);
                }

                // Empty placeholder if both are 0
                if (createdCount === 0 && updatedCount === 0) {
                    const emptyBar = document.createElement('div');
                    emptyBar.style.height = '2px';
                    emptyBar.style.width = '100%';
                    emptyBar.style.background = '#e7e5e4';
                    emptyBar.style.borderRadius = '2px';
                    col.appendChild(emptyBar);
                }

                // Label below
                const label = document.createElement('span');
                label.className = 'text-stone-600 mt-1';
                label.style.fontSize = '0.5625rem';
                label.style.lineHeight = '1';
                label.style.whiteSpace = 'nowrap';
                label.style.overflow = 'hidden';
                label.style.textOverflow = 'ellipsis';
                label.style.maxWidth = '100%';
                label.textContent = bucket.label;
                col.appendChild(label);

                wrapper.appendChild(col);
            });

            chart.appendChild(wrapper);
        },

        async selectBar(index, barType) {
            // Toggle off if clicking same bar
            if (this.selectedBar === index && this.selectedBarType === barType) {
                this.closePreview();
                this.renderChart();
                return;
            }

            const bucket = this.buckets[index];
            if (!bucket) return;

            this.selectedBar = index;
            this.selectedBarType = barType;

            const count = barType === 'created' ? (bucket.created || 0) : (bucket.updated || 0);
            const typeLabel = barType === 'created' ? 'Created' : 'Updated';
            this.previewTitle = bucket.label + ' \u2014 ' + typeLabel + ' (' + count + ')';
            this.previewTotalCount = count;

            // Re-render chart to update selected state
            this.renderChart();

            // Fetch preview items
            if (this._previewAborter) {
                this._previewAborter();
                this._previewAborter = null;
            }

            const params = new URLSearchParams(window.location.search);
            if (barType === 'created') {
                params.set('CreatedAfter', bucket.start);
                params.set('CreatedBefore', bucket.end);
            } else {
                params.set('UpdatedAfter', bucket.start);
                params.set('UpdatedBefore', bucket.end);
            }
            params.set('pageSize', '20');

            try {
                const url = this.apiUrl.replace('/timeline', '.json') + '?' + params.toString();
                const { abort, ready } = abortableFetch(url);
                this._previewAborter = abort;

                const response = await ready;
                if (!response.ok) throw new Error('HTTP ' + response.status);

                const data = await response.json();
                // The API typically returns an array or object with items
                this.previewItems = Array.isArray(data) ? data : (data.items || data.results || data.data || []);
            } catch (err) {
                if (err.name !== 'AbortError') {
                    console.error('Failed to load preview:', err);
                    this.previewItems = [];
                }
            } finally {
                this._previewAborter = null;
            }
        },

        closePreview() {
            this.selectedBar = null;
            this.selectedBarType = null;
            this.previewItems = [];
            this.previewTitle = '';
            this.previewTotalCount = 0;
        },
    };
}

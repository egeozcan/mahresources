import { abortableFetch } from '../index.js';

export default function timeline({ apiUrl, entityType, defaultView }) {
    return {
        // Configuration (from template params)
        apiUrl,
        entityType,
        defaultView,

        // State
        timelineMode: 'created',
        granularity: 'month',
        anchor: new Date().toISOString().slice(0, 10),
        columns: 12,
        buckets: [],
        hasMore: { left: true, right: false },
        selectedBar: null,
        selectedBarType: null,
        previewHtml: '',
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
                // P3 fix: calculate without mutating first, then compare + assign
                const width = this.$el ? this.$el.clientWidth : 720;
                const newCols = Math.max(5, Math.min(30, Math.floor(width / 60)));
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
                const key = this.timelineMode;
                for (const b of this.buckets) {
                    const v = b[key] || 0;
                    if (v > this.maxCount) this.maxCount = v;
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

        setTimelineMode(mode) {
            this.timelineMode = mode;
            this.closePreview();
            // Recompute maxCount for the new mode
            this.maxCount = 0;
            for (const b of this.buckets) {
                const v = b[mode] || 0;
                if (v > this.maxCount) this.maxCount = v;
            }
            this.renderChart();
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
                // Shift forward by the same distance prev() shifts back:
                // prev sets anchor = firstStart, so it moves back by (anchor - firstStart).
                // Mirror that by adding the same offset forward.
                const firstStart = new Date(this.buckets[0].start);
                const anchorDate = new Date(this.anchor + 'T00:00:00Z');
                const shiftMs = anchorDate.getTime() - firstStart.getTime();
                const newAnchor = new Date(anchorDate.getTime() + shiftMs);
                const today = new Date().toISOString().slice(0, 10);
                const candidate = newAnchor.toISOString().slice(0, 10);
                this.anchor = candidate > today ? today : candidate;
            }
            this.closePreview();
            this.fetchBuckets();
        },

        renderChart() {
            const chart = this.$refs.chart;
            if (!chart) return;

            chart.innerHTML = '';

            if (this.buckets.length === 0 || this.maxCount === 0) {
                chart.innerHTML = '<p class="text-center text-stone-500 py-8 text-sm">No activity in this period.</p>';
                return;
            }

            // Y-axis scale + bars row (fixed height), then labels row below
            const chartRow = document.createElement('div');
            chartRow.className = 'timeline-chart-row';

            // Y-axis with tick marks
            const yAxis = document.createElement('div');
            yAxis.className = 'timeline-y-axis';
            const ticks = this._yAxisTicks(this.maxCount);
            ticks.forEach(tick => {
                const tickEl = document.createElement('div');
                tickEl.className = 'timeline-y-tick';
                tickEl.style.bottom = (tick.value / this.maxCount * 100) + '%';
                const label = document.createElement('span');
                label.className = 'timeline-y-label';
                label.textContent = tick.label;
                tickEl.appendChild(label);
                yAxis.appendChild(tickEl);
            });

            const barsRow = document.createElement('div');
            barsRow.className = 'timeline-bars-row';

            chartRow.appendChild(yAxis);
            chartRow.appendChild(barsRow);

            const labelsRow = document.createElement('div');
            labelsRow.className = 'timeline-labels-row';

            const mode = this.timelineMode;
            const barClass = mode === 'created' ? 'timeline-bar-created' : 'timeline-bar-updated';

            this.buckets.forEach((bucket, index) => {
                const count = bucket[mode] || 0;
                const tooltipText = bucket.label + ' \u2014 ' + count + ' ' + mode;

                // Bar column
                const col = document.createElement('div');
                col.className = 'timeline-bucket-col';
                col.title = tooltipText;

                if (count > 0) {
                    const bar = document.createElement('button');
                    bar.type = 'button';
                    bar.className = 'timeline-bar ' + barClass + (this.selectedBar === index ? ' selected' : '');
                    bar.style.height = this.barHeight(count);
                    bar.setAttribute('aria-label', tooltipText);
                    bar.addEventListener('click', () => this.selectBar(index, mode));
                    col.appendChild(bar);
                } else {
                    const emptyBar = document.createElement('div');
                    emptyBar.className = 'timeline-bar-empty';
                    col.appendChild(emptyBar);
                }

                barsRow.appendChild(col);

                // Label cell (separate row, always visible)
                const labelCell = document.createElement('div');
                labelCell.className = 'timeline-label-cell';
                const label = document.createElement('span');
                label.className = 'timeline-label';
                label.textContent = this._shortLabel(bucket.label, index);
                labelCell.appendChild(label);
                labelsRow.appendChild(labelCell);
            });

            chart.appendChild(chartRow);
            chart.appendChild(labelsRow);
        },

        // Build date filter params from bucket bounds.
        // Bucket end is exclusive (e.g. 2026-03-16 for the Mar 09 week bucket).
        // Pass the exclusive end date directly: ApplyDateRange uses `<=` and
        // SQLite string comparison means '2026-03-15T10:00:00Z' < '2026-03-16'
        // so all items within the bucket are correctly included while items on
        // the next bucket's start date are excluded ('2026-03-16T...' > '2026-03-16').
        // Also drops 'page' param so drill-down always starts at page 1.
        _buildDateParams(bucket, barType) {
            const params = new URLSearchParams(window.location.search);
            params.delete('page');

            const startDate = bucket.start.slice(0, 10);
            const beforeDate = bucket.end.slice(0, 10);

            if (barType === 'created') {
                params.set('CreatedAfter', startDate);
                params.set('CreatedBefore', beforeDate);
            } else {
                params.set('UpdatedAfter', startDate);
                params.set('UpdatedBefore', beforeDate);
            }
            return params;
        },

        get showAllUrl() {
            if (this.selectedBar === null) return '#';
            const bucket = this.buckets[this.selectedBar];
            if (!bucket) return '#';
            const params = this._buildDateParams(bucket, this.selectedBarType);
            return this.defaultView + '?' + params.toString();
        },

        async selectBar(index, barType) {
            // Toggle off if clicking same bar
            if (this.selectedBar === index && this.selectedBarType === barType) {
                // P3 fix: abort in-flight preview before closing
                if (this._previewAborter) {
                    this._previewAborter();
                    this._previewAborter = null;
                }
                this.closePreview();
                this.renderChart();
                return;
            }

            const bucket = this.buckets[index];
            if (!bucket) return;

            this.selectedBar = index;
            this.selectedBarType = barType;

            const count = bucket[barType] || 0;
            this.previewTitle = bucket.label + ' \u2014 ' + count + ' ' + barType;
            this.previewTotalCount = count;

            // Re-render chart to update selected state
            this.renderChart();

            // Fetch preview via .body suffix (returns body fragment, no full page layout)
            if (this._previewAborter) {
                this._previewAborter();
                this._previewAborter = null;
            }

            const params = this._buildDateParams(bucket, barType);
            params.set('pageSize', '20');

            try {
                const url = this.defaultView + '.body?' + params.toString();
                const { abort, ready } = abortableFetch(url);
                this._previewAborter = abort;

                const response = await ready;
                if (!response.ok) throw new Error('HTTP ' + response.status);

                this.previewHtml = await response.text();

                // Re-initialize lightbox so thumbnail clicks open the gallery
                this.$nextTick(() => {
                    window.Alpine?.store('lightbox')?.initFromDOM();
                });
            } catch (err) {
                if (err.name !== 'AbortError') {
                    console.error('Failed to load preview:', err);
                    this.previewHtml = '';
                }
            } finally {
                this._previewAborter = null;
            }
        },

        _yAxisTicks(maxCount) {
            // Generate ~4 nice round tick values from 0 to maxCount
            if (maxCount <= 0) return [];
            const rough = maxCount / 4;
            const magnitude = Math.pow(10, Math.floor(Math.log10(rough)));
            const nice = [1, 2, 5, 10].find(n => n * magnitude >= rough) * magnitude;
            const ticks = [];
            for (let v = 0; v <= maxCount; v += nice) {
                if (v === 0) continue;
                ticks.push({ value: v, label: v >= 1000 ? (v / 1000).toFixed(v % 1000 === 0 ? 0 : 1) + 'k' : String(v) });
            }
            return ticks;
        },

        _shortLabel(label, index) {
            if (this.buckets.length <= 10) return label;

            // Monthly format: "YYYY-MM" -> abbreviated
            const monthMatch = label.match(/^(\d{4})-(\d{2})$/);
            if (monthMatch) {
                const year = monthMatch[1];
                const month = monthMatch[2];
                const prevLabel = index > 0 ? this.buckets[index - 1].label : '';
                const prevYear = prevLabel.slice(0, 4);
                if (index === 0 || year !== prevYear) {
                    return year + '-' + month;
                }
                return month;
            }

            return label;
        },

        closePreview() {
            this.selectedBar = null;
            this.selectedBarType = null;
            this.previewHtml = '';
            this.previewTitle = '';
            this.previewTotalCount = 0;
        },
    };
}

import { abortableFetch } from '../index.js';

export function adminOverview() {
    return {
        // Server stats section
        serverStats: null,
        serverStatsLoading: true,
        serverStatsError: null,
        _serverStatsAborter: null,
        _pollInterval: null,

        // Data stats section
        dataStats: null,
        dataStatsLoading: true,
        dataStatsError: null,
        _dataStatsAborter: null,

        // Expensive stats section
        expensiveStats: null,
        expensiveStatsLoading: true,
        expensiveStatsError: null,
        _expensiveStatsAborter: null,

        init() {
            this.fetchServerStats();
            this.fetchDataStats();
            this.fetchExpensiveStats();

            // Poll server stats every 10 seconds
            this._pollInterval = setInterval(() => {
                this.fetchServerStats();
            }, 10000);
        },

        destroy() {
            if (this._pollInterval) {
                clearInterval(this._pollInterval);
                this._pollInterval = null;
            }
            if (this._serverStatsAborter) {
                this._serverStatsAborter();
                this._serverStatsAborter = null;
            }
            if (this._dataStatsAborter) {
                this._dataStatsAborter();
                this._dataStatsAborter = null;
            }
            if (this._expensiveStatsAborter) {
                this._expensiveStatsAborter();
                this._expensiveStatsAborter = null;
            }
        },

        fetchServerStats() {
            // Abort any in-flight request
            if (this._serverStatsAborter) {
                this._serverStatsAborter();
            }

            const { abort, ready } = abortableFetch('/v1/admin/server-stats');
            this._serverStatsAborter = abort;

            ready
                .then(response => {
                    if (!response.ok) throw new Error(`HTTP ${response.status}`);
                    return response.json();
                })
                .then(data => {
                    this.serverStats = data;
                    this.serverStatsError = null;
                    this.serverStatsLoading = false;
                })
                .catch(err => {
                    if (err.name !== 'AbortError') {
                        this.serverStatsError = err.message || 'Failed to load server stats';
                        this.serverStatsLoading = false;
                    }
                })
                .finally(() => {
                    this._serverStatsAborter = null;
                });
        },

        fetchDataStats() {
            if (this._dataStatsAborter) {
                this._dataStatsAborter();
            }

            const { abort, ready } = abortableFetch('/v1/admin/data-stats');
            this._dataStatsAborter = abort;

            ready
                .then(response => {
                    if (!response.ok) throw new Error(`HTTP ${response.status}`);
                    return response.json();
                })
                .then(data => {
                    this.dataStats = data;
                    this.dataStatsError = null;
                    this.dataStatsLoading = false;
                })
                .catch(err => {
                    if (err.name !== 'AbortError') {
                        this.dataStatsError = err.message || 'Failed to load data stats';
                        this.dataStatsLoading = false;
                    }
                })
                .finally(() => {
                    this._dataStatsAborter = null;
                });
        },

        fetchExpensiveStats() {
            if (this._expensiveStatsAborter) {
                this._expensiveStatsAborter();
            }

            const { abort, ready } = abortableFetch('/v1/admin/data-stats/expensive');
            this._expensiveStatsAborter = abort;

            ready
                .then(response => {
                    if (!response.ok) throw new Error(`HTTP ${response.status}`);
                    return response.json();
                })
                .then(data => {
                    this.expensiveStats = data;
                    this.expensiveStatsError = null;
                    this.expensiveStatsLoading = false;
                })
                .catch(err => {
                    if (err.name !== 'AbortError') {
                        this.expensiveStatsError = err.message || 'Failed to load detailed statistics';
                        this.expensiveStatsLoading = false;
                    }
                })
                .finally(() => {
                    this._expensiveStatsAborter = null;
                });
        },

        formatNumber(n) {
            if (n === null || n === undefined) return '0';
            if (n >= 1_000_000) {
                return (n / 1_000_000).toFixed(2).replace(/\.?0+$/, '') + 'M';
            }
            if (n >= 1_000) {
                return (n / 1_000).toFixed(1).replace(/\.?0+$/, '') + 'K';
            }
            return n.toLocaleString();
        },
    };
}

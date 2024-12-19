{% extends "/layouts/base.tpl" %}

{% block body %}
    <div x-data="{
            resultTable: document.createElement('div'),
            error: null,
            updated: null,
            loading: false,
            results: [],
            queryParams: {},
            query() {
                this.loading = true
                fetch('/v1/query/run?id={{ query.ID }}', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(this.queryParams) })
                .then(x => x.ok
                    ? x.json().then(json => {
                        this.resultTable = renderJsonTable(json);
                        this.error = null;
                        this.updated = new Date();
                        this.results = json;
                        window.results = json;
                    })
                    : x.text().then(e => {
                        this.results = [];
                        this.error = { message: e };
                        this.updated = null;
                        this.resultTable = document.createElement('div')
                    }))
                .catch(e => this.error = e)
                .finally(() => this.loading = false)
            }
        }">
        <code x-ref="query" class="bg-gray-100 mb-4 p-4 block">
            {{ query.Text }}
        </code>
        <div x-init="queryParams = parseQueryParams($refs.query.innerHTML)">
            <template x-for="(queryVal, queryParamName) in queryParams">
                <div>
                    <p x-text="queryParamName"></p>
                    <input class="mb-4" type="text" @input="e => queryParams[queryParamName] = getJSONValue(e.target.value)" @keyup="e => e.key === 'Enter' && query()" >
                </div>
            </template>
            <template x-if="!loading">
                <div>
                    <button
                            @click="query"
                            x-effect="$refs.output.replaceChildren(resultTable)"
                            type="submit"
                            class="
                            inline-flex justify-center
                            py-2 px-4
                            border border-transparent
                            shadow-sm text-sm font-medium rounded-md
                            text-white bg-green-600 hover:bg-green-700
                            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500
                        "
                    >
                        Run
                    </button>
                    <template x-if="updated">
                        <p class="mt-2">
                            Updated on
                            <span class="text-green-800" x-text="updated"></span>
                        </p>
                    </template>
                    <template x-if="!error && results && results.length > 0">
                        <div class="result-container mt-2 mb-2">
                            {% autoescape off %}
                            {{ query.Template }}
                            {% endautoescape %}
                        </div>
                    </template>
                    <div class="output mt-2" x-ref="output"></div>
                    <template x-if="error">
                        <div>
                            <h3>Something went wrong.</h3>
                            <p class="text-red-800" x-text="(error.message || 'unknown error')"></p>
                        </div>
                    </template>
                </div>
            </template>
        </div>

        <template x-if="loading">
            <div class="flex items-center justify-start space-x-2 animate-pulse">
                <div class="w-8 h-8 bg-blue-400 rounded-full"></div>
                <div class="w-8 h-8 bg-blue-400 rounded-full"></div>
                <div class="w-8 h-8 bg-blue-400 rounded-full"></div>
            </div>
        </template>
    </div>
{% endblock %}

{% block sidebar %}

{% endblock %}
{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=query.Text preview=false %}
{% endblock %}

{% block sidebar %}
    <button x-data="{}" x-on:click="
                () => fetch('/v1/query/run?id={{ query.ID }}', { method: 'POST', headers: {
                    'Content-Type': 'application/json'
                }, body: '{}' }).then(x => x.json()).then(json => document.querySelector('.output').appendChild(renderJsonTable(json)))"
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
    <div class="output"></div>
{% endblock %}
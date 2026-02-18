<div class="grid grid-cols-3 gap-3 sm:grid-cols-6 pb-2" role="group" aria-label="Display options">
    {% for option in options %}
        <a href="{{ option.Link }}"
                {% if option.Active %}aria-current="true"{% endif %}
                class="
                    border rounded-md py-1 flex items-center justify-center text-xs
                    {% if option.Active %} ring-2 ring-offset-2 ring-indigo-500 bg-indigo-600 border-transparent text-white hover:bg-indigo-700
                    {% else %} bg-white border-gray-200 text-gray-900 hover:bg-gray-50
                    {% endif %}
                "
        >
            <span>{{ option.Title }}</span>
        </a>
    {% endfor %}
</div>

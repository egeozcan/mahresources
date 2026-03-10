<div class="grid grid-cols-3 gap-3 sm:grid-cols-6 pb-2" role="group" aria-label="Display options">
    {% for option in options %}
        <a href="{{ option.Link }}"
                {% if option.Active %}aria-current="true"{% endif %}
                class="
                    border rounded-md py-1 flex items-center justify-center text-xs
                    {% if option.Active %} ring-2 ring-offset-2 ring-amber-600 bg-amber-700 border-transparent text-white hover:bg-amber-800
                    {% else %} bg-white border-stone-200 text-stone-900 hover:bg-stone-50
                    {% endif %}
                "
        >
            <span>{{ option.Title }}</span>
        </a>
    {% endfor %}
</div>

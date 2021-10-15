{% if ID %}<a href="/tag?id={{ ID }}">{% endif %}
    <div class="
        ml-2 text-xs inline-flex items-center font-bold leading-sm uppercase px-3 py-1
            {% if active %}bg-green-200 text-green-700 rounded-full
            {% else %} rounded-full bg-yellow-100 text-gray-700 border
            {% endif %}">
        {{ name }}
    </div>
{% if ID %}</a>{% endif %}
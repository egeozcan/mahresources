{% if ID %}
<a href="/tag?id={{ ID }}" class="inline-block">
    {% endif %}
    <span
    {% if reactiveActive %}
    :class="activeFilterTags.includes('{{ name|escapejs }}') ? 'bg-amber-100 text-amber-700' : 'bg-yellow-100 text-stone-700 border'"
    :data-active="activeFilterTags.includes('{{ name|escapejs }}') ? 'true' : 'false'"
    {% endif %}
    class="
    ml-2 text-xs inline-flex items-center font-bold leading-sm uppercase px-3 py-1 font-mono
        {% if reactiveActive %}rounded-full
        {% else %}
        {% if active %}bg-amber-100 text-amber-700 rounded-full
        {% else %} rounded-full bg-yellow-100 text-stone-700 border
        {% endif %}{% endif %}">
    {{ name }}{% if count %} ({{ count }}){% endif %}
</span>
    {% if ID %}
</a>
{% endif %}

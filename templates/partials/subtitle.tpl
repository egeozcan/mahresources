<h2 class="inline-block
    {% if small %}text-xl
    {% else %}text-2xl
    {% endif %}
    font-bold leading-7 text-gray-900 sm:truncate pb-2"
    title="{% if title %}{{ title }}{% else %}{{ alternativeTitle }}{% endif %}"
>
    {% if title %}{{ title }}{% else %}{{ alternativeTitle }}{% endif %}
</h2>
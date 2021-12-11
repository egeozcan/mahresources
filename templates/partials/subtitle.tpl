<h2 class="inline-block
    {% if small %}text-xl
    {% else %}text-2xl
    {% endif %}
    font-bold leading-7 text-gray-900 sm:truncate pb-2">
    {% if title %}{{ title }}{% else %}{{ alternativeTitle }}{% endif %}
</h2>
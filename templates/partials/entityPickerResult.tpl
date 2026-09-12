<div class="flex items-start gap-3 min-w-0">
    {% if entityType == 'resource' %}
    <img src="/v1/resource/preview?id={{ id }}" alt="" loading="lazy" class="w-20 h-20 object-contain bg-stone-100 flex-shrink-0">
    {% endif %}
    <div class="min-w-0">
        <a href="/{{ entityType }}?id={{ id }}" target="_blank" rel="noopener noreferrer" class="font-medium text-amber-800 underline break-words">{{ name }}<span class="sr-only"> (opens in a new tab)</span></a>
        {% if typeName %}<div class="text-sm text-stone-600">{{ typeName }}</div>{% endif %}
        {% if entity.Owner %}<div class="text-sm text-stone-600">Owner: {{ entity.Owner.Name }}</div>{% endif %}
        {% if entity.Tags %}<div class="flex flex-wrap gap-1 mt-1">{% for tag in entity.Tags %}<span class="text-xs bg-stone-100 px-1">{{ tag.Name }}</span>{% endfor %}</div>{% endif %}
    </div>
</div>

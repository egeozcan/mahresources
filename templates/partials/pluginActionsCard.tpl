{% if pluginCardActions %}
<div x-data="cardActionMenu()" @click.outside="close()" class="inline-block relative">
    <button @click="toggle()" class="card-badge card-badge--action" aria-label="More actions" aria-haspopup="true" :aria-expanded="open">
        &#x22EF;
    </button>
    <div x-show="open" x-cloak class="absolute right-0 mt-1 w-48 bg-white dark:bg-gray-800 shadow-lg rounded-md z-50 border border-gray-200 dark:border-gray-600" role="menu">
        {% for action in pluginCardActions %}
        <button @click="runAction({{ action|json }}, {{ entity.ID }}, '{{ entityType }}')"
                class="block w-full text-left px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-gray-700" role="menuitem">
            {{ action.Label }}
        </button>
        {% endfor %}
    </div>
</div>
{% endif %}

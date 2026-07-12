{% if pluginCardActions %}
<div x-data="cardActionMenu()" x-id="['plugin-actions-menu']" @click.outside="close()" class="inline-block relative">
    <button type="button" x-ref="trigger" @click="toggle()"
            @keydown.arrow-down.prevent="openAndFocus('first')"
            @keydown.arrow-up.prevent="openAndFocus('last')"
            class="card-badge card-badge--action focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-600 focus-visible:ring-offset-2"
            aria-label="Plugin actions for {{ entity.Name }}" aria-haspopup="menu" :aria-expanded="open" :aria-controls="$id('plugin-actions-menu')">
        &#x22EF;
    </button>
    <div x-show="open" x-cloak x-ref="menu" :id="$id('plugin-actions-menu')"
         @keydown="onMenuKeydown($event)" @keydown.tab="close()"
         class="absolute right-0 mt-1 w-48 overflow-hidden bg-white text-stone-900 dark:bg-stone-800 dark:text-stone-100 shadow-lg rounded-md z-50 border border-stone-200 dark:border-stone-600"
         role="menu" aria-label="Plugin actions">
        {% for action in pluginCardActions %}
        <button type="button" @click="runAction({{ action|json }}, {{ entity.ID }}, '{{ entityType }}')"
                class="block w-full text-left px-4 py-2 text-sm font-mono text-stone-900 dark:text-stone-100 hover:bg-stone-100 dark:hover:bg-stone-700 focus-visible:outline-none focus-visible:bg-amber-100 focus-visible:text-stone-950 dark:focus-visible:bg-amber-300 dark:focus-visible:text-stone-950"
                role="menuitem">
            {{ action.Label }}
        </button>
        {% endfor %}
    </div>
</div>
{% endif %}

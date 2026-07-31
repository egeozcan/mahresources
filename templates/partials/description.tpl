{% if description or descriptionEditUrl %}
<div x-data="descriptionEditor({ url: '{{ descriptionEditUrl }}{% if descriptionEditId %}?id={{ descriptionEditId }}{% endif %}' })"
    x-show="$store.savedSetting.localSettings.showDescriptions ?? true"
    class="description flex-1 relative"
     :class="{ 'lg:prose-xl prose font-sans bg-white border border-stone-200 rounded p-4 mb-2': !editing }"
    @click.away="editing && save({ event: $event })"
>
    <template x-if="!editing">
        {# Finding 149: the title was unconditional, so every card whose include #}
        {# passes no descriptionEditUrl (list cards, similar-resource cards, the #}
        {# relation halves) promised an editor that startEditing() refuses to    #}
        {# open. The handler is bound only where it can fire, so the element     #}
        {# does not claim an interaction it does not have either. #}
        <div class="contents"{% if descriptionEditUrl %} @dblclick="startEditing()" title="Double-click to edit"{% endif %} data-description-display>
            {% autoescape off %}
                {% if description %}
                    {% if !preview %}{% process_shortcodes description|markdown2|render_mentions descriptionEntity %}{% endif %}
                    {% if preview %}{{ description|markdown|render_mentions|truncatechars_html:250 }}{% endif %}
                {% else %}
                    <p class="text-stone-500 italic">No description</p>
                {% endif %}
            {% endautoescape %}
        </div>
    </template>
    {% if descriptionEditUrl %}
    <template x-if="editing">
        {# One element owns everything the editor renders, so focusout can #}
        {# tell "moved to Save" from "left the editor" (findings 15/53/87). #}
        <div class="contents" data-description-editor @keydown="onKeyDown($event)" @focusout="onFocusOut($event)">
            <textarea
                x-init="if (serverValue !== null) $el.value = serverValue"
                @keydown.escape.prevent.stop="cancel()"
                @keydown.ctrl.enter.prevent.stop="save()"
                @keydown.meta.enter.prevent.stop="save()"
                autofocus
                name="description"
                aria-label="Edit description"
                aria-describedby="description-edit-error"
                class="w-full"
            >{{ description }}</textarea>
            <div class="flex items-center gap-2 mt-2">
                <button type="button"
                        @mousedown.prevent
                        @click="save()"
                        :disabled="saving"
                        class="inline-flex justify-center py-1.5 px-3 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50">
                    Save
                </button>
                <button type="button"
                        @mousedown.prevent
                        @click="cancel()"
                        class="inline-flex justify-center py-1.5 px-3 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
                    Cancel
                </button>
                <span class="text-xs text-stone-500">Ctrl+Enter saves</span>
            </div>
            {# Not a live region: window.mahAnnounce already announces the #}
            {# failure assertively, and a second one would double-speak it. #}
            <p id="description-edit-error"
               data-description-error
               x-show="error"
               x-text="error"
               class="mt-2 text-sm text-red-700"></p>
        </div>
    </template>
    {% endif %}
</div>
{% endif %}

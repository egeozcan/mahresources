{% if description %}
<div x-data="() => ({ editing: false, descriptionEditUrl: '{{ descriptionEditUrl }}{% if descriptionEditId %}?id={{ descriptionEditId }}{% endif %}' })"
    x-show="$store.savedSetting.localSettings.showDescriptions ?? true"
    class="description flex-1 relative"
     :class="{ 'lg:prose-xl prose font-sans bg-stone-50 p-4 mb-2': !editing }"
>
    <template x-if="!editing">
        <div class="contents" @dblclick="editing = !!descriptionEditUrl" title="Double-click to edit">
            {% autoescape off %}
                {% if !preview %}{{ description|markdown2|render_mentions }}{% endif %}
                {% if preview %}{{ description|markdown|render_mentions|truncatechars_html:250 }}{% endif %}
            {% endautoescape %}
        </div>
    </template>
    {% if descriptionEditUrl %}
    <template x-if="editing">
        <div class="contents">
            <textarea
                @click.away="
                    const formData = new FormData();
                    formData.append('description', $el.value);
                    const clickedLink = $event && $event.target && $event.target.closest('a[href]');
                    fetch(descriptionEditUrl, { method: 'POST', body: formData })
                        .then(r => { if (r.ok && !clickedLink) location.reload(); })
                        .catch(e => console.error('Failed to save description:', e));
                "
                @keydown.escape="editing = false"
                autofocus
                name="description"
                aria-label="Edit description"
                class="w-full"
            >{{ description }}</textarea>
        </div>
    </template>
    {% endif %}
</div>
{% endif %}
{% if description or descriptionEditUrl %}
<div x-data="() => ({ editing: false, descriptionEditUrl: '{{ descriptionEditUrl }}{% if descriptionEditId %}?id={{ descriptionEditId }}{% endif %}' })"
    x-show="$store.savedSetting.localSettings.showDescriptions ?? true"
    class="description flex-1 relative"
     :class="{ 'lg:prose-xl prose font-sans bg-stone-50 p-4 mb-2': !editing }"
>
    <template x-if="!editing">
        <div class="contents" @dblclick="editing = !!descriptionEditUrl" title="Double-click to edit">
            {% autoescape off %}
                {% if description %}
                    {% if !preview %}{{ description|markdown2|render_mentions }}{% endif %}
                    {% if preview %}{{ description|markdown|render_mentions|truncatechars_html:250 }}{% endif %}
                {% else %}
                    <p class="text-stone-500 italic">No description</p>
                {% endif %}
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
                    const container = $el.closest('.description');
                    fetch(descriptionEditUrl, { method: 'POST', body: formData })
                        .then(r => {
                            if (r.ok && !clickedLink) {
                                location.reload();
                            } else if (!r.ok) {
                                console.error('Failed to save description: HTTP', r.status);
                                editing = false;
                                if (container) {
                                    container.style.transition = 'background-color 0.3s';
                                    container.style.backgroundColor = '#fee2e2';
                                    setTimeout(() => { container.style.backgroundColor = ''; }, 2000);
                                }
                            }
                        })
                        .catch(e => {
                            console.error('Failed to save description:', e);
                            editing = false;
                            if (container) {
                                container.style.transition = 'background-color 0.3s';
                                container.style.backgroundColor = '#fee2e2';
                                setTimeout(() => { container.style.backgroundColor = ''; }, 2000);
                            }
                        });
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

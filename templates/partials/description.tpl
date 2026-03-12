{% if description %}
<div x-data="() => ({ editing: false, descriptionEditUrl: '{{ descriptionEditUrl }}' })"
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
            <form x-ref="form" method="post" action="{{ descriptionEditUrl }}">
                <textarea @click.away="editing = false" autofocus name="description" aria-label="Edit description" class="w-full">{{ description }}</textarea>
            </form>
        </div>
    </template>
    {% endif %}
</div>
{% endif %}
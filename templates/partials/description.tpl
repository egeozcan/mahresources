{% if description %}
<div x-data="() => ({ editing: false, descriptionEditUrl: '{{ descriptionEditUrl }}' })"
    x-show="$store.savedSetting.localSettings.showDescriptions ?? true"
    class="description flex-1 relative"
     :class="{ 'lg:prose-xl prose bg-gray-50 p-4 mb-2': !editing }"
>
    <template x-if="!editing">
        <div class="contents" @dblclick="editing = !!descriptionEditUrl">
            {% autoescape off %}
                {% if !preview %}{{ description|markdown2 }}{% endif %}
                {% if preview %}{{ description|markdown|truncatechars_html:250 }}{% endif %}
            {% endautoescape %}
        </div>
    </template>
    {% if descriptionEditUrl %}
    <template x-if="editing">
        <div class="contents">
            <form x-ref="form" method="post" action="{{ descriptionEditUrl }}">
                <textarea @click.away="editing = false" autofocus name="description" class="w-full">{{ description }}</textarea>
            </form>
        </div>
    </template>
    {% endif %}
</div>
{% endif %}
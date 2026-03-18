{% if pageTitle != nil %}
<section class="title border-b-2 border-light-blue-400 pb-3">
    {% if breadcrumb && breadcrumb.HomeUrl %}
        {% include "/partials/breadcrumb.tpl" with HomeName=breadcrumb.HomeName HomeUrl=breadcrumb.HomeUrl Entries=breadcrumb.Entries %}
    {% endif %}
    <div class="flex items-end flex-1 min-w-0 gap-3 {% if breadcrumb && breadcrumb.HomeUrl %}mt-3{% endif %}">
        <h1 class="flex flex-col items-start gap-1 flex-1 min-w-0 text-2xl font-bold leading-7 text-stone-900 sm:text-3xl">
            {% if prefix %}<small class="break-words px-2 text-xs leading-5 font-semibold font-mono rounded-full bg-amber-100 text-amber-700">{{ prefix }}</small>{% endif %}
            {% if mainEntityType && mainEntity %}
                <span class="break-all"><inline-edit post="/v1/{{ mainEntityType }}/editName?id={{ mainEntity.ID }}" name="name">{{ pageTitle }}</inline-edit></span>
            {% else %}
                <span class="break-all">{{ pageTitle }}</span>
            {% endif %}
        </h1>
        {% if action %}
        <a href="{{ action.Url }}" class="
            ml-4 inline-flex items-center
            px-4 py-2
            border border-stone-300 rounded-md
            shadow-sm text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            {{ action.Name }}
        </a>
        {% endif %}
        {% if secondaryAction %}
        <a href="{{ secondaryAction.Url }}"
           class="
            ml-4 inline-flex items-center
            px-4 py-2
            border border-stone-300 rounded-md
            shadow-sm text-sm font-mono font-medium text-stone-700 bg-white hover:bg-stone-50
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            {{ secondaryAction.Name }}
        </a>
        {% endif %}
        {% if deleteAction %}
            {% include "/partials/form/deleteButton.tpl" with action=deleteAction.Url text=deleteAction.Name id=deleteAction.ID %}
        {% endif %}
    </div>
</section>
{% endif %}
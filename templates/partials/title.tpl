{% if pageTitle != nil %}
<section class="title md:flex md:items-center md:justify-between border-b-2 border-light-blue-400 pb-3">
    <div class="flex items-end flex-1 min-w-0 gap-3">
        <h2 class="flex flex-col items-start gap-2 text-2xl font-bold leading-7 text-gray-900 sm:text-3xl sm:truncate">
            {% if prefix %}<small class="px-2 text-xs leading-5 font-semibold rounded-full bg-green-100 text-green-800 ">{{ prefix }}</small>{% endif %}
            <span>{{ pageTitle }}</span>
        </h2>
        {% if action %}
        <a href="{{ action.Url }}" class="
            ml-4 inline-flex items-center
            px-4 py-2
            border border-gray-300 rounded-md
            shadow-sm text-sm font-medium text-white bg-green-500 hover:bg-green-700
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-900">
            {{ action.Name }}
        </a>
        {% endif %}
        {% if secondaryAction %}
        <a href="{{ secondaryAction.Url }}"
           class="
            ml-4 inline-flex items-center
            px-4 py-2
            border border-gray-300 rounded-md
            shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            {{ secondaryAction.Name }}
        </a>
        {% endif %}
        {% if deleteAction %}
            {% include "/partials/form/deleteButton.tpl" with action=deleteAction.Url text=deleteAction.Name id=deleteAction.ID %}
        {% endif %}
    </div>
</section>
{% endif %}
{% if pageTitle != nil %}
<section class="title md:flex md:items-center md:justify-between border-b-2 border-light-blue-400 pb-3">
    <div class="flex flex-1 min-w-0">
        <h2 class="inline-block text-2xl font-bold leading-7 text-gray-900 sm:text-3xl sm:truncate">
            {{ pageTitle }}
        </h2>
        {% if action != nil %}
        <a href="{{ action.Url }}" class="ml-4 inline-flex items-center px-4 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            {{ action.Name }}
        </a>
        {% endif %}
    </div>
</section>
{% endif %}
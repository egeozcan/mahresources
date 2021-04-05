{% if pageTitle != nil %}
<div class="md:flex md:items-center md:justify-between border-b-2 border-light-blue-400 mb-2 pb-4">
    <div class="flex-1 min-w-0">
        <h2 class="text-2xl font-bold leading-7 text-gray-900 sm:text-3xl sm:truncate">
            {{ pageTitle }}
        </h2>
    </div>
    <div class="mt-4 flex md:mt-0 md:ml-4">
        {% if action != nil %}
        <a href="{{ action.Url }}" class="inline-flex items-center px-4 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            {{ action.Name }}
        </a>
        {% endif %}
        <!--<button type="button" class="ml-3 inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            Publish
        </button>-->
    </div>
</div>
{% endif %}
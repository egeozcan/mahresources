{% if pageTitle != nil %}
<div class="md:flex md:items-center md:justify-between border-b-2 border-light-blue-400 mb-2 pb-4">
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
    {% if search != nil %}
    <div class="mt-4 flex md:mt-0 md:ml-4">
        <form>
            {% for queryParam, values in queryValues %}
                {% if queryParam != "Name" && queryParam != search.QueryParamName %}
                    {% for value in values %}
                        <input type="hidden" name="{{ queryParam }}" value="{{ value }}">
                    {% endfor %}
                {% endif %}
            {% endfor %}
            <div class="max-w-lg w-full lg:max-w-xs">
                <label for="search" class="sr-only">{{ search.Text }}</label>
                <div class="relative">
                    <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                        <svg class="h-5 w-5 text-gray-400" x-description="Heroicon name: solid/search" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                            <path fill-rule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z" clip-rule="evenodd"></path>
                        </svg>
                    </div>
                    <input id="search" name="{{ search.QueryParamName }}" value="{{ queryValues.Name.0 }}" class="block w-full pl-10 pr-3 py-2 border border-gray-300 rounded-md leading-5 bg-white shadow-sm placeholder-gray-500 focus:outline-none focus:placeholder-gray-400 focus:ring-1 focus:ring-blue-600 focus:border-blue-600 sm:text-sm" placeholder="{{ search.Text }}" type="search">
                </div>
            </div>
        </form>
        {% endif %}
        <!--<button type="button" class="ml-3 inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            Publish
        </button>-->
    </div>
</div>
{% endif %}
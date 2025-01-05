{% if pagination != nil %}
<nav aria-label="Pagination" class="border-t border-gray-200 px-4 flex items-center justify-between sm:px-0 pb-2">
    <div class="-mt-px w-0 flex-1 flex {% if pagination.PrevLink.Link == '' %}invisible{% endif %}">
        {% if pagination.PrevLink.Link != '' %}
        <a href="{{ pagination.PrevLink.Link }}" class="border-t-2 border-transparent pt-4 pr-1 inline-flex items-center text-sm font-medium text-gray-500 hover:text-gray-700 hover:border-gray-300">
            <svg class="mr-3 h-5 w-5 text-gray-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                <path fill-rule="evenodd" d="M7.707 14.707a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 1.414L5.414 9H17a1 1 0 110 2H5.414l2.293 2.293a1 1 0 010 1.414z" clip-rule="evenodd" />
            </svg>
            {{ pagination.PrevLink.Display }}
        </a>
        {% endif %}
    </div>
    <div class="hidden md:-mt-px md:flex">
        {% for page in pagination.Entries %}
        {% if page.Link != "" && !page.Selected %}
        <a href="{{ page.Link }}" class="border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 border-t-2 pt-4 px-4 inline-flex items-center text-sm font-medium">
            {{ page.Display }}
        </a>
        {% endif %}
        {% if page.Link != "" && page.Selected %}
        <a href="{{ page.Link }}" class="border-indigo-500 text-indigo-600 border-t-2 pt-4 px-4 inline-flex items-center text-sm font-medium" aria-current="page">
            {{ page.Display }}
        </a>
        {% endif %}
        {% if page.Link == "" %}
        <span class="border-transparent text-gray-500 border-t-2 pt-4 px-4 inline-flex items-center text-sm font-medium">
                    ...
                </span>
        {% endif %}
        {% endfor %}
    </div>
    <div class="-mt-px w-0 flex-1 flex justify-end {% if pagination.NextLink.Link == '' %}invisible{% endif %}">
        {% if pagination.NextLink.Link != '' %}
        <a href="{{ pagination.NextLink.Link }}" class="border-t-2 border-transparent pt-4 pl-1 inline-flex items-center text-sm font-medium text-gray-500 hover:text-gray-700 hover:border-gray-300">
            {{ pagination.NextLink.Display }}
            <svg class="ml-3 h-5 w-5 text-gray-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                <path fill-rule="evenodd" d="M12.293 5.293a1 1 0 011.414 0l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414-1.414L14.586 11H3a1 1 0 110-2h11.586l-2.293-2.293a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
        </a>
        {% endif %}
    </div>
</nav>
{% endif %}
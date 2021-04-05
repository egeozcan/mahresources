<a class="no-underline" href="{{ tag.Link }}">
    <div class="ml-2 text-xs inline-flex items-center font-bold leading-sm uppercase px-3 py-1 {% if tag.Active %}bg-green-200 text-green-700 rounded-full {% else %} rounded-full bg-white text-gray-700 border {% endif %}">
        {{ tag.Name }}
    </div>
</a>
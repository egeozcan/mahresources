{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for album in albums %}
        {% include "./partials/album.tpl" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
    <form>
        {% for value in queryValues.tags %}
            <input type="hidden" name="tag" value="{{ value }}">
        {% endfor %}
        <label for="search"
               class="block text-sm font-medium text-gray-700">
            Filter by name
        </label>
        <input type="text"
               name="Name"
               value="{{ queryValues.Name.0 }}"
               id="search"
               autocomplete="album-name"
               class="mt-1 focus:ring-indigo-500 focus:border-indigo-500 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md">
        <div class="flex justify-end mt-3">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                Search
            </button>
        </div>
    </form>
{% endblock %}
{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for album in albums %}
        {% include "./partials/album.tpl" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
    <h3 class="font-regular text-base md:text-lg leading-snug truncate">Filter</h3>
    <form class="mt-5">
        {% include "./partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags.SelectedTags id="autocompleter"|nanoid %}
        <label for="search"
               class="block text-sm font-medium text-gray-700 mt-2">
            Name
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
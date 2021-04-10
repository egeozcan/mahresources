{% with id=name|nanoid %}
<label for="search"
       class="block text-sm font-medium text-gray-700 mt-2">
    {{ label }}
</label>
<input type="search"
       name="{{ name }}"
       value="{{ value }}"
       id="search"
       autocomplete="{{ name }}"
       class="mt-1 focus:ring-indigo-500 focus:border-indigo-500 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md">
{% endwith %}
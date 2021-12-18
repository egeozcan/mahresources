{% with id=name|nanoid %}
<label for="{{ name }}"
       class="block text-sm font-medium text-gray-700 mt-2">
    {{ label }}
</label>
<select x-data type="search"
       name="{{ name }}"
       id="{{ name }}"
       autocomplete="{{ name }}"
       :value="new URL(window.location).searchParams.getAll('{{ name }}')"
       class="mt-1 focus:ring-indigo-500 focus:border-indigo-500 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md">
    {% for value in values %}
    <option value="{{ value.Value }} desc">{{ value.Name }} &#8595;</option>
    <option value="{{ value.Value }} asc">{{ value.Name }} &#8593;</option>
    {% endfor %}
</select>
{% endwith %}
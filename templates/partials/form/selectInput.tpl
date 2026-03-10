{% with id=getName(name) %}
<label for="{{ name }}"
       class="block text-sm font-mono font-medium text-stone-700 mt-2">
    {{ label }}
</label>
<select x-data type="search"
       name="{{ name }}"
       id="{{ name }}"
       autocomplete="off"
       :value="new URL(window.location).searchParams.getAll('{{ name }}')"
       class="mt-1 focus:ring-amber-600 focus:border-amber-600 block w-full shadow-sm sm:text-sm border-stone-300 rounded-md">
    {% for value in values %}
    <option value="{{ value.Value }} desc">{{ value.Name }} &#8595;</option>
    <option value="{{ value.Value }} asc">{{ value.Name }} &#8593;</option>
    {% endfor %}
</select>
{% endwith %}
{% with id=getName(name) %}
<label for="{{ name }}"
       class="block text-xs font-mono font-medium text-stone-600 mt-2">
    {{ label }}
</label>
<select x-data type="search"
       name="{{ name }}"
       id="{{ name }}"
       autocomplete="off"
       :value="new URL(window.location).searchParams.getAll('{{ name }}')"
       class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
    {% for value in values %}
    <option value="{{ value.Value }} desc">{{ value.Name }} &#8595;</option>
    <option value="{{ value.Value }} asc">{{ value.Name }} &#8593;</option>
    {% endfor %}
</select>
{% endwith %}
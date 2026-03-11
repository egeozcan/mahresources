{% with id=getName(name) %}
<label for="{{ name }}"
       class="block text-xs font-mono font-medium text-stone-600 mt-2">
    {{ label }}
</label>
<input type="search"
       name="{{ name }}"
       value="{{ value }}"
       id="{{ name }}"
       autocomplete="off"
       class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
{% endwith %}
{% with id=getName(name) %}
<label for="{{ name }}"
       class="block text-sm font-mono font-medium text-stone-700 mt-2">
    {{ label }}
</label>
<input type="search"
       name="{{ name }}"
       value="{{ value }}"
       id="{{ name }}"
       autocomplete="off"
       class="mt-1 focus:ring-amber-600 focus:border-amber-600 block w-full shadow-sm sm:text-sm border-stone-300 rounded-md">
{% endwith %}
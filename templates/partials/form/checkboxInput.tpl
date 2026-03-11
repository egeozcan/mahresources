<label for="{{ id }}" class="flex items-center gap-2 mt-1 cursor-pointer group">
    <input id="{{ id }}" value="1" name="{{ name }}" type="checkbox" {% if value %} checked="checked" {% endif %} class="focus:ring-1 focus:ring-amber-600 h-3.5 w-3.5 text-amber-700 border-stone-300 rounded">
    <span class="text-xs font-mono font-medium text-stone-600 group-hover:text-stone-700">{{ label }}</span>
</label>
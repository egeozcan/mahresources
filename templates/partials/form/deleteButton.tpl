<form method="post" x-data="confirmAction()" x-bind="events" action="{{ action }}">
    <input type="submit" class="
    inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-2 px-4
    {% endif %}
    border border-transparent
    shadow-sm text-sm font-medium rounded-md
    text-white bg-red-600 hover:bg-red-700
    focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
    value="{% if text %}{{ text }}{% else %}Delete{% endif %}"
    >
    {% if id %}
    <input type="hidden" name="id" value="{{ id }}">
    {% endif %}
</form>
{# `confirmMessage` names the action and its blast radius. Without it the form   #}
{# falls back to confirmAction's default, which is deliberately generic — a page #}
{# that knows what will be destroyed should say so (findings 98/140).            #}
<form method="post" x-data="confirmAction({% if confirmMessage %}{ message: '{{ confirmMessage|escapejs }}' }{% endif %})" x-bind="events" action="{{ action }}">
    <input type="submit" class="
    inline-flex justify-center
    {% if small %}py-1 px-2
    {% else %}py-2 px-4
    {% endif %}
    border border-transparent
    shadow-sm text-sm font-mono font-medium rounded-md
    text-white bg-red-700 hover:bg-red-800
    focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600"
    value="{% if text %}{{ text }}{% else %}Delete{% endif %}"
    >
    {% if id %}
    <input type="hidden" name="id" value="{{ id }}">
    {% endif %}
</form>
{% if description %}
<div class="description flex-1 prose lg:prose-xl bg-gray-50 p-4 mb-2">
    {% autoescape off %}
        {% if !preview %}{{ description|markdown2 }}{% endif %}
        {% if preview %}{{ description|markdown|truncatechars_html:250 }}{% endif %}
    {% endautoescape %}
</div>
{% endif %}
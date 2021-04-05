{% macro sub_tags(tags, subTags) export %}
    {% for subtag in subTags %}
        {% include "../partials/tag.tpl" with tag=tags.GetTag(subtag.ID) %}
    {% endfor %}
{% endmacro %}
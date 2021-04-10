{% macro sub_tags(tags, subTags) export %}
    {% for subtag in subTags %}
        {% include "../partials/tag.tpl" with tag=tags.GetRelation(subtag.ID) %}
    {% endfor %}
{% endmacro %}
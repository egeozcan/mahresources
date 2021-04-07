{% macro query_param_hidden_inputs(queryValues, paramToAvoid) export %}
    {% for queryParam, values in queryValues %}
        {% if queryParam != "Name" && queryParam != paramToAvoid %}
            {% for value in values %}
                <input type="hidden" name="{{ queryParam }}" value="{{ value }}">
            {% endfor %}
        {% endif %}
    {% endfor %}
{% endmacro %}
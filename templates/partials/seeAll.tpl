{# with entities= formAction= formID= formParamName= templateName= addAction= addFormParamName= addFormSecondParamName= addFormSecondParamValue= #}
    <section class="mb-6">
        <div class="flex gap-4 items-center mb-4">
            {% include "partials/subtitle.tpl" with small=true title=subtitle %}
            {% if entities && formParamName %}
                <form action="{{ formAction }}">
                    <input type="hidden" name="{{ formParamName }}" value="{{ formID }}">
                    {% include "partials/form/searchButton.tpl" with small=true text="See All" %}
                </form>
            {% endif %}
            {% if addAction %}
                <form action="{{ addAction }}">
                    <input type="hidden" name="{% if addFormParamName %}{{ addFormParamName }}{% else %}{{ formParamName }}{% endif %}" value="{{ formID }}">
                    {% if addFormSecondParamName %}
                    <input type="hidden" name="{{ addFormSecondParamName }}" value="{{ addFormSecondParamValue }}">
                    {% endif %}
                    {% include "partials/form/addButton.tpl" with small=true %}
                </form>
            {% endif %}
        </div>
        {% if entities %}
        <div class="list-container">
            {% for entity in entities %}
                {% include partial(templateName) %}
            {% endfor %}
        </div>
        {% else %}
        <p>None found.</p>
        {% endif %}
    </section>
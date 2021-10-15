{# with entities= formAction= formID= formParamName= entityName= #}
{% if entities %}
    <div class="flex gap-4 items-center mb-4">
        {% include "partials/subtitle.tpl" with title=subtitle %}
        <form action="{{ formAction }}">
            <input type="hidden" name="{{ formParamName }}" value="{{ formID }}">
            {% include "partials/form/searchButton.tpl" with text="See All" %}
        </form>
    </div>
    <section class="note-container">
        {% for entity in entities %}
            {% include partial(entityName) %}
        {% endfor %}
    </section>
{% endif %}
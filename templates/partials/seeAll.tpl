{# with entities= formAction= formID= formParamName= templateName= addAction= addFormParamName= addFormSecondParamName= addFormSecondParamValue= #}
    <div class="detail-panel">
        <div class="detail-panel-header">
            <h3 class="detail-panel-title">{{ subtitle }}</h3>
            <div class="detail-panel-actions">
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
        </div>
        <div class="detail-panel-body">
            {% if entities %}
            <div class="list-container"{% if templateName == "resource" and formAction and formParamName and formID %} data-lightbox-source="{{ formAction }}" data-lightbox-param-name="{{ formParamName }}" data-lightbox-param-value="{{ formID }}"{% endif %}>
                {% for entity in entities %}
                    {% include partial(templateName) %}
                {% endfor %}
            </div>
            {% else %}
            <div class="detail-empty">No {{ subtitle|lower }} found</div>
            {% endif %}
        </div>
    </div>

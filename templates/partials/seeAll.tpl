{# with entities= formAction= formID= formParamName= templateName= addAction= addFormParamName= addFormSecondParamName= addFormSecondParamValue= showUntaggedLink= #}
    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">{{ subtitle }}</h2>
            <div class="detail-panel-actions">
                {% if entities && formParamName %}
                    <form action="{{ formAction }}">
                        <input type="hidden" name="{{ formParamName }}" value="{{ formID }}">
                        {% include "partials/form/searchButton.tpl" with small=true text="See All" %}
                    </form>
                {% endif %}
                {% if showUntaggedLink && formParamName %}
                    <a href="/resources/details?{{ formParamName }}={{ formID }}&Untagged=1" class="inline-flex justify-center py-1 px-2 border border-stone-300 text-xs font-mono font-semibold tracking-wide rounded text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 transition-colors duration-100">Tag untagged</a>
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
                    {% include partial(templateName) with tagBaseUrl=formAction %}
                {% endfor %}
            </div>
            {% else %}
            <div class="detail-empty">No {{ subtitle|lower }} found</div>
            {% endif %}
        </div>
    </div>

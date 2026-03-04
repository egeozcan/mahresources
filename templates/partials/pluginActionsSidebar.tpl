{% if pluginDetailActions %}
<div x-data class="sidebar-section" role="group" aria-label="Plugin actions">
    <h4 class="sidebar-section-title">Plugin Actions</h4>
    {% for action in pluginDetailActions %}
    <button class="sidebar-action-btn plugin-action-btn"
            @click="$dispatch('plugin-action-open', Object.assign({}, {{ action|json }}, { plugin: '{{ action.PluginName }}', action: '{{ action.ID }}', entityIds: [{{ entityId }}], entityType: '{{ entityType }}' }))">
        {{ action.Label }}
    </button>
    {% endfor %}
</div>
{% endif %}

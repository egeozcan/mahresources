{% if owner %}
<details class="detail-collapsible mb-4">
    <summary>Owner: {{ owner.GetName() }}</summary>
    <div class="detail-panel-body">
        {% include "/partials/group.tpl" with entity=owner %}
    </div>
</details>
{% endif %}

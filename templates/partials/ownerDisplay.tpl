{% if owner %}
<details class="mb-6">
    <summary class="bg-stone-100 shadow rounded-lg block w-full p-4 text-left cursor-pointer select-none">Owner: {{ owner.GetName() }}</summary>
    <div class="p-4 border-dashed border-4 border-stone-100 border-t-0">
        {% include "/partials/group.tpl" with entity=owner %}
    </div>
</details>
{% endif %}
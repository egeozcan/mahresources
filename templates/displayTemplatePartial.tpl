{% extends "/layouts/base.tpl" %}

{% block body %}

{# Name is shown once via the title-bar h1 (mainEntity/mainEntityType set by the provider). #}
{% include "/partials/description.tpl" with description=templatePartial.Description descriptionEntity=templatePartial descriptionEditUrl="/v1/templatePartial/editDescription" descriptionEditId=templatePartial.ID %}

<div class="meta-strip">
    <div class="meta-strip-item">
        <span class="meta-strip-label">Reference</span>
        <span class="meta-strip-value"><code>[partial name="{{ templatePartial.Name }}"]</code></span>
    </div>
</div>

<section class="mt-6">
    <h2 class="text-base font-semibold font-mono text-stone-800 mb-2">Content</h2>
    <pre class="bg-stone-50 border border-stone-200 rounded-md p-4 text-sm leading-relaxed overflow-x-auto"><code>{{ templatePartial.Content }}</code></pre>
</section>

{% endblock %}

{% block sidebar %}
{% endblock %}

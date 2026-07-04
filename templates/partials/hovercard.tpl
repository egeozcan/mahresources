{# Hover-card fragment (Phase 6 item 3). hoverEntity is a scoped-loaded Group/Resource/Note (or nil, fail-closed). Reuses the list-card CustomAvatar/CustomSummary machinery in a compact form. Injected via innerHTML; Alpine.initTree is called on it client-side so entity-scoped directives hydrate. #}
{% if hoverEntity %}
<div class="hovercard-card" x-data='{ "entity": {{ hoverEntity|json }} }'>
    <div class="flex items-start gap-3">
        <div class="hovercard-avatar flex-shrink-0">
            {% if hoverType == "group" %}
                {% process_shortcodes hoverEntity.Category.CustomAvatar hoverEntity %}
                {% if not hoverEntity.Category.CustomAvatar %}{% include "partials/avatar.tpl" with initials=hoverEntity.Initials() %}{% endif %}
            {% elif hoverType == "note" %}
                {% process_shortcodes hoverEntity.NoteType.CustomAvatar hoverEntity %}
                {% if not hoverEntity.NoteType.CustomAvatar %}{% include "partials/avatar.tpl" with initials=hoverEntity.Initials() %}{% endif %}
            {% elif hoverType == "resource" %}
                <img class="hovercard-thumb w-14 h-14 rounded object-cover bg-stone-100" height="56" width="56" src="/v1/resource/preview?id={{ hoverEntity.ID }}&height=120&v={{ hoverEntity.Hash }}" alt="">
            {% endif %}
        </div>
        <div class="min-w-0 flex-1">
            <a class="hovercard-title block font-semibold text-stone-900 truncate hover:underline" href="/{{ hoverType }}?id={{ hoverEntity.ID }}">{{ hoverEntity.GetName() }}</a>
            <div class="hovercard-type text-xs text-stone-500 truncate">
                {% if hoverType == "group" && hoverEntity.Category %}{{ hoverEntity.Category.Name }}{% endif %}
                {% if hoverType == "note" && hoverEntity.NoteType %}{{ hoverEntity.NoteType.Name }}{% endif %}
                {% if hoverType == "resource" && hoverEntity.ResourceCategory %}{{ hoverEntity.ResourceCategory.Name }}{% endif %}
            </div>
        </div>
    </div>

    <div class="hovercard-summary mt-2 text-sm text-stone-700">
        {% if hoverType == "group" %}{% process_shortcodes hoverEntity.Category.CustomSummary hoverEntity %}{% endif %}
        {% if hoverType == "note" %}{% process_shortcodes hoverEntity.NoteType.CustomSummary hoverEntity %}{% endif %}
        {% if hoverType == "resource" %}{% process_shortcodes hoverEntity.ResourceCategory.CustomSummary hoverEntity %}{% endif %}
    </div>

    {% if hoverEntity.Description %}
    <div class="hovercard-desc mt-1 text-xs text-stone-500">{{ hoverEntity.Description|truncatechars:160 }}</div>
    {% endif %}
</div>
{% else %}
<div class="hovercard-empty text-sm text-stone-500 italic">Preview unavailable</div>
{% endif %}

{% if pageTitle != nil %}
<section class="title border-b-2 border-light-blue-400 pb-3">
    {% if breadcrumb && breadcrumb.HomeUrl %}
        {% include "/partials/breadcrumb.tpl" with HomeName=breadcrumb.HomeName HomeUrl=breadcrumb.HomeUrl Entries=breadcrumb.Entries %}
    {% endif %}
    {# Finding 89: this row was flex-nowrap, so at 390px a long title was crushed #}
    {# into 166px of a 358px container and grew to 500px tall — about fifteen      #}
    {# one-or-two-word lines — with the edit pencil floating in the empty space to #}
    {# its right and Edit/Delete pushed ~450px down the page. flex-wrap lets the   #}
    {# actions take their own row; at desktop the content still fits on one.       #}
    <div class="flex flex-wrap items-end flex-1 min-w-0 gap-3 {% if breadcrumb && breadcrumb.HomeUrl %}mt-3{% endif %}">
        {# Finding 89, and finding 141 with it: flex-wrap on the row above is not #}
        {# enough, because `flex-1 min-w-0` lets the heading shrink to 166px of a  #}
        {# 358px container instead of forcing the actions onto their own line — so #}
        {# a long title still grew to 500px tall in a narrow column with the edit  #}
        {# pencil floating beside it. basis-full below sm makes the heading claim  #}
        {# the whole row; sm:basis-0 restores flex-1's basis at desktop.           #}
        {# 141 is downstream of this: the inline-edit input is width:100% of its   #}
        {# host, and the host can be no wider than this heading.                   #}
        <h1 class="flex flex-col items-start gap-1 flex-1 min-w-0 basis-full sm:basis-0 text-2xl font-bold leading-7 text-stone-900 sm:text-3xl">
            {% if prefix %}<small class="break-words px-2 text-xs leading-5 font-semibold font-mono rounded-full bg-amber-100 text-amber-700">{{ prefix }}</small>{% endif %}
            {% if mainEntityType && mainEntity %}
                {# Finding 64/59: an entity whose Name is empty — a relation, because #}
                {# the create form does not require one — left this <h1> holding only #}
                {# an empty <inline-edit>, so the page had a blank top-level heading #}
                {# next to a floating pencil. pageTitle is what <title> already #}
                {# computes ("Relation from X to Y"), so the heading falls back to it. #}
                {# inline-edit shows the placeholder but keeps its value empty, so #}
                {# opening the editor does not prefill the fallback as a real name. #}
                {# w-full: the h1 is `flex flex-col items-start`, so this span was #}
                {# shrink-to-fit and the inline-edit host inside it could only be   #}
                {# as wide as the name — 267px of a 358px heading even after the    #}
                {# heading itself was widened. The editor's input is width:100% of  #}
                {# the host, so this is the last link in finding 141's chain.       #}
                <span class="break-words w-full"><inline-edit post="/v1/{{ mainEntityType }}/editName?id={{ mainEntity.ID }}" name="name"{% if mainEntity.Name %}{% else %} value-is-placeholder{% endif %}>{% if mainEntity.Name %}{{ mainEntity.Name }}{% else %}{{ pageTitle }}{% endif %}</inline-edit></span>
            {% else %}
                {# Finding 156: a detail page whose entity has no inline-rename    #}
                {# route (the template partial — a rename would break every        #}
                {# [partial name=…] pointing at it) fell back to pageTitle here,   #}
                {# and pageTitle is also what <title> uses, so it carries the type #}
                {# ("Template Partial: status-badge"). Next to the type pill that  #}
                {# printed the type twice. headingTitle lets such a page name the  #}
                {# heading separately from the document title.                     #}
                <span class="break-words">{% if headingTitle %}{{ headingTitle }}{% else %}{{ pageTitle }}{% endif %}</span>
            {% endif %}
        </h1>
        {% if action %}
        <a href="{{ action.Url }}" class="
            ml-4 inline-flex items-center
            px-4 py-2
            border border-stone-300 rounded-md
            shadow-sm text-sm font-mono font-medium text-white bg-amber-700 hover:bg-amber-800
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            {{ action.Name }}
        </a>
        {% endif %}
        {% if secondaryAction %}
        <a href="{{ secondaryAction.Url }}"
           class="
            ml-4 inline-flex items-center
            px-4 py-2
            border border-stone-300 rounded-md
            shadow-sm text-sm font-mono font-medium text-stone-700 bg-white hover:bg-stone-50
            focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            {{ secondaryAction.Name }}
        </a>
        {% endif %}
        {% if deleteAction %}
            {% include "/partials/form/deleteButton.tpl" with action=deleteAction.Url text=deleteAction.Name id=deleteAction.ID %}
        {% endif %}
    </div>
</section>
{% endif %}
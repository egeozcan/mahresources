{# The shared zero-result line for every server-rendered list view (WS6,      #}
{# findings 54 and 68/77/146). Before this, twelve list templates rendered a  #}
{# silently blank content column when a filter matched nothing, while nine    #}
{# others had their own copy of the markup below.                             #}
{#                                                                            #}
{# Parameters:                                                                #}
{#   label     — the plural noun, lower case, e.g. "resources"                #}
{#   createUrl — optional; omitted for entities with no create page           #}
{#                                                                            #}
{# hasActiveFilter and clearFiltersUrl come from StaticTemplateCtx, so this   #}
{# works in any template route without the provider knowing about it.         #}
<div class="detail-empty">
    {% if hasActiveFilter %}
        No {{ label }} match these filters. <a href="{{ clearFiltersUrl }}" class="text-amber-700 hover:text-amber-900 underline">Clear filters</a>.
    {% else %}
        No {{ label }} yet.{% if createUrl %} <a href="{{ createUrl }}" class="text-amber-700 hover:text-amber-900 underline">Create one</a>.{% endif %}
    {% endif %}
</div>

{# The entity every slot preview renders against. tpEntityType, tpCarrierId, tpGeneratePath come from the enclosing with; tpCarrierId is null on the create form. #}
<div class="rounded-md border border-stone-300 bg-stone-50 p-3 space-y-2" data-template-preview-context
     x-data="templatePreviewContext({ entityType: '{{ tpEntityType }}', generatePath: '{{ tpGeneratePath }}', categoryId: {{ tpCarrierId|default:"null" }} })">
    <div class="flex flex-wrap items-start gap-3">
        {# The shared single-entity field: keyboard navigation, the live region and Browse come with it. Standalone, so it submits nothing with the category form. #}
        <div class="w-64" data-tp-entity-field>
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity=tpEntityType standalone=true max=1 id="tp-entity-"|add:tpEntityType title='Preview against' placeholder="Search "|add:tpEntityType|add:"…" describedBy="tp-entity-hint-"|add:tpEntityType parameters='scopeParameters()' onChange='onEntityChange' %}
        </div>
        {# mt-9 lines the button up with the search input, below its label; the chip below the input would otherwise pull it down. #}
        <button type="button" @click="refreshAll()"
                class="mt-9 inline-flex items-center px-2 py-1 text-xs font-mono font-medium text-stone-700 bg-white border border-stone-300 rounded hover:bg-stone-100 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">
            Refresh all previews
        </button>
    </div>
    <p id="tp-entity-hint-{{ tpEntityType }}" class="text-xs text-stone-600">
        Each template below ends with a live preview against this {{ tpEntityType }}. List header and footer previews render against this category itself. Previews run in an isolated sandbox, so interactive editors and API-backed widgets do not work in them.
    </p>
    {# WS6 finding 29: when the category has no members, the preview borrows an entity from outside it. Say so: the sample does not carry this category's meta. #}
    <p x-show="usingUnscopedSample" class="text-xs text-amber-900 font-mono px-2 py-1 bg-amber-50">
        This category has no entities yet, so the previews use one from elsewhere. It does not carry this category's metadata.
    </p>
</div>

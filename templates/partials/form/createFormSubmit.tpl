{# Finding 129: every create and edit form ended in a lone Save. There was no  #}
{# Cancel, no link back to the entity and no breadcrumb, so abandoning an edit #}
{# meant the browser's Back button — and a screen-reader or keyboard user      #}
{# tabbing to the end of a long form found no way out at all. `cancelUrl` is   #}
{# the entity's own page when editing and its list when creating; each         #}
{# including template knows which and passes it.                               #}
<div class="pt-5">
    <div class="flex justify-end">
        {% if cancelUrl %}
        <a href="{{ cancelUrl }}"
           data-testid="form-cancel"
           class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            Cancel
        </a>
        {% endif %}
        {# `busyWhenUploading` is passed only by createResource.tpl, whose form   #}
        {# x-data is the bulk-upload component. Twelve templates include this     #}
        {# partial and most have some other x-data on the form, where a bare      #}
        {# `phase` would be an undefined identifier and Alpine would log an       #}
        {# expression error on every render. Gating in Pongo2 means the binding   #}
        {# only ever exists where the name does.                                  #}
        <button type="submit"
                {% if busyWhenUploading %}
                {# Disabled through `partial` too, not just `uploading`: Save  #}
                {# rebuilds the batch from the file input, so on a partial run   #}
                {# it would resend the files that already succeeded and turn     #}
                {# every one of them into a duplicate failure. Retry failed is   #}
                {# the control for that state; choosing files again starts over. #}
                :disabled="phase === 'uploading' || phase === 'partial'"
                :class="phase === 'uploading' || phase === 'partial' ? 'opacity-60 cursor-not-allowed' : ''"
                {% endif %}
                class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            Save
        </button>
    </div>
</div>

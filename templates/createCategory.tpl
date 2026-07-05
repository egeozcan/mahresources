{% extends "/layouts/base.tpl" %}

{% block body %}
{% if queryValues.error.0 %}
<div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
  <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
</div>
{% endif %}
<form class="space-y-8" method="post" action="/v1/category">
    {% if category.ID %}
    <input type="hidden" value="{{ category.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=queryValues.name.0|default:category.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=queryValues.Description.0|default:category.Description %}

    {% include "/partials/form/templateBundleTools.tpl" with carrier="category" %}

    <fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-2" x-data="{ showTemplateDocs: false }">
        <legend class="text-base font-semibold font-mono text-stone-800 px-2">Custom Templates</legend>

        <div class="text-sm text-stone-600">
            <p>HTML templates rendered in specific slots of detail and list views for groups in this category.</p>
            <button type="button"
                    @click="showTemplateDocs = !showTemplateDocs"
                    class="mt-1 text-sm text-amber-700 hover:text-amber-900 font-mono flex items-center gap-1 cursor-pointer"
                    :aria-expanded="showTemplateDocs.toString()"
                    aria-controls="cat-template-docs-panel">
                <svg :class="showTemplateDocs && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>
                Reference
            </button>
        </div>

        <div x-show="showTemplateDocs" x-collapse id="cat-template-docs-panel"
             class="text-sm text-stone-600 bg-white border border-stone-200 rounded-md p-4 space-y-3 font-sans">
            <div>
                <h3 class="font-semibold text-stone-700">Slot Locations</h3>
                <dl class="mt-1 space-y-1 text-xs">
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Header</dt>
                        <dd>Top of the group detail page, above the description</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom CSS</dt>
                        <dd>CSS injected as a <code class="bg-stone-100 px-1 rounded">&lt;style&gt;</code> block on the group detail page, its list pages, and MRQL result cards that use a Custom MRQL Result template</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Sidebar</dt>
                        <dd>Right sidebar on the group detail page</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Summary</dt>
                        <dd>Group cards in list views, below the title</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Avatar</dt>
                        <dd>Replaces the default initials avatar on group cards</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom List Header</dt>
                        <dd>Top of group list pages filtered to exactly this category, rendered against the category itself (<code class="bg-stone-100 px-1 rounded">[meta]</code> is empty, <code class="bg-stone-100 px-1 rounded">[mrql]</code> runs at global scope)</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom MRQL Result</dt>
                        <dd>Server-rendered template in <code class="bg-stone-100 px-1 rounded">[mrql]</code> results; Alpine directives not available</dd>
                    </div>
                </dl>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Shortcodes</h3>
                <p class="text-xs text-stone-400 mt-1">Type <code class="bg-stone-100 px-1 rounded">[</code> in any template editor for autocomplete; hover a shortcode for its full attribute list.</p>
                <div class="mt-1 space-y-3 text-xs">
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[meta path="dotted.path"]</code>
                        &mdash; render a metadata field value inline. Schema-aware when a Meta JSON Schema is defined.
                        <br><span class="text-stone-400 ml-4">
                            <b class="text-stone-500">path</b> (required) dot-notation into Meta JSON
                            &middot; <b class="text-stone-500">editable</b>="true" show edit button
                            &middot; <b class="text-stone-500">hide-empty</b>="true" hide when absent
                            &middot; <b class="text-stone-500">default</b>="text" fallback when the value is missing
                        </span>
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[meta path="status"]
[meta path="contact.email" editable="true"]
[meta path="address.city" hide-empty="true"]
[meta path="review.score" default="Unrated"]
&lt;div class="flex gap-4"&gt;
  &lt;strong&gt;Rating:&lt;/strong&gt; [meta path="review.score" editable="true"]
  &lt;strong&gt;Notes:&lt;/strong&gt; [meta path="review.notes" hide-empty="true"]
&lt;/div&gt;</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[property path="FieldName"]</code>
                        &mdash; render a struct field of the group. Output is HTML-escaped by default.
                        <br><span class="text-stone-400 ml-4">
                            <b class="text-stone-500">path</b> (required) field name or dot path (e.g. <span class="font-mono">Owner.Name</span>)
                            &middot; <b class="text-stone-500">raw</b>="true" skip HTML escaping
                            &middot; <b class="text-stone-500">format</b>="date|datetime|time|filesize"
                            &middot; <b class="text-stone-500">layout</b>="Jan 2, 2006" custom time layout
                            &middot; <b class="text-stone-500">default</b>="text" fallback when empty
                        </span>
                        <br><span class="text-stone-400 ml-4">
                            Fields: <span class="font-mono">ID, Name, Description, CreatedAt, UpdatedAt, URL, OwnerId, CategoryId, Meta</span>
                        </span>
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[property path="Name"]
[property path="CreatedAt" format="date"]
[property path="Description" raw="true"]
&lt;a :href="'/group?id=' + entity.ID"&gt;[property path="Name"]&lt;/a&gt;</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[mrql query='...']</code>
                        &mdash; inline MRQL query results.
                        <br><span class="text-stone-400 ml-4">
                            <b class="text-stone-500">query</b> or <b class="text-stone-500">saved</b> (one required) MRQL expression or saved query name
                        </span>
                        <br><span class="text-stone-400 ml-4">
                            <b class="text-stone-500">format</b>=table|list|compact|custom
                            &middot; <b class="text-stone-500">limit</b>=20
                            &middot; <b class="text-stone-500">buckets</b>=5 (for GROUP BY)
                        </span>
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[mrql query='type = resource AND tags = "photos"']
[mrql query='type = note AND created > -7d' format="table" limit="10"]
[mrql query='type = resource AND contentType ~ "image/*"' format="list" limit="5"]
[mrql query='type = group AND category = 3 GROUP BY meta.status' buckets="10"]
[mrql saved="recent-uploads" format="compact"]</code></pre>
                        <p class="mt-1 text-stone-400">
                            <b class="text-stone-500">scope</b>=entity|parent|root|global
                            &mdash; filter to a group subtree. Default: <code class="bg-stone-100 px-1 rounded">entity</code> (current group).
                            An explicit <code class="bg-stone-100 px-1 rounded">SCOPE</code> clause in the query takes precedence.
                            Nests up to 10 levels deep inside Custom MRQL Result templates.
                        </p>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[conditional path="..." eq="..."]…[/conditional]</code>
                        &mdash; render the inner content only when a condition holds. Test a <b class="text-stone-500">path</b> (Meta), <b class="text-stone-500">field</b> (struct field), or <b class="text-stone-500">mrql</b> result count.
                        <br><span class="text-stone-400 ml-4">
                            Operators: <span class="font-mono">eq neq gt lt gte lte in contains matches empty not-empty</span>
                            &middot; <b class="text-stone-500">combine</b>="all"|"any" (AND / OR)
                            &middot; add <b class="text-stone-500">[elseif …]</b> / <b class="text-stone-500">[else]</b> branches
                            &middot; numbered suffixes (<span class="font-mono">path2, eq2…</span>) add conditions
                        </span>
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[conditional path="rating" not-empty="true"]
  Rated: [meta path="rating"]
[/conditional]
[conditional path="tier" eq="gold"]Gold[elseif path="tier" eq="silver"]Silver[else]Basic[/conditional]</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[each path="arrayPath"]…[item]…[/each]</code>
                        &mdash; iterate an array in Meta, rendering the block once per element. Reference the element with <code class="bg-stone-100 px-1 rounded">[item]</code> (<code class="bg-stone-100 px-1 rounded">[item path="field"]</code> for objects, <code class="bg-stone-100 px-1 rounded">[item index="true"]</code> for its 1-based position). An optional <code class="bg-stone-100 px-1 rounded">[else]</code> branch renders when the array is empty.
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[each path="ingredients"]
  &lt;li&gt;[item path="name"] &mdash; [item path="qty" default="?"]&lt;/li&gt;
[else]
  &lt;li&gt;No items&lt;/li&gt;
[/each]</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[link to="self"]</code>
                        &mdash; resolve a detail-page URL. Inline it renders just the URL; as a block it wraps its content in an anchor.
                        <br><span class="text-stone-400 ml-4">
                            <b class="text-stone-500">to</b>="self|owner|root|category"
                        </span>
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>&lt;a href="[link]" class="underline"&gt;[property path="Name"]&lt;/a&gt;
[link to="owner"]Back to group[/link]</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[partial name="kebab-name"]</code>
                        &mdash; expand a reusable Template Partial by name, rendered with the current entity so its own shortcodes resolve here. Manage these under Template Partials.
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[partial name="status-badge"]</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[lazy]…[/lazy]</code>
                        &mdash; defer the inner content: it renders on the server only when the block scrolls into view, keeping expensive shortcodes (especially [mrql]) out of the initial page load. Ideal for per-card slots on long lists.
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[lazy][mrql query='type = "resource"' format="list"][/lazy]</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[details summary="…"]…[/details]</code>
                        &mdash; a keyboard- and screen-reader-accessible disclosure; the inner content renders on the server only the first time it is opened.
                        <br><span class="text-stone-400 ml-4">
                            <b class="text-stone-500">summary</b>="label" &nbsp; <b class="text-stone-500">open</b>="true|false"
                        </span>
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[details summary="Nutrition"][meta path="calories"] kcal[/details]</code></pre>
                    </div>
                    <div>
                        <code class="bg-stone-100 px-1 rounded">[plugin:name:shortcode attr="val"]</code>
                        &mdash; render a plugin-provided shortcode. See each plugin's docs page for available shortcodes.
                        <pre class="mt-1 bg-stone-50 border border-stone-200 rounded p-2 text-[11px] leading-relaxed overflow-x-auto"><code>[plugin:meta-editors:star-rating path="rating"]
[plugin:meta-editors:slider path="progress" min="0" max="100"]</code></pre>
                    </div>
                </div>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">HTML &amp; Styling</h3>
                <p class="text-xs">Raw HTML and <a href="https://tailwindcss.com/docs" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Tailwind CSS</a> utility classes are fully supported.</p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Alpine.js</h3>
                <p class="text-xs">
                    An <code class="bg-stone-100 px-1 rounded">entity</code> variable with the full group object is available at render time, e.g.
                    <code class="bg-stone-100 px-1 rounded">x-text="entity.Name"</code> or
                    <code class="bg-stone-100 px-1 rounded">x-show="entity.Meta?.status"</code>.
                </p>
            </div>
        </div>

        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom Header" name="CustomHeader" value=category.CustomHeader mode="html" description="Rendered at the top of the group detail page, above the description." shortcodes=true generate=true %}
        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom CSS" name="CustomCSS" value=category.CustomCSS mode="css" description="Injected as a <style> block on the group detail page, its list pages, and MRQL result cards that use a Custom MRQL Result template." shortcodes=true generate=true %}
        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=category.CustomSidebar mode="html" description="Rendered in the group detail page sidebar." shortcodes=true generate=true %}
        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom Summary" name="CustomSummary" value=category.CustomSummary mode="html" description="Rendered on group cards in list views, below the title." shortcodes=true generate=true %}
        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=category.CustomAvatar mode="html" description="Replaces the default initials avatar on group cards." shortcodes=true generate=true %}
        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom List Header" name="CustomListHeader" value=category.CustomListHeader mode="html" description="Rendered at the top of group list pages filtered to exactly this category. Processed against the category itself: [property path=&quot;Name&quot;] is the category name, [meta] is empty, and [mrql] runs at global scope." shortcodes=true generate=true %}
        {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Custom MRQL Result" name="CustomMRQLResult" value=category.CustomMRQLResult mode="html" description="Server-rendered in [mrql] results. Shortcodes work; Alpine directives do not." shortcodes=true generate=true %}
    </fieldset>

    {% include "/partials/form/templatePreviewPane.tpl" with entityType="group" previewPath="/v1/category/previewTemplate" generatePath="/v1/category/generateTemplate" categoryId=category.ID %}
    <div class="flex gap-2 items-start">
        <div class="flex-1">
            {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=category.MetaSchema mode="json" id="metaSchemaTextarea" generate=true %}
        </div>
        {% include "/partials/form/schemaEditorModal.tpl" with textareaId="metaSchemaTextarea" %}
    </div>

    {% include "/partials/sectionConfigForm.tpl" with sectionConfigValue=category.SectionConfig sectionConfigType="group" %}

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}
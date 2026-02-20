{% extends "/layouts/base.tpl" %}

{% block body %}
{% if not readOnlyEnforced %}
<div class="bg-yellow-100 border-l-4 border-yellow-500 text-yellow-700 p-4 mb-4" role="alert">
    <p class="font-bold">Warning</p>
    <p>Queries run without database-level read-only enforcement. Configure a separate <code>DB_READONLY_DSN</code> with read-only access for safety.</p>
</div>
{% endif %}
<form class="space-y-8" method="post" action="/v1/query">
    {% if query.ID %}
    <input type="hidden" value="{{ query.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=query.Name required=true %}
    {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Query" name="Text" value=query.Text mode="sql" dbType=dbType %}

    <div x-data="{ open: false }" class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start">
        <div></div>
        <div class="sm:col-span-2">
            <button type="button" @click="open = !open"
                    class="text-sm text-indigo-600 hover:text-indigo-800 flex items-center gap-1">
                <svg :class="open && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>
                SQL query reference
            </button>
            <div x-show="open" x-collapse class="mt-2 text-sm text-gray-600 bg-gray-50 border border-gray-200 rounded-md p-4 space-y-3">
                <div>
                    <h4 class="font-semibold text-gray-700">Parameters</h4>
                    <p>Use <code class="bg-gray-200 px-1 rounded">:paramName</code> for named parameters. When the query runs, each parameter gets an input field. Example:</p>
                    <pre class="bg-gray-100 p-2 rounded mt-1 overflow-x-auto">SELECT * FROM resources WHERE name LIKE :search</pre>
                    {% if dbType == "POSTGRES" %}
                    <p class="mt-1">PostgreSQL type casts (<code class="bg-gray-200 px-1 rounded">::</code>) work normally &mdash; they are handled automatically.</p>
                    {% endif %}
                </div>
                <div>
                    <h4 class="font-semibold text-gray-700">Tables</h4>
                    <p>Autocompletion (Ctrl+Space) suggests table and column names. Main tables:</p>
                    <div class="mt-1 grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-1">
                        <div>
                            <code class="text-indigo-700 font-semibold">resources</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, original_name, hash, location, description, content_type, content_category, file_size, width, height, owner_id, resource_category_id, meta, created_at, updated_at</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">notes</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description, meta, owner_id, note_type_id, start_date, end_date, created_at, updated_at</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">groups</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description, url, meta, owner_id, category_id, created_at, updated_at</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">tags</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description, created_at, updated_at</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">categories</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">note_types</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">resource_categories</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">queries</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, text, template, created_at, updated_at</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">group_relations</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description, from_group_id, to_group_id, relation_type_id</span>
                        </div>
                        <div>
                            <code class="text-indigo-700 font-semibold">group_relation_types</code>
                            <span class="text-xs text-gray-500">&mdash; id, name, description, from_category_id, to_category_id</span>
                        </div>
                    </div>
                </div>
                <div>
                    <h4 class="font-semibold text-gray-700">Join tables (many-to-many)</h4>
                    <p class="text-xs">
                        <code class="bg-gray-200 px-1 rounded">resource_tags</code>,
                        <code class="bg-gray-200 px-1 rounded">resource_notes</code>,
                        <code class="bg-gray-200 px-1 rounded">groups_related_resources</code>,
                        <code class="bg-gray-200 px-1 rounded">groups_related_notes</code>,
                        <code class="bg-gray-200 px-1 rounded">group_related_groups</code>,
                        <code class="bg-gray-200 px-1 rounded">note_tags</code>,
                        <code class="bg-gray-200 px-1 rounded">group_tags</code>
                    </p>
                    <p class="text-xs mt-1">Each join table has two foreign key columns, e.g. <code class="bg-gray-200 px-1 rounded">resource_tags</code> has <code class="bg-gray-200 px-1 rounded">resource_id</code> and <code class="bg-gray-200 px-1 rounded">tag_id</code>.</p>
                </div>
                <div>
                    <h4 class="font-semibold text-gray-700">Example queries</h4>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto">-- All resources with a specific tag
SELECT r.* FROM resources r
JOIN resource_tags rt ON rt.resource_id = r.id
JOIN tags t ON t.id = rt.tag_id
WHERE t.name = :tagName</pre>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto mt-1">-- Notes created in a date range
SELECT * FROM notes
WHERE created_at >= :startDate
  AND created_at <= :endDate
ORDER BY created_at DESC</pre>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto mt-1">-- Groups and their resource counts
SELECT g.id, g.name, COUNT(gr.resource_id) AS resource_count
FROM groups g
LEFT JOIN groups_related_resources gr ON gr.group_id = g.id
GROUP BY g.id, g.name
ORDER BY resource_count DESC</pre>
                </div>
                <p class="text-xs text-gray-500">Queries are read-only &mdash; INSERT, UPDATE, and DELETE statements will be rejected.</p>
            </div>
        </div>
    </div>

    {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Template" name="Template" value=query.Template mode="html" dbType=dbType %}

    <div x-data="{ open: false }" class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start">
        <div></div>
        <div class="sm:col-span-2">
            <button type="button" @click="open = !open"
                    class="text-sm text-indigo-600 hover:text-indigo-800 flex items-center gap-1">
                <svg :class="open && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>
                Template reference
            </button>
            <div x-show="open" x-collapse class="mt-2 text-sm text-gray-600 bg-gray-50 border border-gray-200 rounded-md p-4 space-y-3">
                <div>
                    <h4 class="font-semibold text-gray-700">Overview</h4>
                    <p>The template is raw HTML rendered below the results table after the query runs. Use it to add custom visualizations, summaries, or actions based on the query results.</p>
                </div>
                <div>
                    <h4 class="font-semibold text-gray-700">Accessing results</h4>
                    <p>Query results are available in two ways:</p>
                    <ul class="list-disc list-inside mt-1 space-y-1">
                        <li><code class="bg-gray-200 px-1 rounded">window.results</code> &mdash; the full JSON array of result rows (each row is an object with column names as keys)</li>
                        <li><code class="bg-gray-200 px-1 rounded">results</code> &mdash; same array, also accessible as an Alpine.js reactive variable in the page scope</li>
                    </ul>
                </div>
                <div>
                    <h4 class="font-semibold text-gray-700">Visibility</h4>
                    <p>The template container is only shown when results are non-empty. It is wrapped in an Alpine.js <code class="bg-gray-200 px-1 rounded">x-if="!error && results && results.length > 0"</code> block.</p>
                </div>
                <div>
                    <h4 class="font-semibold text-gray-700">Examples</h4>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto">&lt;!-- Show total count --&gt;
&lt;p&gt;Total results: &lt;span x-text="results.length"&gt;&lt;/span&gt;&lt;/p&gt;</pre>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto mt-1">&lt;!-- Sum a numeric column --&gt;
&lt;p&gt;Total size:
  &lt;span x-text="results.reduce((s, r) =&gt; s + (r.file_size || 0), 0).toLocaleString()"&gt;&lt;/span&gt;
  bytes
&lt;/p&gt;</pre>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto mt-1">&lt;!-- Link to each result --&gt;
&lt;template x-for="row in results"&gt;
  &lt;a :href="'/resource?id=' + row.id" x-text="row.name" class="block text-blue-600"&gt;&lt;/a&gt;
&lt;/template&gt;</pre>
                    <pre class="bg-gray-100 p-2 rounded overflow-x-auto mt-1">&lt;!-- Custom script using window.results --&gt;
&lt;canvas id="myChart" width="400" height="200"&gt;&lt;/canvas&gt;
&lt;script&gt;
  // Use window.results to build charts, export data, etc.
  const data = window.results;
  console.log('Got', data.length, 'rows');
&lt;/script&gt;</pre>
                </div>
                <p class="text-xs text-gray-500">The template is rendered as-is (no server-side escaping). You can use any HTML, CSS, and JavaScript.</p>
            </div>
        </div>
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}

</form>
{% endblock %}
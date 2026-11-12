{% if not readOnlyEnforced %}
<div class="bg-yellow-100 border-l-4 border-yellow-500 text-yellow-700 p-4 mb-4" role="alert">
    <p class="font-bold">Warning</p>
    <p>Saved queries are not limited to reading content tables: {{ readOnlyProblem }}.</p>
    <p>On PostgreSQL with authentication on, saved queries are disabled until this is fixed; <code>mahresources -print-readonly-role-sql=&lt;role&gt;</code> prints the SQL that creates a correctly restricted role for <code>DB_READONLY_DSN</code>.</p>
</div>
{% endif %}

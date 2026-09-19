{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-6" data-testid="plugin-command-history">
    {% if notice == "cancelled" %}
    <p role="status" tabindex="-1" autofocus data-testid="command-cancel-notice"
       class="border-l-4 border-t-4 border-green-700 bg-green-50 p-3 text-sm text-green-900">
        Cancellation requested. The durable run status is shown below.
    </p>
    {% endif %}

    <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-stone-600">{{ commandRunsCount }} durable plugin command run{% if commandRunsCount != 1 %}s{% endif %}, newest first.</p>
        <a href="/plugins/manage" class="text-sm text-amber-700 underline decoration-amber-300 hover:decoration-amber-700 rounded focus:outline-none focus:ring-2 focus:ring-amber-600">Plugin management</a>
    </div>

    {% if commandRun %}
    <section aria-labelledby="command-run-detail-heading" class="border-l-4 border-t-4 border-amber-700 bg-white p-4 shadow-sm space-y-4" data-testid="command-run-detail">
        <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
                <h2 id="command-run-detail-heading" class="font-mono text-lg font-semibold text-stone-900">Run {{ commandRun.ID }}</h2>
                <p class="text-sm text-stone-600">{{ commandRun.PluginName }} / {{ commandRun.CommandName }}</p>
            </div>
            <span class="rounded-full bg-stone-100 px-2 py-1 text-sm font-medium text-stone-800" data-testid="command-run-status">{{ commandRun.Status }}</span>
        </div>
        <dl class="grid gap-2 text-sm sm:grid-cols-2">
            <div><dt class="font-semibold">Submitted</dt><dd>{{ commandRun.CreatedAt }}</dd></div>
            <div><dt class="font-semibold">Started</dt><dd>{% if commandRun.StartedAt %}{{ commandRun.StartedAt }}{% else %}Not started{% endif %}</dd></div>
            <div><dt class="font-semibold">Finished</dt><dd>{% if commandRun.FinishedAt %}{{ commandRun.FinishedAt }}{% else %}Not finished{% endif %}</dd></div>
            <div><dt class="font-semibold">Exit code</dt><dd>{% if exitCodeAvailable %}{{ commandRun.ExitCode }}{% else %}&mdash;{% endif %}</dd></div>
            <div><dt class="font-semibold">Actor provenance</dt><dd>{% if commandRun.ActorlessAtSubmission %}System (actorless at submission){% elif commandRun.CreatedByUserID %}User ID {{ commandRun.CreatedByUserID }}{% else %}Submitting user was deleted{% endif %}</dd></div>
            <div><dt class="font-semibold">Cancellation requested</dt><dd>{% if commandRun.CancelRequested %}Yes{% else %}No{% endif %}</dd></div>
            <div class="sm:col-span-2"><dt class="font-semibold">Terminal reason</dt><dd>{% if commandRun.Error %}{{ commandRun.Error }}{% else %}&mdash;{% endif %}</dd></div>
        </dl>
        <div>
            <h3 class="font-mono font-semibold text-stone-900">Redacted parameters</h3>
            <pre class="mt-1 overflow-x-auto whitespace-pre-wrap rounded bg-stone-100 p-3 text-xs" data-testid="command-run-params">{{ commandRun.ParamsJSON }}</pre>
        </div>
        <div>
            <h3 class="font-mono font-semibold text-stone-900">Redacted command</h3>
            {% if outputAvailable %}
            <pre class="mt-1 overflow-x-auto whitespace-pre-wrap rounded bg-stone-100 p-3 text-xs" data-testid="command-run-argv">{{ commandRun.Output.ArgvJSON }}</pre>
            {% else %}
            <p class="mt-1 text-sm text-stone-600">The retained command/output row has expired; durable run and import history remains.</p>
            {% endif %}
        </div>
        <div>
            <h3 class="font-mono font-semibold text-stone-900">Imports</h3>
            <table class="mt-1 w-full text-left text-sm">
                <caption class="sr-only">Files imported from this command run</caption>
                <thead><tr><th scope="col" class="py-1">File</th><th scope="col">Status</th><th scope="col">Resource</th><th scope="col">Outcome</th></tr></thead>
                <tbody>
                {% for item in commandRun.Imports %}
                <tr class="border-t border-stone-200"><td class="py-1">{{ item.FileName }}</td><td>{{ item.Status }}</td><td>{% if item.ResourceID %}<a class="text-amber-700 underline" href="/resource?id={{ item.ResourceID }}">{{ item.ResourceID }}</a>{% else %}&mdash;{% endif %}</td><td>{% if item.Error %}{{ item.Error }}{% else %}&mdash;{% endif %}</td></tr>
                {% empty %}<tr><td colspan="4" class="py-2 text-stone-600">No import attempts.</td></tr>{% endfor %}
                </tbody>
            </table>
        </div>
        <div>
            <h3 class="font-mono font-semibold text-stone-900">Program output</h3>
            <p class="mt-1 border-l-4 border-red-700 bg-red-50 p-3 text-sm text-red-900" role="note">
                Program output can echo secrets even when command parameters are redacted. Treat this text as sensitive.
            </p>
            {% if commandRun.OutputUnverified %}
            <p class="mt-2 border-l-4 border-yellow-700 bg-yellow-50 p-3 text-sm text-yellow-900" role="note">This output tail could not be verified during recovery.</p>
            {% endif %}
            {% if outputAvailable %}
            <pre class="mt-2 max-h-96 overflow-auto whitespace-pre-wrap rounded bg-stone-950 p-3 text-xs text-stone-100" data-testid="command-run-output">{{ commandRun.Output.OutputTail }}</pre>
            {% else %}
            <p class="mt-2 text-sm text-stone-600" data-testid="command-run-output-pruned">Output is no longer available.</p>
            {% endif %}
        </div>
        {% if commandRun.Status == "queued" %}
        <form method="post" action="/v1/plugin/command-run/cancel">
            <input type="hidden" name="csrf_token" value="{{ csrfToken }}">
            <input type="hidden" name="id" value="{{ commandRun.ID }}">
            <button type="submit" class="rounded border-l-4 border-t-4 border-red-700 bg-red-50 px-3 py-2 text-sm font-semibold text-red-900 focus:outline-none focus:ring-2 focus:ring-amber-600">Cancel queued run</button>
        </form>
        {% endif %}
    </section>
    {% endif %}

    <section aria-labelledby="command-runs-heading" class="overflow-x-auto">
        <h2 id="command-runs-heading" class="sr-only">Command runs</h2>
        <table class="w-full min-w-[48rem] text-left text-sm" data-testid="command-runs-table">
            <caption class="sr-only">Durable plugin command runs, newest first</caption>
            <thead class="border-b-2 border-stone-300 font-mono"><tr><th scope="col" class="p-2">Run</th><th scope="col" class="p-2">Plugin</th><th scope="col" class="p-2">Command</th><th scope="col" class="p-2">Status</th><th scope="col" class="p-2">Submitted</th><th scope="col" class="p-2">Actions</th></tr></thead>
            <tbody>
            {% for run in commandRuns %}
            <tr class="border-b border-stone-200">
                <td class="p-2 font-mono"><a class="text-amber-700 underline" href="/admin/plugin-command-runs?id={{ run.ID }}">{{ run.ID }}</a></td>
                <td class="p-2">{{ run.PluginName }}</td><td class="p-2">{{ run.CommandName }}</td><td class="p-2">{{ run.Status }}</td><td class="p-2">{{ run.CreatedAt }}</td>
                <td class="p-2">
                    {% if run.Status == "queued" %}
                    <form method="post" action="/v1/plugin/command-run/cancel" class="inline">
                        <input type="hidden" name="csrf_token" value="{{ csrfToken }}"><input type="hidden" name="id" value="{{ run.ID }}">
                        <button type="submit" class="text-red-700 underline rounded focus:outline-none focus:ring-2 focus:ring-amber-600">Cancel queued run</button>
                    </form>
                    {% else %}<span class="text-stone-500">&mdash;</span>{% endif %}
                </td>
            </tr>
            {% empty %}<tr><td colspan="6" class="p-4 text-center text-stone-600">No plugin command runs.</td></tr>{% endfor %}
            </tbody>
        </table>
    </section>
</div>
{% endblock %}

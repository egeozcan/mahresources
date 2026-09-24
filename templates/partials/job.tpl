{# One Job, as a card in the shared entity-list shape (.card, card-header,        #}
{# card-meta, card-badges) so /jobs reads like the other lists. `job` is a JobRow #}
{# from job_template_context.go. `key` is the morph key: a live refresh patches  #}
{# each card in place, keeping its selection and its open details.              #}
<article class="card job-card card--selectable" key="{{ job.ID }}" data-job-id="{{ job.ID }}"
         x-data="selectableItem({ itemId: '{{ job.ID|escapejs }}' })">
    <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events"
           aria-label="Select {{ job.Title }}"
           class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">

    <div data-entity='{{ job.Entity }}'>
        <header class="card-header card-header--compact">
            <div class="card-title-section">
                <h2 class="card-title card-title--simple"><a href="{{ job.DetailURL }}">{{ job.Title }}</a></h2>
                <div class="card-meta">
                    <span class="card-meta-item">{{ job.Kind }}</span>
                    {% if job.Phase %}<span class="card-meta-item">{{ job.Phase }}</span>{% endif %}
                    <span class="card-meta-item">
                        <span class="card-meta-label">Accepted:</span>
                        <time data-local-time datetime="{{ job.Accepted.ISO }}">{{ job.Accepted.Minute }}</time>
                    </span>
                </div>
            </div>
        </header>

        <div class="card-badges">
            {% if job.State == "succeeded" %}
            <span class="card-badge card-badge--success" data-testid="job-state">{{ job.StateLabel }}</span>
            {% elif job.State == "failed" || job.State == "blocked" || job.State == "interrupted" %}
            <span class="card-badge card-badge--danger" data-testid="job-state">{{ job.StateLabel }}</span>
            {% elif job.State == "cancelled" %}
            <span class="card-badge card-badge--muted" data-testid="job-state">{{ job.StateLabel }}</span>
            {% else %}
            <span class="card-badge card-badge--live" data-testid="job-state">{{ job.StateLabel }}</span>
            {% endif %}
            {% if job.Pinned %}<span class="card-badge">Pinned by you</span>{% endif %}
            {% if job.Result.URL %}
            <a href="{{ job.Result.URL }}" aria-label="{{ job.Result.AccessibleLabel }}" class="card-badge card-badge--category" data-testid="job-result-link">{{ job.Result.Label }}</a>
            {% endif %}
        </div>

        {% if job.SummaryText %}<p class="mt-2 break-words text-sm text-stone-700">{{ job.SummaryText }}</p>{% endif %}

        {% if job.Progress %}
        <div class="mt-3 max-w-xl">
            <div class="mb-1 flex justify-between gap-2 text-xs text-stone-600">
                <span>{{ job.Progress.Text }}</span>
                <span>{% if job.Progress.Known %}{{ job.Progress.Percent }}%{% elif job.Progress.Indeterminate %}In progress{% endif %}</span>
            </div>
            <div class="h-2 rounded bg-stone-200" role="progressbar" aria-valuemin="0" aria-valuemax="100"
                 {% if job.Progress.Known %}aria-valuenow="{{ job.Progress.Percent }}"{% endif %}
                 aria-valuetext="{{ job.Progress.AccessibleText }}"
                 aria-label="{{ job.Title }} progress: {{ job.Progress.AccessibleText }}">
                {% if job.Progress.Known %}
                <div class="h-2 rounded bg-amber-800" style="width:{{ job.Progress.Percent }}%"></div>
                {% else %}
                <div class="h-2 rounded bg-amber-800{% if job.Progress.Indeterminate %} w-full animate-pulse{% endif %}"></div>
                {% endif %}
            </div>
        </div>
        {% endif %}

        {% if job.FailureMessage %}<p class="mt-2 break-words text-sm text-red-800">{{ job.FailureMessage }}</p>{% endif %}

        <details class="mt-2 text-xs text-stone-600" data-job-details>
            <summary class="cursor-pointer font-medium text-amber-900">Details</summary>
            <dl class="mt-2 grid gap-1 border-l-2 border-stone-300 pl-3 sm:grid-cols-2">
                <div><dt class="inline">Accepted</dt> <dd class="inline"><time data-local-time="seconds" datetime="{{ job.Accepted.ISO }}">{{ job.Accepted.Display }}</time></dd></div>
                <div><dt class="inline">Started</dt> <dd class="inline">{% if job.Started.ISO %}<time data-local-time="seconds" datetime="{{ job.Started.ISO }}">{{ job.Started.Display }}</time>{% else %}Not started{% endif %}</dd></div>
                {% if job.Finished.ISO %}<div><dt class="inline">Finished</dt> <dd class="inline"><time data-local-time="seconds" datetime="{{ job.Finished.ISO }}">{{ job.Finished.Display }}</time></dd></div>{% endif %}
                <div><dt class="inline">Version</dt> <dd class="inline font-mono">{{ job.Version }}</dd></div>
            </dl>
        </details>
    </div>
</article>

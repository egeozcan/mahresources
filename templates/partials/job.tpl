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
                    <span class="card-meta-item" data-testid="job-kind">{{ job.KindLabel }}</span>
                    {% if job.Owner %}<span class="card-meta-item" data-testid="job-owner"><span class="card-meta-label">Owner:</span> {{ job.Owner }}</span>{% endif %}
                    {% if job.Phase %}<span class="card-meta-item">{{ job.Phase }}</span>{% endif %}
                    <span class="card-meta-item">
                        <span class="card-meta-label">Accepted:</span>
                        <time data-local-time datetime="{{ job.Accepted.ISO }}">{{ job.Accepted.Minute }}</time>
                    </span>
                </div>
            </div>
        </header>

        <div class="card-badges">
            {# The colour is the state's tone from the shared state table (server/jobview/job_states.json); the label says it in words. #}
            <span class="card-badge {{ job.BadgeClass }}" data-testid="job-state">{{ job.StateLabel }}</span>
            {% if job.Pinned %}<span class="card-badge">Pinned by you</span>{% endif %}
            {% if job.Result.URL %}
            <a href="{{ job.Result.URL }}" aria-label="{{ job.Result.AccessibleLabel }}" class="card-badge card-badge--category" data-testid="job-result-link">{{ job.Result.Label }}</a>
            {% endif %}
        </div>

        {% if job.SummaryText %}<p class="mt-2 break-words text-sm text-stone-700">{{ job.SummaryText }}</p>{% endif %}

        {% if job.ScheduledFor.ISO %}
        <p class="mt-2 text-sm text-stone-700" data-testid="job-scheduled-for">Starts <time data-local-time datetime="{{ job.ScheduledFor.ISO }}">{{ job.ScheduledFor.Minute }}</time></p>
        {% endif %}

        {% if job.Progress %}
        {# The data-job-progress hooks are where a live progress frame lands (jobList.js #}
        {# applyCardProgress); the stats line is always rendered so it has a place to.  #}
        <div class="mt-3 max-w-xl" data-job-progress{% if job.ProgressUpdatedAt %} data-progress-updated-at="{{ job.ProgressUpdatedAt }}"{% endif %}>
            <div class="mb-1 flex justify-between gap-2 text-xs text-stone-600">
                <span data-job-progress-text>{{ job.Progress.Text }}</span>
                <span data-job-progress-value>{% if job.Progress.Known %}{{ job.Progress.Percent }}%{% elif job.Progress.Indeterminate %}In progress{% endif %}</span>
            </div>
            {# Stopped work with no known total draws an empty track: a full fill would read as done. #}
            {# Forced colours drop backgrounds, so the track keeps a border and the fill a system colour. #}
            <div class="h-2 overflow-hidden rounded bg-stone-200 forced-colors:border forced-colors:border-[CanvasText]" role="progressbar" aria-valuemin="0" aria-valuemax="100" data-job-progress-bar
                 {% if job.Progress.Known %}aria-valuenow="{{ job.Progress.Percent }}"{% endif %}
                 aria-valuetext="{{ job.Progress.AccessibleText }}"
                 aria-label="{{ job.Title }} progress: {{ job.Progress.AccessibleText }}">
                <div class="h-full rounded bg-amber-800 forced-color-adjust-none forced-colors:bg-[Highlight]{% if not job.Progress.Known and job.Progress.Indeterminate %} w-full motion-safe:animate-pulse{% endif %}" data-job-progress-fill{% if job.Progress.Known %} style="width:{{ job.Progress.Percent }}%"{% elif not job.Progress.Indeterminate %} style="width:0"{% endif %}></div>
            </div>
            <p class="mt-1 text-xs tabular-nums text-stone-600" data-testid="job-stats" data-job-stats{% if not job.Progress.Stats %} hidden{% endif %}>{{ job.Progress.Stats }}</p>
        </div>
        {% endif %}

        {% if job.FailureMessage %}<p class="mt-2 break-words text-sm text-red-800">{{ job.FailureMessage }}</p>{% endif %}

        <details class="mt-2 text-xs text-stone-600" data-job-details>
            <summary class="cursor-pointer font-medium text-amber-900">Details</summary>
            <dl class="mt-2 grid gap-1 border-l-2 border-stone-300 pl-3 sm:grid-cols-2">
                <div><dt class="inline">Accepted</dt> <dd class="inline"><time data-local-time="seconds" datetime="{{ job.Accepted.ISO }}">{{ job.Accepted.Display }}</time></dd></div>
                {% if job.ScheduledFor.ISO %}<div><dt class="inline">Scheduled for</dt> <dd class="inline"><time data-local-time="seconds" datetime="{{ job.ScheduledFor.ISO }}">{{ job.ScheduledFor.Display }}</time></dd></div>{% endif %}
                <div><dt class="inline">Started</dt> <dd class="inline">{% if job.Started.ISO %}<time data-local-time="seconds" datetime="{{ job.Started.ISO }}">{{ job.Started.Display }}</time>{% else %}Not started{% endif %}</dd></div>
                {% if job.Finished.ISO %}<div><dt class="inline">Finished</dt> <dd class="inline"><time data-local-time="seconds" datetime="{{ job.Finished.ISO }}">{{ job.Finished.Display }}</time></dd></div>{% endif %}
                <div><dt class="inline">Version</dt> <dd class="inline font-mono">{{ job.Version }}</dd></div>
                {% for field in job.SummaryFields %}<div data-job-summary-field><dt class="inline">{{ field.Label }}</dt> <dd class="inline break-words">{{ field.Value }}</dd></div>{% endfor %}
            </dl>
        </details>
    </div>
</article>

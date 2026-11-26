{% if entity %}
<article class="card reduction-card" data-testid="reduction-card" data-reduction-id="{{ entity.ID }}">
    <header class="card-header card-header--compact">
        <div class="card-title-section w-full">
            <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                <div class="min-w-[14rem] flex-1">
                    <h2 class="card-title">
                        <a href="/reduction?id={{ entity.ID }}" title="{{ entity.Name }}">{{ entity.Name }}</a>
                    </h2>
                    <p class="mt-1 text-sm text-stone-700" data-testid="reduction-counts">
                        {% if entity.Upgrading %}
                            Being upgraded to the new storage. Its Clusters appear here when that finishes.
                        {% elif entity.StatusEffective == "draft" %}
                            Not computed yet. Nothing is removed until you review and apply.
                        {% elif entity.StatusEffective == "computing" %}
                            Finding duplicates…
                        {% elif entity.StatusEffective == "failed" %}
                            Finding duplicates did not finish.
                        {% elif entity.Progress.Clusters == 0 %}
                            No duplicates found.
                        {% else %}
                            {{ entity.Progress.Clusters }} Cluster{% if entity.Progress.Clusters != 1 %}s{% endif %}{% if entity.Progress.Approved %} &middot; {{ entity.Progress.Approved }} approved{% endif %}{% if entity.Progress.Applied %} &middot; {{ entity.Progress.Applied }} applied{% endif %}{% if entity.Progress.Skipped %} &middot; {{ entity.Progress.Skipped }} skipped{% endif %}{% if entity.Progress.Stale %} &middot; {{ entity.Progress.Stale }} stale{% endif %}
                        {% endif %}
                    </p>
                    {% if entity.Progress.Clusters > 0 %}
                    <div class="mt-2 h-1.5 w-full max-w-md rounded-full bg-stone-200 overflow-hidden"
                         role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="{{ entity.DonePercent }}"
                         aria-label="{{ entity.Name }}: {{ entity.DonePercent }} percent of Clusters applied or skipped"
                         data-testid="reduction-progress">
                        <div class="h-full bg-amber-700" style="width: {{ entity.DonePercent }}%"></div>
                    </div>
                    {% endif %}
                </div>

                <div class="reduction-card-actions flex flex-wrap items-center gap-3 shrink-0">
                    <span class="inline-flex items-center rounded-full border border-stone-300 bg-stone-50 px-2 py-0.5 text-xs font-mono text-stone-700" data-testid="reduction-status">{{ entity.StatusLabel }}</span>
                    {% if entity.ActionKind == "find" || entity.ActionKind == "retry" %}
                    <div x-data="reductionCardAction({ id: {{ entity.ID }}, version: {{ entity.Version }} })">
                        <button type="button" @click="start()" :disabled="busy" data-testid="reduction-action" data-action-kind="{{ entity.ActionKind }}"
                                class="inline-flex items-center justify-center min-h-[2.75rem] py-1.5 px-3 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
                            {{ entity.ActionLabel }}<span class="sr-only"> in {{ entity.Name }}</span>
                        </button>
                    </div>
                    {% elif entity.ActionKind == "upgrading" %}
                    <span data-testid="reduction-action" data-action-kind="upgrading"
                          class="inline-flex items-center justify-center min-h-[2.75rem] py-1.5 px-3 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-stone-50">
                        {{ entity.ActionLabel }}<span class="sr-only"> : {{ entity.Name }}</span>
                    </span>
                    {% else %}
                    <a href="/reduction?id={{ entity.ID }}" data-testid="reduction-action" data-action-kind="{{ entity.ActionKind }}"
                       class="inline-flex items-center justify-center min-h-[2.75rem] py-1.5 px-3 border shadow-sm text-sm font-mono font-medium rounded-md focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 {% if entity.ActionKind == "continue" %}border-transparent text-white bg-amber-700 hover:bg-amber-800{% else %}border-stone-300 text-stone-700 bg-white hover:bg-stone-50{% endif %}">
                        {{ entity.ActionLabel }}<span class="sr-only"> in {{ entity.Name }}</span>
                    </a>
                    {% endif %}
                </div>
            </div>

            <div class="card-meta mt-2">
                <span class="card-meta-item">
                    <span class="card-meta-label">Created:</span>
                    {{ entity.CreatedAt|date:"2006-01-02 15:04" }}
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">Updated:</span>
                    {{ entity.UpdatedAt|date:"2006-01-02 15:04" }}
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">Last computed:</span>
                    {{ entity.ComputedAtLabel }}
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">Matching:</span>
                    {{ entity.MatchingLabel }}
                </span>
            </div>
        </div>
    </header>
</article>
{% endif %}

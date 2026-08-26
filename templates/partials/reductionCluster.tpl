{# One Cluster of a Resource Reduction.                                          #}
{#                                                                               #}
{# The justification leads and the thumbnails follow, because the whole point of  #}
{# the design is that reviewing is reading a justification rather than squinting  #}
{# at thumbnails.                                                                #}
{#                                                                               #}
{# data-lightbox-scope makes the members their own gallery: flicking through five #}
{# candidates must not drag in the rest of the page.                              #}
<article class="card reduction-cluster"
         data-testid="reduction-cluster"
         data-cluster-id="{{ cluster.ID }}"
         data-cluster-tier="{{ cluster.Tier }}"
         data-lightbox-scope
         aria-labelledby="cluster-{{ cluster.ID }}-heading">

    <header class="card-header card-header--compact">
        <div class="card-title-section">
            <h3 class="card-title" id="cluster-{{ cluster.ID }}-heading">
                {% if cluster.Tier == "identical" %}
                    Identical Resources
                {% else %}
                    Near-Identical Resources
                {% endif %}
                <span class="text-stone-500 font-normal">&mdash; {{ cluster.Members|length }} Resources</span>
            </h3>
            <div class="card-meta">
                <span class="card-meta-item">
                    <span class="card-meta-label">Winner:</span>
                    {% if cluster.Winner %}
                        <a href="/resource?id={{ cluster.Winner.ID }}" class="card-meta-link">{{ cluster.Winner.Name }}</a>
                    {% else %}
                        <span class="text-stone-500">withheld</span>
                    {% endif %}
                </span>
                <span class="card-meta-item" data-testid="cluster-decided-by">
                    {% if cluster.Undecided %}
                        <span class="text-amber-800">No criterion could decide &mdash; the Winner is the lowest id</span>
                    {% else %}
                        <span class="card-meta-label">Chosen by:</span>
                        {{ cluster.DecidedByLabel }}{% if cluster.Margin %}, by {{ cluster.Margin }}{% endif %}
                    {% endif %}
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">State:</span>
                    <span data-testid="cluster-state">{{ cluster.StateLabel }}</span>
                </span>
            </div>
        </div>

        {% if cluster.State != "applied" %}
        <div class="flex items-center gap-2 flex-wrap">
            <label class="flex items-center gap-2 text-sm text-stone-700">
                {# Every reason to disable this lives inside the binding. Alpine removes #}
                {# the attribute whenever the binding evaluates false, so a *separate*    #}
                {# static `{% if cluster.Withheld %}disabled{% endif %}` would be         #}
                {# stripped at init and a Cluster the reviewer cannot fully see would     #}
                {# become checkable. The real guard is apply's own re-check; this only    #}
                {# stops the UI lying. The bare `disabled` is the pre-init state: this    #}
                {# control does nothing until Alpine has wired it, and if Alpine never    #}
                {# arrives it does nothing at all.                                        #}
                <input type="checkbox"
                       data-testid="cluster-checkbox"
                       {% if cluster.Checked %}checked{% endif %}
                       disabled
                       :disabled="busy || {% if cluster.Withheld %}true{% else %}false{% endif %} || !isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})"
                       @change="act('{{ cluster.ID }}', $event.target.checked ? 'check' : 'uncheck')"
                       class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Apply this Cluster
            </label>

            {% if cluster.State == "skipped" %}
            <button type="button" data-testid="cluster-reopen"
                    :disabled="busy"
                    @click="act('{{ cluster.ID }}', 'reopen')"
                    class="reduction-action">Reopen</button>
            {% else %}
            <button type="button" data-testid="cluster-skip"
                    :disabled="busy || !isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})"
                    @click="act('{{ cluster.ID }}', 'skip')"
                    class="reduction-action">Skip</button>
            {% endif %}
        </div>
        {% endif %}
    </header>

    {% if cluster.Lossy %}
    <p class="text-sm text-amber-800 px-3" role="note" data-testid="cluster-lossy">
        A Loser here holds a {{ cluster.Lossy|join:", " }} the Winner does not. Merging discards it.
    </p>
    {% endif %}

    {% if cluster.Withheld %}
    <p class="text-sm text-red-700 px-3" role="note" data-testid="cluster-withheld">
        {{ cluster.Withheld }} of these Resources are outside what you may see, so this Cluster cannot be applied.
    </p>
    {% endif %}

    {% if cluster.Oversized %}
    <p class="text-sm text-amber-800 px-3" role="note" data-testid="cluster-oversized">
        An unusually large Near-Identical Cluster. Expand it and look before you check it.
    </p>
    <p class="px-3" x-show="!isExpanded('{{ cluster.ID }}', true)">
        <button type="button" data-testid="cluster-expand"
                @click="expand('{{ cluster.ID }}')"
                aria-controls="cluster-{{ cluster.ID }}-members"
                class="reduction-action">Expand these {{ cluster.Members|length }} Resources</button>
    </p>
    {% endif %}

    <div class="reduction-cluster-members flex flex-wrap gap-3 p-3"
         id="cluster-{{ cluster.ID }}-members"
         x-show="isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})">
        {% for member in cluster.Members %}
        <div class="reduction-member{% if member.IsWinner %} reduction-member--winner{% endif %}{% if member.Ejected %} reduction-member--ejected{% endif %}"
             data-testid="reduction-member"
             data-resource-id="{{ member.ResourceID }}"
             {% if member.IsWinner %}data-winner="true"{% endif %}
             {% if member.Ejected %}data-ejected="true"{% endif %}>
            {% if member.Resource %}
                <a href="/v1/resource/view?id={{ member.Resource.ID }}&v={{ member.Resource.Hash }}#{{ member.Resource.ContentType }}"
                   @click.prevent="$store.lightbox.openFromClick($event, {{ member.Resource.ID }}, '{{ member.Resource.ContentType }}')"
                   data-lightbox-item
                   data-resource-id="{{ member.Resource.ID }}"
                   data-content-type="{{ member.Resource.ContentType }}"
                   data-resource-name="{{ member.Resource.Name }}"
                   data-resource-hash="{{ member.Resource.Hash }}"
                   data-resource-width="{{ member.Resource.Width }}"
                   data-resource-height="{{ member.Resource.Height }}">
                    <img height="120" src="/v1/resource/preview?id={{ member.Resource.ID }}&height=120&v={{ member.Resource.Hash }}"
                         alt="Preview of {{ member.Resource.Name }}" loading="lazy">
                </a>
                <p class="text-xs text-stone-700 mt-1 max-w-[10rem] truncate">
                    <a class="reduction-member-link" href="/resource?id={{ member.Resource.ID }}" title="{{ member.Resource.Name }}">{{ member.Resource.Name }}</a>
                </p>
                <p class="text-xs text-stone-500">
                    {% if member.Resource.Width %}{{ member.Resource.Width }}&times;{{ member.Resource.Height }} &middot; {% endif %}
                    {{ member.Resource.FileSize | humanReadableSize }}
                </p>
            {% else %}
                <p class="text-xs text-stone-500 max-w-[10rem]">Resource {{ member.ResourceID }} is outside what you may see.</p>
            {% endif %}

            <p class="text-xs mt-1">
                {% if member.IsWinner %}
                    <span class="reduction-badge reduction-badge--winner">Winner</span>
                {% elif member.Ejected %}
                    <span class="reduction-badge reduction-badge--ejected">Ejected</span>
                {% else %}
                    <span class="reduction-badge reduction-badge--loser">Will be deleted</span>
                {% endif %}
                {% if not member.InExtent %}
                    <span class="reduction-badge reduction-badge--outside">Outside the Extent</span>
                {% endif %}
                {% if member.Distance != nil %}
                    <span class="reduction-badge reduction-badge--distance">distance {{ member.Distance }}</span>
                {% endif %}
            </p>

            {% if cluster.State != "applied" %}
            <div class="flex flex-wrap gap-1 mt-1">
                {% if not member.IsWinner and not member.Ejected %}
                <button type="button" data-testid="member-promote"
                        :disabled="busy"
                        @click="act('{{ cluster.ID }}', 'promote', {{ member.ResourceID }})"
                        class="reduction-action reduction-action--small">Make Winner</button>
                {% endif %}
                {% if member.Ejected %}
                <button type="button" data-testid="member-restore"
                        :disabled="busy"
                        @click="act('{{ cluster.ID }}', 'restore', {{ member.ResourceID }})"
                        class="reduction-action reduction-action--small">Put back</button>
                {% elif not member.IsWinner %}
                <button type="button" data-testid="member-eject"
                        :disabled="busy"
                        @click="act('{{ cluster.ID }}', 'eject', {{ member.ResourceID }})"
                        class="reduction-action reduction-action--small">Eject</button>
                {% endif %}
            </div>
            {% endif %}
        </div>
        {% endfor %}
    </div>

    {% if cluster.Members|length == 2 and cluster.Winner %}
    <p class="px-3 pb-3 text-sm">
        <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700"
           href="/resource/compare?r1={{ cluster.Members.0.ResourceID }}&r2={{ cluster.Members.1.ResourceID }}"
           data-testid="cluster-compare-link">Compare these two side by side</a>
    </p>
    {% endif %}
</article>

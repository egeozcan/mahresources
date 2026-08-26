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
    {% endif %}

    <div class="reduction-cluster-members flex flex-wrap gap-3 p-3">
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
                    <a href="/resource?id={{ member.Resource.ID }}" title="{{ member.Resource.Name }}">{{ member.Resource.Name }}</a>
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

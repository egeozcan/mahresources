{# One copy in a Cluster. Included twice by reductionCluster.tpl, the kept copy first. #}
{# A member with no Resource behind it is one of two things, and they read very        #}
{# differently. On an applied Cluster it is a copy this Reduction just merged away.     #}
{# Anywhere else it is a Resource outside what this reviewer may see, rendered as a     #}
{# placeholder and nothing else (no id, no role, no distance, no controls), because     #}
{# printing any of those answers "what is over there?" for somebody whose whole         #}
{# confinement is that they may not ask.                                                #}
{% if not member.Resource %}
<div class="reduction-member reduction-member--gone" data-testid="reduction-member-withheld">
    <p class="text-xs text-stone-600 max-w-[10rem]">
        {% if cluster.Merged %}Merged and removed.{% elif cluster.State == "stale" %}Removed elsewhere.{% else %}Resource unavailable.{% endif %}
    </p>
    {# A copy that is gone is left out of a stale Cluster the same way as one that is not: this reopens the rest for review. #}
    {% if cluster.State == "stale" and not member.IsWinner and not member.Ejected and not cluster.WinnerChanged %}
    <button type="button" data-testid="member-eject" class="reduction-btn reduction-btn--small"
            :disabled="$store.reductionReview.busy"
            @click="$store.reductionReview.act('{{ cluster.ID }}', 'eject', {{ member.ResourceID }})">Don't remove<span class="sr-only"> (the copy that was removed elsewhere)</span></button>
    {% endif %}
</div>
{% else %}
<div class="reduction-member {% if member.IsWinner %}reduction-member--keep{% elif member.Ejected %}reduction-member--ejected{% else %}reduction-member--remove{% endif %}"
     key="{{ member.ResourceID }}"
     data-testid="reduction-member"
     data-resource-id="{{ member.ResourceID }}"
     {% if member.IsWinner %}data-winner="true"{% endif %}
     {% if member.Ejected %}data-ejected="true"{% endif %}>
    <p class="reduction-roles">
        {% if member.IsWinner %}
            <span class="reduction-badge reduction-badge--keep" data-testid="member-role">Keep</span>
        {% elif member.Ejected %}
            <span class="reduction-badge reduction-badge--ejected" data-testid="member-role">Not included</span>
        {% else %}
            <span class="reduction-badge reduction-badge--remove" data-testid="member-role">Remove</span>
        {% endif %}
        {% if member.Changed %}
            <span class="reduction-badge reduction-badge--changed" data-testid="member-changed">{% if member.IsWinner %}Kept copy changed{% else %}This copy changed{% endif %}</span>
        {% endif %}
        {% if not member.InExtent %}
            <span class="reduction-badge reduction-badge--outside">Outside the selection</span>
        {% endif %}
        {% if member.DistanceLabel %}
            {% set dist = member.Distance|integer %}
            <span class="reduction-badge reduction-badge--distance" title="perceptual distance {{ dist }}" data-testid="member-distance">{% if dist <= 2 %}Very close{% elif dist <= 6 %}Close{% else %}Similar{% endif %}<span class="sr-only"> (perceptual distance {{ dist }})</span></span>
        {% endif %}
    </p>

    <a href="/v1/resource/view?id={{ member.Resource.ID }}&v={{ member.Resource.Hash }}#{{ member.Resource.ContentType }}"
       @click.prevent="$store.lightbox.openFromClick($event, {{ member.Resource.ID }}, '{{ member.Resource.ContentType|escapejs }}')"
       role="button" aria-label="Preview {{ member.Resource.Name }}"
       @keydown.space.prevent="$el.click()"
       data-lightbox-item
       data-resource-id="{{ member.Resource.ID }}"
       data-content-type="{{ member.Resource.ContentType }}"
       data-resource-name="{{ member.Resource.Name }}"
       data-resource-hash="{{ member.Resource.Hash }}"
       data-resource-width="{{ member.Resource.Width }}"
       data-resource-height="{{ member.Resource.Height }}">
        {# A preview that fails to load (no thumbnail, a failed fetch) is swapped for a type placeholder; the link's own label already names the Resource. #}
        <img height="160" src="/v1/resource/preview?id={{ member.Resource.ID }}&height=160&v={{ member.Resource.Hash }}"
             alt="" loading="lazy" decoding="async"
             x-init="if ($el.complete && $el.src && !$el.naturalWidth) { $el.hidden = true; $el.nextElementSibling.hidden = false }"
             @error="$el.hidden = true; $el.nextElementSibling.hidden = false">
        <span class="reduction-thumb-placeholder" data-testid="member-thumb-placeholder" hidden>
            <svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
            <span class="reduction-thumb-type">{{ member.Resource.ContentType }}</span>
        </span>
    </a>
    <p class="reduction-member-name">
        <a class="reduction-member-link" href="/resource?id={{ member.Resource.ID }}" title="{{ member.Resource.Name }}">{{ member.Resource.Name }}</a>
    </p>

    {# Facts, compared with the kept copy. A value that differs is bold, led by a not-equal sign (a cue that is not colour) and named for a screen reader. #}
    {% set kept = cluster.Winner %}
    {% set compare = (not member.IsWinner) and kept %}
    <ul class="reduction-facts" aria-label="Details of {{ member.Resource.Name }}">
        {% if member.Resource.Width %}
            {% if compare and (member.Resource.Width != kept.Width or member.Resource.Height != kept.Height) %}
            <li class="reduction-fact--differs"><span aria-hidden="true">&ne; </span><strong>{{ member.Resource.Width }}&times;{{ member.Resource.Height }}</strong><span class="sr-only"> (differs from the kept copy)</span></li>
            {% else %}
            <li>{{ member.Resource.Width }}&times;{{ member.Resource.Height }}</li>
            {% endif %}
        {% endif %}
        {% if compare and member.Resource.FileSize != kept.FileSize %}
        <li class="reduction-fact--differs"><span aria-hidden="true">&ne; </span><strong>{{ member.Resource.FileSize | humanReadableSize }}</strong><span class="sr-only"> (differs from the kept copy)</span></li>
        {% else %}
        <li>{{ member.Resource.FileSize | humanReadableSize }}</li>
        {% endif %}
        {% if compare and member.Resource.ContentType != kept.ContentType %}
        <li class="reduction-fact--differs"><span aria-hidden="true">&ne; </span><strong>{{ member.Resource.ContentType }}</strong><span class="sr-only"> (differs from the kept copy)</span></li>
        {% else %}
        <li>{{ member.Resource.ContentType }}</li>
        {% endif %}
        {% if compare and member.Resource.CreatedAt|date:"2006-01-02" != kept.CreatedAt|date:"2006-01-02" %}
        <li class="reduction-fact--differs"><span aria-hidden="true">&ne; </span><strong>Added {{ member.Resource.CreatedAt|date:"2006-01-02" }}</strong><span class="sr-only"> (differs from the kept copy)</span></li>
        {% else %}
        <li>Added {{ member.Resource.CreatedAt|date:"2006-01-02" }}</li>
        {% endif %}
        {% if compare %}
            {% for w in cluster.Members %}{% if w.IsWinner %}
                {% if member.TagCount != w.TagCount or member.NoteCount != w.NoteCount or member.GroupCount != w.GroupCount %}
                <li class="reduction-fact--differs"><span aria-hidden="true">&ne; </span><strong>{{ member.TagCount }} tag{% if member.TagCount != 1 %}s{% endif %}, {{ member.NoteCount }} note{% if member.NoteCount != 1 %}s{% endif %}, {{ member.GroupCount }} group{% if member.GroupCount != 1 %}s{% endif %}</strong><span class="sr-only"> (differs from the kept copy)</span></li>
                {% else %}
                <li>{{ member.TagCount }} tag{% if member.TagCount != 1 %}s{% endif %}, {{ member.NoteCount }} note{% if member.NoteCount != 1 %}s{% endif %}, {{ member.GroupCount }} group{% if member.GroupCount != 1 %}s{% endif %}</li>
                {% endif %}
            {% endif %}{% endfor %}
        {% else %}
        <li>{{ member.TagCount }} tag{% if member.TagCount != 1 %}s{% endif %}, {{ member.NoteCount }} note{% if member.NoteCount != 1 %}s{% endif %}, {{ member.GroupCount }} group{% if member.GroupCount != 1 %}s{% endif %}</li>
        {% endif %}
    </ul>

    <div class="reduction-member-actions">
    {% if cluster.State == "stale" %}
        {% if not member.IsWinner and not member.Ejected and not cluster.WinnerChanged %}
        <button type="button" data-testid="member-eject" class="reduction-btn reduction-btn--small"
                :disabled="$store.reductionReview.busy"
                @click="$store.reductionReview.act('{{ cluster.ID }}', 'eject', {{ member.ResourceID }})">Don't remove<span class="sr-only"> ({{ member.Resource.Name }})</span></button>
        {% endif %}
    {% elif cluster.State != "applied" %}
        {% if not member.IsWinner and not member.Ejected %}
        <button type="button" data-testid="member-promote" class="reduction-btn reduction-btn--small"
                :disabled="$store.reductionReview.busy"
                @click="$store.reductionReview.act('{{ cluster.ID }}', 'promote', {{ member.ResourceID }})">Keep this instead<span class="sr-only"> ({{ member.Resource.Name }})</span></button>
        {% endif %}
        {% if member.IsLoser and cluster.Winner and not member.Ejected %}
        {# Story 24 wants the two-up look from every Cluster, and the compare page takes exactly two Resources, #}
        {# so the pair worth looking at is this copy against the kept one that would absorb it.               #}
        <a class="reduction-btn reduction-btn--small" data-testid="member-compare"
           href="/resource/compare?r1={{ cluster.Winner.ID }}&r2={{ member.ResourceID }}&from=reduction:{{ raw.ID }}"
           @click="$store.reductionReview.compareHref($event, '{{ cluster.ID }}')">Compare<span class="sr-only"> {{ member.Resource.Name }} with the kept copy</span></a>
        {% endif %}
        {# "Include again" undoes a "Don't remove" by re-proposing the deletion, which an out-of-selection member may never be part of. #}
        {% if member.Ejected and member.InExtent %}
        <button type="button" data-testid="member-restore" class="reduction-btn reduction-btn--small"
                :disabled="$store.reductionReview.busy"
                @click="$store.reductionReview.act('{{ cluster.ID }}', 'restore', {{ member.ResourceID }})">Include again<span class="sr-only"> ({{ member.Resource.Name }})</span></button>
        {% elif not member.IsWinner and not member.Ejected %}
        <button type="button" data-testid="member-eject" class="reduction-btn reduction-btn--small"
                :disabled="$store.reductionReview.busy"
                @click="$store.reductionReview.act('{{ cluster.ID }}', 'eject', {{ member.ResourceID }})">Don't remove<span class="sr-only"> ({{ member.Resource.Name }})</span></button>
        {% endif %}
    {% endif %}
    </div>
</div>
{% endif %}

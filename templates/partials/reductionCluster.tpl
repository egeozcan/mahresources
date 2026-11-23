{# One Cluster of a Resource Reduction.                                          #}
{#                                                                               #}
{# The justification leads and the thumbnails follow, because the whole point of  #}
{# the design is that reviewing is reading a justification rather than squinting  #}
{# at thumbnails.                                                                #}
{#                                                                               #}
{# data-lightbox-scope makes the members their own gallery: flicking through five #}
{# candidates must not drag in the rest of the page.                              #}
{% if cluster.Withheld %}
{# No id and no tier on this branch. The Cluster id is a hash of the tier and the #}
{# member ids, and ids here are small integers — published beside one visible     #}
{# member it is enough to recover the hidden one by enumeration, which is the     #}
{# disclosure the branch exists to prevent. The heading is anchored on the        #}
{# Cluster's position on the page instead.                                        #}
{# The key anchors Alpine.morph, which the review store's refresh uses to        #}
{# re-render the Clusters in place. Without it the morph matches the articles    #}
{# positionally, so when a filter (or the action itself) removes the Cluster     #}
{# that was acted on, the node behind it is patched into the shape of the next   #}
{# one and keeps that one's live DOM state — a checkbox the reviewer just        #}
{# clicked moves onto the neighbouring Cluster. The withheld branch publishes no #}
{# Cluster id by design, so it anchors on its position instead.                  #}
<article class="card reduction-cluster"
         key="withheld-{{ cluster.Position }}"
         data-testid="reduction-cluster"
         aria-labelledby="cluster-withheld-{{ cluster.Position }}-heading">
    {# A Cluster reaching a Resource this reviewer may not see is not rendered as #}
    {# a proposal at all. Its Winner, the criterion that chose it, the margin, the #}
    {# member count and the curation warning are all statements *about* that       #}
    {# Resource — "there is a higher-resolution copy over there" is precisely what #}
    {# a confined account may not be told. It cannot be applied either, which the  #}
    {# apply path enforces independently of this.                                  #}
    <header class="card-header card-header--compact">
        <div class="card-title-section">
            <h3 class="card-title" id="cluster-withheld-{{ cluster.Position }}-heading">Cluster unavailable</h3>
        </div>
    </header>
    <p class="text-sm text-stone-600 px-3 pb-3" data-testid="cluster-withheld">
        You do not have access to all resources in this Cluster. It cannot be reviewed or applied.
    </p>
</article>
{% else %}
{% if cluster.State == "applied" %}
{# An applied Cluster is a record, not a decision: one line and the kept copy. #}
<article class="card reduction-cluster reduction-cluster--applied"
         key="{{ cluster.ID }}"
         tabindex="-1"
         data-testid="reduction-cluster"
         data-cluster-id="{{ cluster.ID }}"
         data-cluster-tier="{{ cluster.Tier }}"
         data-cluster-state="applied"
         aria-labelledby="cluster-{{ cluster.ID }}-heading">
    <h3 class="reduction-cluster-title" id="cluster-{{ cluster.ID }}-heading">
        {% if cluster.Tier == "identical" %}Identical{% else %}Near-Identical{% endif %} &middot; {{ cluster.Members|length }} copies
    </h3>
    <p class="text-sm text-stone-700">
        <span class="reduction-state" data-testid="cluster-state">{{ cluster.StateLabel }}</span>.
        {% if cluster.Winner %}Kept <a class="reduction-link" href="/resource?id={{ cluster.Winner.ID }}">{{ cluster.Winner.Name }}</a>.{% endif %}
        {% if cluster.Merged %}The other copies were merged into it and removed.{% else %}The apply claimed this Cluster and has not confirmed the merge yet; the other copies still exist.{% endif %}
    </p>
</article>
{% else %}
<article class="card reduction-cluster{% if cluster.NeedsAttention %} reduction-cluster--attention{% endif %}"
         key="{{ cluster.ID }}"
         tabindex="-1"
         data-testid="reduction-cluster"
         data-cluster-id="{{ cluster.ID }}"
         data-cluster-tier="{{ cluster.Tier }}"
         data-cluster-state="{% if cluster.State == "skipped" %}skipped{% elif cluster.State == "stale" %}stale{% else %}open{% endif %}"
         data-cluster-oversized="{% if cluster.Oversized %}true{% else %}false{% endif %}"
         data-lightbox-scope
         aria-labelledby="cluster-{{ cluster.ID }}-heading">

    <header class="reduction-cluster-head" key="head">
        <div class="reduction-cluster-heading">
            <h3 class="reduction-cluster-title" id="cluster-{{ cluster.ID }}-heading">
                {% if cluster.Tier == "identical" %}Identical{% else %}Near-Identical{% endif %}
                &middot; {{ cluster.Members|length }} copies
            </h3>
            <span class="reduction-state" data-testid="cluster-state">{{ cluster.StateLabel }}</span>
        </div>

        {# The reason is one sentence. A Cluster the reviewer re-chose says so, and a tie says what broke it. #}
        <p class="reduction-reason" data-testid="cluster-decided-by">
            {% if cluster.Winner %}
                {% if cluster.Undecided %}
                    All copies tie on every criterion; keeping the one added first.
                {% else %}
                    Keeping &ldquo;{{ cluster.Winner.Name }}&rdquo;: {{ cluster.DecidedByLabel }}{% if cluster.Margin %}, {{ cluster.Margin }}{% endif %}.
                {% endif %}
            {% else %}
                The kept copy is withheld.
            {% endif %}
        </p>

        {% if cluster.State != "applied" and cluster.State != "stale" %}
        <div class="reduction-cluster-controls">
            {% if cluster.State == "skipped" %}
            <button type="button" data-testid="cluster-reopen" class="reduction-btn"
                    :disabled="$store.reductionReview.busy"
                    @click="$store.reductionReview.act('{{ cluster.ID }}', 'reopen')">Reopen</button>
            {% else %}
            {% if cluster.LoserCount == 0 %}
            {# Every other copy was left out: there is nothing to approve, so no toggle to press (the server refuses it too). Skip stays. #}
            <p class="reduction-nothing" data-testid="cluster-nothing-to-remove">Nothing to remove: every other copy is kept.</p>
            {% else %}
            {# aria-pressed with a constant accessible name: the visible text changes, the name does not. #}
            {# The disabled binding holds every reason; a static one would be stripped at init.            #}
            <button type="button" data-testid="cluster-approve" class="reduction-btn reduction-approve"
                    aria-pressed="{% if cluster.Checked %}true{% else %}false{% endif %}"
                    :aria-keyshortcuts="$store.reductionReview.shortcutsOn ? 'A' : null"
                    disabled
                    :disabled="$store.reductionReview.busy || !$store.reductionReview.isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})"
                    @click="$store.reductionReview.toggleApproval('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %}, $event)">
                <span class="sr-only">Approve</span>
                <span aria-hidden="true">{% if cluster.Checked %}Approved &#10003;{% else %}Approve{% endif %}</span>
            </button>
            {% endif %}
            <button type="button" data-testid="cluster-skip" class="reduction-btn"
                    :aria-keyshortcuts="$store.reductionReview.shortcutsOn ? 'S' : null"
                    :disabled="$store.reductionReview.busy || !$store.reductionReview.isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})"
                    @click="$store.reductionReview.act('{{ cluster.ID }}', 'skip')">Skip</button>
            {% endif %}
        </div>
        {% endif %}
    </header>

    {% if cluster.State == "stale" %}
    {# A stale Cluster is a record of why it cannot be applied, not a proposal: no approve, skip or per-copy buttons. #}
    <div class="reduction-warning" role="note" key="stale" data-testid="cluster-stale-reason">
        <p><strong>This Cluster cannot be applied:</strong>
        {% if cluster.StaleReason %}{{ cluster.StaleReason }}.{% else %}it no longer matches what was reviewed.{% endif %}
        {% if cluster.WinnerChanged %}The kept copy itself changed, so leaving other copies out will not help: start a new Reduction over the same selection.{% else %}Use Don't remove on a copy that changed or went to review the rest again.{% endif %}</p>
    </div>
    {% endif %}

    {% if cluster.KeepsSmaller %}
    <div class="reduction-warning" role="note" key="keeps-smaller" data-testid="cluster-keeps-smaller">
        <p>The kept copy has a lower resolution than a copy being removed ({{ cluster.KeptDimensions }} vs {{ cluster.LargerDimensions }}).</p>
        {% if cluster.State != "skipped" and cluster.LargestMemberID %}
        <button type="button" data-testid="cluster-keep-larger" class="reduction-btn"
                :disabled="$store.reductionReview.busy"
                @click="$store.reductionReview.act('{{ cluster.ID }}', 'promote', {{ cluster.LargestMemberID }})">Keep the larger copy instead</button>
        {% endif %}
    </div>
    {% endif %}

    {% if cluster.Lossy %}
    <p class="reduction-warning reduction-warning--text" role="note" key="lossy" data-testid="cluster-lossy">
        Merging will discard these fields from one or more removed copies: {{ cluster.Lossy|join:", " }}.
    </p>
    {% endif %}

    {% if cluster.Oversized %}
    <p class="reduction-warning reduction-warning--text" role="note" key="oversized" data-testid="cluster-oversized">
        Large Near-Identical Cluster. Expand to review before approving it.
    </p>
    <p class="px-3" key="expand" x-show="!$store.reductionReview.isExpanded('{{ cluster.ID }}', true)">
        <button type="button" data-testid="cluster-expand" class="reduction-btn"
                :aria-keyshortcuts="$store.reductionReview.shortcutsOn ? 'X' : null"
                @click="$store.reductionReview.expand('{{ cluster.ID }}')"
                aria-controls="cluster-{{ cluster.ID }}-members">Show {{ cluster.Members|length }} copies</button>
    </p>
    {% endif %}

    <div class="reduction-cluster-members" key="members"
         id="cluster-{{ cluster.ID }}-members"
         x-show="$store.reductionReview.isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})">
        {# The kept copy first, then the rest in the plan's order. #}
        {% for member in cluster.Members %}{% if member.IsWinner %}{% include "/partials/reductionMember.tpl" %}{% endif %}{% endfor %}
        {% for member in cluster.Members %}{% if not member.IsWinner %}{% include "/partials/reductionMember.tpl" %}{% endif %}{% endfor %}
    </div>

    <p class="reduction-footnote" key="footnote">Removed copies' tags, notes and groups move to the kept copy.</p>
</article>
{% endif %}
{% endif %}

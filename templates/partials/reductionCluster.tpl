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
         data-cluster-version="{{ raw.Version }}"
         data-cluster-merged="{% if cluster.Merged %}true{% else %}false{% endif %}"
         data-member-count="{% if cluster.MemberCount %}{{ cluster.MemberCount }}{% else %}{{ cluster.Members|length }}{% endif %}"
         aria-labelledby="cluster-{{ cluster.ID }}-heading">
    <h3 class="reduction-cluster-title" id="cluster-{{ cluster.ID }}-heading">
        {% if cluster.Tier == "identical" %}Identical{% else %}Near-Identical{% endif %} &middot; {% if cluster.MemberCount %}{{ cluster.MemberCount }}{% else %}{{ cluster.Members|length }}{% endif %} copies
    </h3>
    <p class="text-sm text-stone-700">
        <span class="reduction-state" data-testid="cluster-state">{{ cluster.StateLabel }}</span>.
        {% if cluster.Winner %}Kept <a class="reduction-link" href="/resource?id={{ cluster.Winner.ID }}">{{ cluster.Winner.Name }}</a>.{% endif %}
        {% if cluster.Merged %}{{ cluster.MergedCount }} cop{% if cluster.MergedCount != 1 %}ies{% else %}y{% endif %} removed{% if cluster.LeftOutCount %}, {{ cluster.LeftOutCount }} left out{% endif %}.{% else %}The apply claimed this Cluster and has not confirmed the merge yet; the other copies still exist.{% endif %}
    </p>
    {# A record past one card's worth of copies pages the rest the way an open Cluster does. #}
    {% if cluster.MoreMembers %}
    <div class="reduction-cluster-members" key="members" id="cluster-{{ cluster.ID }}-members">
        {% for member in cluster.Members %}{% if member.IsWinner %}{% include "/partials/reductionMember.tpl" %}{% endif %}{% endfor %}
        {% for member in cluster.Members %}{% if not member.IsWinner %}{% include "/partials/reductionMember.tpl" %}{% endif %}{% endfor %}
    </div>
    <div class="px-3 pb-2" key="more-members" data-testid="cluster-more-members-wrap">
        <p class="text-sm text-stone-700" data-testid="cluster-shown-count" id="cluster-{{ cluster.ID }}-shown">Showing {{ cluster.MembersShown }} of {{ cluster.MemberCount }} copies.</p>
        <button type="button" class="reduction-btn mt-1" data-testid="cluster-more-members"
                data-members-after="{{ cluster.MembersAfter }}"
                aria-controls="cluster-{{ cluster.ID }}-members"
                aria-describedby="cluster-{{ cluster.ID }}-shown"
                @click="$store.reductionReview.loadMoreMembers('{{ cluster.ID }}', $event.currentTarget)">Show more members</button>
    </div>
    {% endif %}
</article>
{% else %}
<article class="card reduction-cluster{% if cluster.NeedsAttention %} reduction-cluster--attention{% endif %}"
         key="{{ cluster.ID }}"
         tabindex="-1"
         data-testid="reduction-cluster"
         data-cluster-id="{{ cluster.ID }}"
         data-cluster-tier="{{ cluster.Tier }}"
         data-cluster-version="{{ raw.Version }}"
         data-cluster-state="{% if cluster.State == "skipped" %}skipped{% elif cluster.State == "stale" %}stale{% else %}open{% endif %}"
         data-cluster-oversized="{% if cluster.Oversized %}true{% else %}false{% endif %}"
         data-member-count="{% if cluster.MemberCount %}{{ cluster.MemberCount }}{% else %}{{ cluster.Members|length }}{% endif %}"
         data-cluster-fixed-keep="{% if cluster.DecidedBy == "seed" %}true{% else %}false{% endif %}"
         data-cluster-winner-changed="{% if cluster.WinnerChanged %}true{% else %}false{% endif %}"
         data-lightbox-scope
         aria-labelledby="cluster-{{ cluster.ID }}-heading">

    <header class="reduction-cluster-head" key="head">
        <div class="reduction-cluster-heading">
            <h3 class="reduction-cluster-title" id="cluster-{{ cluster.ID }}-heading">
                {% if cluster.Tier == "identical" %}Identical{% else %}Near-Identical{% endif %}
                &middot; {% if cluster.MemberCount %}{{ cluster.MemberCount }}{% else %}{{ cluster.Members|length }}{% endif %} copies
            </h3>
            <span class="reduction-state" data-testid="cluster-state">{{ cluster.StateLabel }}</span>
        </div>

        {# The reason is one sentence. A Cluster the reviewer re-chose says so, and a tie says what broke it. #}
        <p class="reduction-reason" data-testid="cluster-decided-by"{% if cluster.Winner and not cluster.Undecided %} title="{{ cluster.Winner.Name }}"{% endif %}>
            {% if cluster.Winner %}
                {% if cluster.Undecided %}
                    All copies tie on every criterion; keeping the one added first.
                {% else %}
                    {# The visible name is cut so a URL-length name cannot swell the header; the full sentence stays for screen readers and in the title. #}
                    {# A full Near star keeps its seed: the label is a justification of its own ("kept: ..."), not a rule criterion, so it follows the name in brackets. #}
                    {% if cluster.DecidedBy == "seed" %}
                    <span aria-hidden="true">Keeping &ldquo;{{ cluster.Winner.Name|truncatechars:60 }}&rdquo; ({{ cluster.DecidedByLabel }}).</span>
                    <span class="sr-only">Keeping &ldquo;{{ cluster.Winner.Name }}&rdquo; ({{ cluster.DecidedByLabel }}).</span>
                    {% else %}
                    <span aria-hidden="true">Keeping &ldquo;{{ cluster.Winner.Name|truncatechars:60 }}&rdquo;: {{ cluster.DecidedByLabel }}{% if cluster.Margin %}, {{ cluster.Margin }}{% endif %}.</span>
                    <span class="sr-only">Keeping &ldquo;{{ cluster.Winner.Name }}&rdquo;: {{ cluster.DecidedByLabel }}{% if cluster.Margin %}, {{ cluster.Margin }}{% endif %}.</span>
                    {% endif %}
                {% endif %}
            {% else %}
                {% if cluster.State == "stale" and cluster.WinnerChanged %}The kept copy was changed or deleted.{% else %}The kept copy is withheld.{% endif %}
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
                    {% if cluster.Oversized and cluster.MoreMembers %}:aria-describedby="$store.reductionReview.isReviewable('{{ cluster.ID }}', true, true) ? null : 'cluster-{{ cluster.ID }}-approve-hint'"{% endif %}
                    disabled
                    :disabled="$store.reductionReview.busy || !$store.reductionReview.isReviewable('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %}, {% if cluster.MoreMembers %}true{% else %}false{% endif %})"
                    @click="$store.reductionReview.toggleApproval('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %}, $event)">
                {# The visible text is the name in both states (WCAG 2.5.3); the state is aria-pressed, shown as a check and a filled button. #}
                <span class="reduction-approve-check" aria-hidden="true">&#10003;</span>Approve
            </button>
            {% if cluster.Oversized and cluster.MoreMembers %}
            {# Shown once the Cluster is expanded and until every copy is on the page; the button names it with aria-describedby. #}
            <p class="reduction-nothing" id="cluster-{{ cluster.ID }}-approve-hint" data-testid="cluster-approve-needs-members"
               x-show="$store.reductionReview.isExpanded('{{ cluster.ID }}', true) && !$store.reductionReview.isReviewable('{{ cluster.ID }}', true, true)">
                Approving needs all {{ cluster.MemberCount }} copies on screen: use Show more members until every copy is listed.
            </p>
            {% endif %}
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
    {% if cluster.StaleReason == "too large: compute again" %}
    {# A Cluster found before the 200-copy cap, kept as history: no decision on it can be written. #}
    <div class="reduction-warning" role="note" key="stale" data-testid="cluster-stale-reason" data-cluster-read-only="true">
        <p><strong>This Cluster is read-only:</strong> it holds more copies than a Cluster can hold now, so it cannot be changed or applied. Compute again to review these copies in Clusters of up to 200.</p>
    </div>
    {% else %}
    <div class="reduction-warning" role="note" key="stale" data-testid="cluster-stale-reason">
        <p><strong>This Cluster cannot be applied:</strong>
        {% if cluster.StaleReason == "left the selection" %}a copy it would remove left the selection after the last compute.{% elif cluster.StaleReason %}{{ cluster.StaleReason }}.{% else %}it no longer matches what was reviewed.{% endif %}
        {% if cluster.WinnerChanged %}The kept copy changed or was deleted, so leaving other copies out will not help: start a new Reduction for these copies.{% else %}Use Don't remove on a copy that changed or went to review the rest again.{% endif %}</p>
    </div>
    {% endif %}
    {% endif %}

    {% if cluster.KeepsSmaller %}
    <div class="reduction-warning" role="note" key="keeps-smaller" data-testid="cluster-keeps-smaller">
        <p>The kept copy has a lower resolution than a copy being removed{% if cluster.LargerDimensions %} ({{ cluster.KeptDimensions }} vs {{ cluster.LargerDimensions }}){% endif %}.</p>
        {% if cluster.State != "skipped" and cluster.LargestMemberID %}
        <button type="button" data-testid="cluster-keep-larger" class="reduction-btn"
                {% if cluster.DecidedBy == "seed" %}aria-describedby="cluster-{{ cluster.ID }}-fixed-note"{% endif %}
                :disabled="$store.reductionReview.busy{% if cluster.DecidedBy == "seed" %} || true{% endif %}"
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
                aria-controls="cluster-{{ cluster.ID }}-members">{% if cluster.MoreMembers %}Show copies{% else %}Show {{ cluster.Members|length }} copies{% endif %}</button>
    </p>
    {% endif %}

    {% if cluster.DecidedBy == "seed" %}
    <p class="reduction-warning reduction-warning--text" role="note" key="fixed-keep" id="cluster-{{ cluster.ID }}-fixed-note" data-testid="cluster-fixed-keep">
        This Cluster is at the size limit; its kept copy is fixed, so another copy cannot be kept instead.
    </p>
    {% endif %}

    {% if cluster.Truncated > 0 %}
    <p class="reduction-warning reduction-warning--text" role="note" key="truncated" data-testid="cluster-truncated">
        {% if cluster.Tier == "identical" %}Showing {{ cluster.MemberCount }} copies; {{ cluster.Truncated }} more {% if cluster.Truncated == 1 %}was{% else %}were{% endif %} left out. Apply this Cluster, then compute again for the rest.{% else %}Showing {{ cluster.MemberCount }} copies; there are more. Apply this Cluster, then compute again for the rest.{% endif %}
    </p>
    {% endif %}

    <div class="reduction-cluster-members" key="members"
         id="cluster-{{ cluster.ID }}-members"
         x-show="$store.reductionReview.isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})">
        {# The kept copy first, then the rest in the plan's order. #}
        {% for member in cluster.Members %}{% if member.IsWinner %}{% include "/partials/reductionMember.tpl" %}{% endif %}{% endfor %}
        {% for member in cluster.Members %}{% if not member.IsWinner %}{% include "/partials/reductionMember.tpl" %}{% endif %}{% endfor %}
    </div>

    {% if cluster.MoreMembers %}
    <div class="px-3 pb-2" key="more-members" data-testid="cluster-more-members-wrap"
         x-show="$store.reductionReview.isExpanded('{{ cluster.ID }}', {% if cluster.Oversized %}true{% else %}false{% endif %})">
        <p class="text-sm text-stone-700" data-testid="cluster-shown-count" id="cluster-{{ cluster.ID }}-shown">Showing {{ cluster.MembersShown }} of {{ cluster.MemberCount }} copies.</p>
        <button type="button" class="reduction-btn mt-1" data-testid="cluster-more-members"
                data-members-after="{{ cluster.MembersAfter }}"
                aria-controls="cluster-{{ cluster.ID }}-members"
                aria-describedby="cluster-{{ cluster.ID }}-shown"
                @click="$store.reductionReview.loadMoreMembers('{{ cluster.ID }}', $event.currentTarget)">Show more members</button>
    </div>
    {% endif %}

    {% if cluster.State != "stale" %}
    <p class="reduction-footnote" key="footnote">Removed copies' tags, notes and groups move to the kept copy.</p>
    {% endif %}
</article>
{% endif %}
{% endif %}

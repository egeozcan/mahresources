{% extends "/layouts/base.tpl" %}

{# Before the sidebar in the tab order: base.tpl renders this block ahead of the content grid. #}
{% block skiplinks %}
{% if raw.ComputedAt and reduction.StatusEffective != "computing" %}
<nav aria-label="Skip links">
<a href="#reduction-clusters" data-testid="reduction-skip-link"
   class="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:p-3 focus:bg-white focus:text-amber-800 focus:underline">Skip to Clusters</a>
{% if statusTab == "open" %}
<a href="#reduction-apply-button" data-testid="reduction-skip-apply" x-data x-show="$store.reductionReview.tab === 'open' && ($store.reductionReview.openCount > 0 || $store.reductionReview.checkedCount > 0)" x-cloak @click.prevent="$store.reductionReview.focusApply()"
   class="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:p-3 focus:bg-white focus:text-amber-800 focus:underline">Skip to Apply</a>
{% endif %}
</nav>
{% endif %}
{% endblock %}

{# What the mobile panel is: the review's Settings and Delete, not filters. #}
{% block sidebarLabel %}Settings{% endblock %}

{% block body %}
{# The review is one column: status, notices, tabs, the Clusters, a sticky apply bar and a collapsed About. #}
{# Everything between data-reduction-live and the bar is re-rendered in place after each action, by       #}
{# Alpine.morph (src/components/reductionReview.js refresh). The bar, About and the dialogs are not, so   #}
{# their state (an open menu, an open details) survives; the bar reads the store instead.                  #}
{% set showReview = raw.ComputedAt and reduction.StatusEffective != "computing" %}
<div x-data
     x-init="$store.reductionReview.seed()"
     data-testid="reduction-page"
     data-reduction-id="{{ raw.ID }}"
     data-reduction-version="{{ raw.Version }}"
     data-reduction-checked="{{ review.CheckedCount }}"
     data-reduction-checked-losers="{{ review.CheckedLoserCount }}">
    {# The state the store is seeded from, and re-read from each refreshed page. #}
    <script type="application/json" data-reduction-state>{"id": {{ raw.ID }}, "version": {{ raw.Version }}, "status": "{{ reduction.StatusEffective }}", "jobId": "{{ raw.ComputeJobID|escapejs }}", "checked": {{ review.CheckedCount }}, "checkedLosers": {{ review.CheckedLoserCount }}, "open": {{ tabCounts.open }}, "tab": "{{ statusTab }}", "figures": {{ applyFigures|json|safe }}, "bulk": {{ bulk|json|safe }}, "applying": {% if lastApply and lastApply.running and not lastApply.interrupted %}true{% else %}false{% endif %}, "applyToken": "{{ applyToken|escapejs }}", "lastApplyStartedAt": "{% if lastApply %}{{ lastApply.startedAt|escapejs }}{% endif %}", "settings": {"name": "{{ raw.Name|escapejs }}", "matchingMode": "{{ raw.MatchingMode|escapejs }}", "winnerRule": [{% for criterion in winnerRule %}"{{ criterion.token|escapejs }}"{% if not forloop.Last %}, {% endif %}{% endfor %}], "keepAsVersionNear": {% if raw.KeepAsVersionNear %}true{% else %}false{% endif %}, "keepAsVersionIdentical": {% if raw.KeepAsVersionIdentical %}true{% else %}false{% endif %}}}</script>

    <div data-reduction-live="head">
        <section class="reduction-status" aria-labelledby="reduction-status-heading" data-testid="reduction-status-header" data-reduction-status="{{ reduction.StatusEffective }}">
            <h2 id="reduction-status-heading" class="sr-only">Status</h2>
            <span class="reduction-pill reduction-pill--{{ reduction.StatusEffective }}" data-testid="reduction-status">{{ reduction.StatusLabel }}</span>
            {% if raw.ComputedAt %}
            <span class="text-sm text-stone-600" data-testid="reduction-computed-at">Last computed {{ reduction.ComputedAtLabel }}</span>
            {% endif %}
            <span class="reduction-status-actions">
                {% if reduction.StatusEffective == "draft" %}
                <button type="button" class="reduction-btn reduction-btn--primary" data-testid="reduction-compute"
                        :disabled="$store.reductionReview.busy"
                        @click="$store.reductionReview.compute()">Find duplicates</button>
                {% elif reduction.StatusEffective == "ready" %}
                <button type="button" class="reduction-btn" data-testid="reduction-compute"
                        :disabled="$store.reductionReview.busy || $store.reductionReview.applying"
                        @click="$store.reductionReview.compute()">Recompute</button>
                {% elif reduction.StatusEffective == "failed" %}
                <button type="button" class="reduction-btn reduction-btn--primary" data-testid="reduction-compute"
                        :disabled="$store.reductionReview.busy"
                        @click="$store.reductionReview.compute()">Try again</button>
                {% endif %}
                {% if showReview %}
                <button type="button" class="reduction-btn reduction-btn--link reduction-shortcuts-btn" data-testid="reduction-shortcuts-button"
                        aria-haspopup="dialog" @click="$store.reductionReview.openDialog('shortcuts', $event)">Keyboard shortcuts</button>
                {% endif %}
                <a href="#reduction-settings" class="reduction-btn reduction-btn--link" data-testid="reduction-settings-link"
                   @click.prevent="$store.reductionReview.focusSettings()">Settings</a>
            </span>
        </section>

        {% if reduction.StatusEffective == "draft" %}
        <p class="text-sm text-stone-700 mt-3 max-w-prose" data-testid="reduction-intro">
            Finds copies of the same file and near-identical images in this selection. Nothing changes until you review and apply.
        </p>
        {% endif %}

        {% if reduction.StatusEffective == "computing" %}
        <div class="reduction-progress mt-3" data-testid="reduction-computing" data-reduction-job="{{ raw.ComputeJobID }}">
            <p class="text-sm text-stone-800" data-testid="reduction-progress-text"
               x-text="$store.reductionReview.progress.text">Computing…</p>
            <progress class="reduction-progress-bar" aria-label="Progress of finding duplicates"
                      :value="$store.reductionReview.progress.total > 0 ? $store.reductionReview.progress.count : null"
                      :max="$store.reductionReview.progress.total > 0 ? $store.reductionReview.progress.total : null"></progress>
            <p class="text-xs text-stone-600 mt-1">You can leave this page. The server keeps going and the result is here when you come back.</p>
        </div>
        {% endif %}

        {% if reduction.StatusEffective == "failed" %}
        <p class="mt-3 text-sm text-red-800" role="alert" data-testid="reduction-compute-error">
            Finding duplicates failed{% if raw.ComputeError %}: {{ raw.ComputeError }}{% else %}.{% endif %}
        </p>
        {% endif %}

    </div>

    {# Not re-rendered after an action: what the store says (a start failure, the apply report, an error) and the apply record. #}
    <div class="reduction-static-notices">
        {# The compute-start failure the create flow stored before navigating here (key reduction-compute-error-<id>). #}
        <div class="reduction-notice reduction-notice--error" role="alert" x-show="$store.reductionReview.startError" x-cloak data-testid="reduction-start-error">
            <p>The Reduction was saved, but finding duplicates could not start<span x-text="$store.reductionReview.startError === true ? '' : ': ' + $store.reductionReview.startError"></span>.</p>
            <button type="button" class="reduction-btn" :disabled="$store.reductionReview.busy" @click="$store.reductionReview.compute()">Try again</button>
        </div>

        {# The last apply, from the plan, so a closed tab or a dropped connection is answered on reload. #}
        {% if lastApply %}
        <div class="reduction-notice{% if lastApply.interrupted %} reduction-notice--warn{% elif lastApply.running %} reduction-notice--info{% endif %}"
             role="status" data-testid="reduction-last-apply"
             x-show="!$store.reductionReview.applyCounts">
            {% if lastApply.interrupted %}
            <p>The last apply was interrupted. Applied Clusters are on the Applied tab; the rest are still approved.</p>
            {% elif lastApply.running %}
            <p>An apply started at {{ lastApply.startedAt|slice:"11:16" }} is running. Reload to see the result.</p>
            {% elif lastApply.countsHidden %}
            <p>The last apply finished, {{ lastApply.startedAt|slice:"0:10" }} {{ lastApply.startedAt|slice:"11:16" }}.</p>
            {% else %}
            <p>
                Last apply, {{ lastApply.startedAt|slice:"0:10" }} {{ lastApply.startedAt|slice:"11:16" }}:
                applied {{ lastApply.applied }} Cluster{% if lastApply.applied != 1 %}s{% endif %}
                and removed {{ lastApply.removed }} cop{% if lastApply.removed != 1 %}ies{% else %}y{% endif %}.
                {% if lastApply.stale %}{{ lastApply.stale }} could not be applied; they are on the Stale tab.{% endif %}
            </p>
            {% endif %}
        </div>
        {% endif %}

        {# The apply is running and the reviewer closed its dialog. #}
        <div class="reduction-notice reduction-notice--info" role="status" data-testid="reduction-apply-background-notice"
             x-show="$store.reductionReview.localApplying && !$store.reductionReview.dialog" x-cloak>
            <p>Applying <span x-text="$store.reductionReview.applyingCount"></span> Cluster<span x-text="$store.reductionReview.applyingCount === 1 ? '' : 's'"></span> in the background. This page updates when it finishes.</p>
        </div>

        {# An apply whose response never arrived: said plainly while the plan is read for the answer. #}
        <div class="reduction-notice reduction-notice--info" role="status" x-show="$store.reductionReview.applyUnknown" x-cloak
             x-text="$store.reductionReview.applyUnknown" data-testid="reduction-apply-unknown"></div>

        {# The report of the apply this page just ran (or recovered): what merged, what was refused and why. #}
        <div class="reduction-notice reduction-notice--ok" id="reduction-apply-result" tabindex="-1" role="status"
             x-show="$store.reductionReview.applyCounts" x-cloak data-testid="reduction-apply-result">
            <p class="font-medium text-stone-900" x-text="$store.reductionReview.applySummary()"></p>
            <p class="text-sm">This does not free disk space.</p>
            <template x-if="$store.reductionReview.applyCounts?.stale && !$store.reductionReview.applyResult?.stale?.length">
                <p class="mt-2 text-sm text-amber-900">
                    <span x-text="$store.reductionReview.applyCounts.stale + ($store.reductionReview.applyCounts.stale === 1 ? ' Cluster' : ' Clusters')"></span>
                    could not be applied. <a class="underline" :href="'/reduction?id={{ raw.ID }}&status=stale'">See the Stale tab</a>.
                </p>
            </template>
            <template x-if="$store.reductionReview.applyResult?.stale?.length">
                <div class="mt-2">
                    <p class="text-sm text-amber-900">
                        <span x-text="$store.reductionReview.applyResult.stale.length + ($store.reductionReview.applyResult.stale.length === 1 ? ' Cluster' : ' Clusters')"></span> could not be applied.
                        <a class="underline" :href="'/reduction?id={{ raw.ID }}&status=stale'">See the Stale tab</a>.
                    </p>
                    <ul class="list-disc list-inside text-sm text-stone-800">
                        <template x-for="(outcome, i) in $store.reductionReview.applyResult.stale" :key="outcome.clusterId || ('withheld-' + i)">
                            <li>
                                {# A withheld outcome names nothing, because its id is derived from the member ids and would recover them. #}
                                <template x-if="outcome.withheld"><span x-text="outcome.reason"></span></template>
                                <template x-if="!outcome.withheld">
                                    <span>
                                        <a :href="'/reduction?id={{ raw.ID }}&status=stale'" class="underline" x-text="outcome.label ? 'Cluster keeping ' + outcome.label : 'A Cluster'"></a>
                                        &mdash; <span x-text="outcome.reason"></span>
                                    </span>
                                </template>
                            </li>
                        </template>
                    </ul>
                </div>
            </template>
        </div>

        {# The apply bar carries its own copy of the error while it is on screen; this one is for tabs without a bar. #}
        <div class="reduction-notice reduction-notice--error" :role="$store.reductionReview.barShown ? null : 'alert'" tabindex="-1" data-reduction-error
             x-show="$store.reductionReview.error && !$store.reductionReview.barShown" x-cloak
             x-text="$store.reductionReview.errorText" data-testid="reduction-error"></div>
    </div>

    <div data-reduction-live="body">
        {% if showReview %}
            {% if extent.enteredSince %}
            <div class="reduction-notice reduction-notice--warn" key="reduction-drift" data-testid="reduction-drift">
                <p>{{ extent.enteredSince }} Resource{% if extent.enteredSince != 1 %}s{% endif %} added since the last run. Recompute to include {% if extent.enteredSince != 1 %}them{% else %}it{% endif %}.</p>
            </div>
            {% endif %}
            {% if settingsChanged and statusTab == "open" %}
            <div class="reduction-notice reduction-notice--warn" key="reduction-settings-changed" data-testid="reduction-settings-changed">
                <p>Settings changed since the last run. Recompute to apply them to Clusters you have not acted on.</p>
                <button type="button" class="reduction-btn" :disabled="$store.reductionReview.busy || $store.reductionReview.applying" @click="$store.reductionReview.compute()">Recompute</button>
            </div>
            {% endif %}
            {% if coverage.trusted and coverage.poor and raw.MatchingMode != "identical" %}
            <div class="reduction-notice reduction-notice--warn" key="reduction-coverage-warning" data-testid="reduction-coverage-warning">
                <p>{{ coverage.perceptualMissing }} resource{% if coverage.perceptualMissing != 1 %}s are{% else %} is{% endif %} missing a perceptual hash, so Near-Identical results may be incomplete.
                    <a class="underline" href="/admin/overview">Recompute similarity</a>.</p>
            </div>
            {% endif %}

            <nav aria-label="Cluster status" class="reduction-tabs" data-testid="reduction-tabs">
                {% for tab in reviewNav.tabs %}
                <a href="{{ tab.href }}" class="reduction-tab" data-testid="reduction-tab-{{ tab.key }}"{% if tab.current %} aria-current="page"{% endif %}>
                    {{ tab.label }} <span class="reduction-tab-count">({{ tab.count }})</span>
                </a>
                {% endfor %}
            </nav>

            <div class="reduction-filters" data-testid="reduction-filters">
                <ul class="reduction-chips" aria-label="Filters">
                    {% for chip in reviewNav.tiers %}
                    <li><a href="{{ chip.href }}" class="reduction-chip" data-testid="reduction-chip-tier-{{ chip.label|lower }}"{% if chip.active %} aria-current="true"{% endif %}>{{ chip.label }}{% if chip.active %}<span class="sr-only"> (filter on)</span>{% endif %}</a></li>
                    {% endfor %}
                    {% for chip in reviewNav.approvals %}
                    <li><a href="{{ chip.href }}" class="reduction-chip" data-testid="reduction-chip-approval-{{ chip.label|lower|cut:" " }}"{% if chip.active %} aria-current="true"{% endif %}>{{ chip.label }}{% if chip.active %}<span class="sr-only"> (filter on)</span>{% endif %}</a></li>
                    {% endfor %}
                    <li><a href="{{ reviewNav.attention.href }}" class="reduction-chip" data-testid="reduction-chip-attention"{% if reviewNav.attention.active %} aria-current="true"{% endif %}>{{ reviewNav.attention.label }}{% if reviewNav.attention.active %}<span class="sr-only"> (filter on)</span>{% endif %}</a></li>
                    {% if reviewFilterActive %}
                    <li><a href="{{ reviewNav.clearHref }}" class="reduction-chip reduction-chip--clear" data-testid="reduction-clear-filters">Clear filters</a></li>
                    {% endif %}
                </ul>
                {# A disclosure on a phone, plain controls from 640px up. `open` is the server's default (right with no script) and is closed on a narrow screen by the x-init; the morph keeps whatever the reviewer chose (data-keep-open). #}
                <details class="reduction-sortbox" open data-keep-open data-testid="reduction-sortbox"
                         x-data x-init="const narrow = window.matchMedia('(max-width: 639px)'); $el.open = !narrow.matches; narrow.addEventListener('change', () => { $el.open = !narrow.matches; })">
                    <summary class="reduction-sortbox-summary">Sort and page size</summary>
                    <form method="get" action="/reduction" class="reduction-sort" aria-label="Sort and page size">
                        {% for field in reviewNav.hidden %}<input type="hidden" name="{{ field.name }}" value="{{ field.value }}">{% endfor %}
                        <label class="text-sm text-stone-700">Sort
                            <select name="sort" data-testid="reduction-sort" class="reduction-select">
                                <option value="copies"{% if reviewSort == "copies" %} selected{% endif %}>Most copies first</option>
                                <option value="size"{% if reviewSort == "size" %} selected{% endif %}>Largest files first</option>
                                <option value="computed"{% if reviewSort == "computed" %} selected{% endif %}>Computed order</option>
                            </select>
                        </label>
                        <label class="text-sm text-stone-700">Per page
                            <select name="pageSize" data-testid="reduction-page-size" class="reduction-select">
                                {% for size in pageSizes %}<option value="{{ size }}"{% if size == pageSize %} selected{% endif %}>{{ size }}</option>{% endfor %}
                            </select>
                        </label>
                        {# Changing a select does not navigate on its own (WCAG 3.2.2); this does. #}
                        <button type="submit" class="reduction-btn reduction-btn--small" data-testid="reduction-sort-apply">Show</button>
                    </form>
                </details>
            </div>

            {# Keyboard shortcuts act only while focus is inside this region (WCAG 2.1.4). #}
            <section id="reduction-clusters" tabindex="-1" class="reduction-clusters-region{% if statusTab == "open" %} reduction-clusters-region--bar{% endif %}" aria-labelledby="reduction-clusters-heading"
                     data-testid="reduction-clusters-region"
                     @keydown="$store.reductionReview.onKey($event)">
                <h2 id="reduction-clusters-heading" class="reduction-clusters-title">
                    Clusters <span class="detail-panel-count" data-reduction-count="{{ review.ClusterCount }}">({{ review.ClusterCount }})</span>
                </h2>
                <div class="items-container" data-reduction-clusters data-testid="reduction-clusters">
                    {% for cluster in clusters %}
                        {% include "/partials/reductionCluster.tpl" %}
                    {% empty %}
                        <div class="detail-empty reduction-empty" data-testid="reduction-empty">
                            {% if reviewFilterActive %}
                                No Clusters match these filters.
                            {% elif statusTab == "open" and tabCounts.merging %}
                                {{ tabCounts.merging }} Cluster{% if tabCounts.merging != 1 %}s are{% else %} is{% endif %} being merged. If an apply was interrupted, finish it here; otherwise reload in a moment.
                                <button type="button" class="reduction-btn mt-2" data-testid="reduction-settle"
                                        :disabled="$store.reductionReview.busy || $store.reductionReview.applying"
                                        @click="$store.reductionReview.settleOnly()">Finish the interrupted apply</button>
                            {% elif tabCounts.open|add:tabCounts.skipped|add:tabCounts.stale|add:tabCounts.applied == 0 %}
                                No copies found in this selection. See About this Reduction for how much of it could be checked.
                            {% else %}
                                No {{ statusTab }} Clusters.
                            {% endif %}
                        </div>
                    {% endfor %}
                </div>
                {# No pagination include here: base.tpl already renders one for the page, and a second nav with the same label is a duplicate landmark. #}
            </section>
        {% endif %}
    </div>

    {% if showReview %}
    {# The apply bar. Sticky inside the main column so the cards scroll under it; the Clusters region has bottom padding of its own height. #}
    <div class="reduction-bar" data-testid="reduction-apply-bar" tabindex="-1" role="region" aria-label="Apply approved Clusters"
         x-show="$store.reductionReview.tab === 'open' && ($store.reductionReview.openCount > 0 || $store.reductionReview.checkedCount > 0)" x-cloak
         @keydown.escape="$store.reductionReview.closeMore()">
        <div class="reduction-bar-error" role="alert" tabindex="-1" data-reduction-error data-testid="reduction-bar-error"
             x-show="$store.reductionReview.error" x-cloak>
            <span x-text="$store.reductionReview.errorText"></span>
            <button type="button" class="reduction-btn reduction-btn--small" @click="window.location.reload()">Reload</button>
        </div>
        <p class="reduction-bar-counts" data-testid="reduction-checked-summary">
            <span x-text="$store.reductionReview.checkedCount">{{ review.CheckedCount }}</span> approved
            &middot;
            <span x-text="$store.reductionReview.checkedLoserCount">{{ review.CheckedLoserCount }}</span> cop<span x-text="$store.reductionReview.checkedLoserCount === 1 ? 'y' : 'ies'">{% if review.CheckedLoserCount != 1 %}ies{% else %}y{% endif %}</span> to remove
        </p>
        <p class="reduction-bar-notice" role="status" data-testid="reduction-bulk-notice" x-show="$store.reductionReview.bulkNotice" x-cloak>
            <span x-text="$store.reductionReview.bulkNotice?.text"></span>
            <a class="underline" x-show="$store.reductionReview.bulkNotice?.needsReview" :href="'{{ reviewNav.attentionHref }}'">Review them</a>
        </p>
        <div class="reduction-bar-actions">
            <button type="button" class="reduction-btn reduction-more" data-testid="reduction-more"
                    aria-haspopup="true" aria-controls="reduction-bulk-menu"
                    :aria-expanded="$store.reductionReview.moreOpen.toString()"
                    @click="$store.reductionReview.toggleMore()">More</button>
            <div id="reduction-bulk-menu" class="reduction-bulk" :class="{ 'is-open': $store.reductionReview.moreOpen }">
                {% if statusTab == "open" %}
                <button type="button" class="reduction-btn" data-testid="reduction-approve-page"
                        :disabled="$store.reductionReview.busy || $store.reductionReview.applying"
                        @click="$store.reductionReview.bulk('check', 'page')">Approve page</button>
                <button type="button" class="reduction-btn" data-testid="reduction-clear-page"
                        :disabled="$store.reductionReview.busy || $store.reductionReview.applying"
                        @click="$store.reductionReview.bulk('uncheck', 'page')">Clear page</button>
                {% endif %}
                <button type="button" class="reduction-btn" data-testid="reduction-approve-identical"
                        :disabled="$store.reductionReview.busy || $store.reductionReview.applying || $store.reductionReview.bulkFigures.identicalApprovable === 0"
                        @click="$store.reductionReview.bulk('check', 'identical')">Approve all Identical (<span x-text="$store.reductionReview.bulkFigures.identicalApprovable">{{ bulk.identicalApprovable }}</span>)</button>
            </div>
            <div class="reduction-apply-wrap">
                <button type="button" id="reduction-apply-button" class="reduction-btn reduction-btn--danger" data-testid="reduction-apply"
                        :disabled="$store.reductionReview.busy || $store.reductionReview.applying || $store.reductionReview.refreshing > 0 || ($store.reductionReview.checkedCount === 0 && !$store.reductionReview.figuresStale)"
                        aria-describedby="reduction-apply-warning"
                        @click="$store.reductionReview.apply($event)" x-text="'Apply ' + $store.reductionReview.checkedCount + ' Cluster' + ($store.reductionReview.checkedCount === 1 ? '' : 's') + '\u2026'">Apply {{ review.CheckedCount }} Cluster{% if review.CheckedCount != 1 %}s{% endif %}…</button>
                <span id="reduction-apply-warning" class="reduction-apply-warning">Applying cannot be undone.</span>
            </div>
        </div>
    </div>

    <details class="reduction-about" data-testid="reduction-about">
        <summary>About this Reduction</summary>
        <div class="reduction-about-body">
            <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 text-sm">
                <div>
                    <dt class="text-xs text-stone-600 font-mono">Selection</dt>
                    <dd class="text-stone-800" data-testid="reduction-extent">
                        {% if extentAll %}
                            Everything you can access.
                        {% else %}
                            {{ extent.resourceCount }} Resource{% if extent.resourceCount != 1 %}s{% endif %}
                            and {{ extent.groupCount }} Group{% if extent.groupCount != 1 %}s{% endif %} selected.
                            {% if extent.groupCount %}Resources from all subgroups are included at each compute.{% endif %}
                        {% endif %}
                        {% if raw.ComputedAt and coverage.trusted %}{{ extent.resolvedSize }} Resource{% if extent.resolvedSize != 1 %}s{% endif %} were checked at the last compute.{% endif %}
                    </dd>
                    {% if reviewSelection.groups %}
                    <dd class="mt-1 text-stone-800">Groups:
                        {% for g in reviewSelection.groups %}<a class="reduction-link" href="/group?id={{ g.id }}">{{ g.name }}</a>{% if not forloop.Last %}, {% endif %}{% endfor %}{% if reviewSelection.moreGroups %} and {{ reviewSelection.moreGroups }} more{% endif %}
                    </dd>
                    {% endif %}
                    {% if reviewSelection.resources %}
                    <dd class="mt-1 text-stone-800">Resources:
                        {% for r in reviewSelection.resources %}<a class="reduction-link" href="/resource?id={{ r.id }}">{{ r.name }}</a>{% if not forloop.Last %}, {% endif %}{% endfor %}{% if reviewSelection.moreResources %} and {{ reviewSelection.moreResources }} more{% endif %}
                    </dd>
                    {% endif %}
                </div>
                <div>
                    <dt class="text-xs text-stone-600 font-mono">Matching</dt>
                    <dd class="text-stone-800">{{ reduction.MatchingLabel }}</dd>
                    <dd class="text-stone-800 mt-1" data-testid="reduction-external-resources">
                        {% if raw.ExcludeExternalResources %}
                        Copies outside this selection are excluded from matching.
                        {% else %}
                        A better copy elsewhere in your library may be kept instead.
                        {% endif %}
                    </dd>
                </div>
                <div>
                    <dt class="text-xs text-stone-600 font-mono">Winner rule</dt>
                    <dd class="text-stone-800" data-testid="reduction-winner-rule">
                        The kept copy is chosen by:
                        {% for criterion in winnerRule %}{{ criterion.label }}{% if not forloop.Last %}, then {% endif %}{% endfor %}.
                    </dd>
                </div>
                <div>
                    <dt class="text-xs text-stone-600 font-mono">Coverage</dt>
                    <dd class="text-stone-800" data-testid="reduction-coverage">
                        {% if not coverage.trusted %}
                        Coverage is unavailable because you do not have access to some resources.
                        {% else %}
                        {{ coverage.contentHashed }} of {{ coverage.extentSize }} resources have a content hash{% if coverage.hashless %}; {{ coverage.hashless }} cannot be checked for identical content{% endif %}.
                        {{ coverage.perceptualHashed }} of {{ coverage.perceptualEligible }} eligible resources have a perceptual hash for Near-Identical matching.
                        {% endif %}
                    </dd>
                </div>
                <div>
                    <dt class="text-xs text-stone-600 font-mono">Created</dt>
                    <dd class="text-stone-800">{{ raw.CreatedAt|date:"2006-01-02 15:04" }}</dd>
                </div>
            </dl>
            <p class="text-sm text-stone-600 mt-3">
                Add resources or groups with <strong>Find duplicates</strong> on
                <a class="reduction-link" href="/resources">the resources list</a> or
                <a class="reduction-link" href="/groups">the groups list</a>.
            </p>
        </div>
    </details>
    {% else %}
    <details class="reduction-about" data-testid="reduction-about">
        <summary>About this Reduction</summary>
        <div class="reduction-about-body text-sm text-stone-800">
            <p data-testid="reduction-extent">
                {% if extentAll %}Everything you can access.{% else %}{{ extent.resourceCount }} Resource{% if extent.resourceCount != 1 %}s{% endif %}
                and {{ extent.groupCount }} Group{% if extent.groupCount != 1 %}s{% endif %} selected.{% endif %}
            </p>
            <p class="mt-1" data-testid="reduction-external-resources">
                {% if raw.ExcludeExternalResources %}Copies outside this selection are excluded from matching.{% else %}A better copy elsewhere in your library may be kept instead.{% endif %}
            </p>
            <p class="mt-1">Matching: {{ reduction.MatchingLabel }}. Created {{ raw.CreatedAt|date:"2006-01-02 15:04" }}.</p>
        </div>
    </details>
    {% endif %}

    {% include "/partials/reductionDialogs.tpl" %}
</div>
{% endblock %}

{% block sidebar %}
{# The sidebar keeps the two things that are not the review: the settings, and Delete. #}
{# Its own Alpine root, so it reads the shared store rather than this page's scope.     #}
<div x-data="reductionSettingsForm({{ winnerRule|json }})" id="reduction-settings" data-testid="reduction-settings" tabindex="-1" class="w-full">
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Settings" %}
        <form method="post" action="/v1/reduction/edit" class="space-y-3 w-full" data-testid="reduction-settings-form"
              @submit.prevent="save($event)">
            <input type="hidden" name="id" value="{{ raw.ID }}">
            {% include "/partials/form/textInput.tpl" with name='name' label='Name' value=raw.Name %}

            <fieldset>
                <legend class="block text-sm font-medium font-mono text-stone-700">Matching mode</legend>
                <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input type="radio" name="matchingMode" value="both" {% if raw.MatchingMode == "both" %}checked{% endif %}
                           class="reduction-control border-stone-300 text-amber-700 focus:ring-amber-600">
                    Identical and Near-Identical Resources
                </label>
                <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input type="radio" name="matchingMode" value="identical" {% if raw.MatchingMode == "identical" %}checked{% endif %}
                           class="reduction-control border-stone-300 text-amber-700 focus:ring-amber-600">
                    Identical Resources only
                </label>
                <p class="text-xs text-stone-600 mt-1">
                    Near-Identical matching supports JPEG, PNG, GIF and WebP. Other formats are checked for identical content only.
                </p>
            </fieldset>

            <div>
                <label class="block text-sm font-medium font-mono text-stone-700" for="reduction-preset">Which copy to keep</label>
                <select id="reduction-preset" data-testid="reduction-preset" x-model="preset" @change="applyPreset()"
                        class="reduction-select mt-1 block w-full">
                    <option value="best">Best quality</option>
                    <option value="smallest">Smallest file</option>
                    <option value="oldest">Oldest copy</option>
                    <option value="custom">Custom</option>
                </select>
                <p class="text-xs text-stone-600 mt-1" x-text="presetHint"></p>
            </div>

            <fieldset x-show="preset === 'custom'" x-cloak>
                <legend class="block text-sm font-medium font-mono text-stone-700">Custom order</legend>
                <p class="text-xs text-stone-600 mt-1">
                    Choose criteria in priority order. Leave unused fields empty.
                </p>
                {# Ordered selects rather than a drag-and-drop list: this has to be operable from the keyboard #}
                {# and announced by a screen reader. The server drops empties and duplicates.                  #}
                {% for slot in winnerRuleSlots %}
                <div class="mt-1">
                    <label class="block text-xs font-mono text-stone-700" for="winner-rule-{{ slot.position }}">{{ slot.label }}</label>
                    <select id="winner-rule-{{ slot.position }}" data-testid="winner-rule-slot" x-model="slots[{{ forloop.Counter0 }}]"
                            class="reduction-select mt-1 block w-full">
                        <option value="">None</option>
                        {% for criterion in winnerCriteria %}
                        <option value="{{ criterion.token }}">{{ criterion.label }}</option>
                        {% endfor %}
                    </select>
                </div>
                {% endfor %}
            </fieldset>

            <fieldset>
                <legend class="block text-sm font-medium font-mono text-stone-700">Add removed copies as versions of the kept copy</legend>
                {# Not partials/form/checkboxInput.tpl: that renders a 14px box, under the WCAG 2.2 SC 2.5.8 24px floor. #}
                <label for="keepAsVersionNear" class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input id="keepAsVersionNear" name="keepAsVersionNear" type="checkbox" value="1" x-ref="keepNear"
                           {% if raw.KeepAsVersionNear %}checked{% endif %}
                           class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    For Near-Identical Clusters
                </label>
                <label for="keepAsVersionIdentical" class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input id="keepAsVersionIdentical" name="keepAsVersionIdentical" type="checkbox" value="1" x-ref="keepIdentical"
                           {% if raw.KeepAsVersionIdentical %}checked{% endif %}
                           class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    For Identical Clusters
                </label>
                <p class="text-xs text-stone-600 mt-1">
                    Off: their files move to the deleted-files backup instead. They stay on disk either way. Versions a removed copy already had always move to the kept copy.
                </p>
            </fieldset>

            {# Visible as well as announced (the announcement is the live region; this is for everyone else). #}
            <p class="reduction-notice reduction-notice--warn" x-show="notice" x-cloak x-text="notice" data-testid="reduction-settings-notice"></p>
            <p class="text-sm text-red-800" role="alert" x-show="error" x-cloak x-text="error" data-testid="reduction-settings-error"></p>
            <p class="text-sm text-green-800" role="status" x-show="saved" x-cloak data-testid="reduction-settings-saved">Settings saved.</p>
            <div class="flex gap-2">
                <button type="submit" class="reduction-btn reduction-btn--primary" :disabled="saving" data-testid="reduction-settings-save">Save settings</button>
                <a href="/reductions" class="reduction-btn reduction-btn--link">Cancel</a>
            </div>
        </form>
    </div>

    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Delete" %}
        <p class="text-sm text-stone-600 mb-2">
            Deleting a reduction removes its saved review. Resources are kept.
        </p>
        {% include "/partials/form/deleteButton.tpl" with action="/v1/reduction/delete" id=raw.ID text="Delete Resource Reduction" confirmMessage="Delete this Resource Reduction and its saved review? Resources will be kept." %}
    </div>
</div>
{% endblock %}

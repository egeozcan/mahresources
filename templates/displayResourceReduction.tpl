{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data
     x-init="$store.reductionReview.seed({ reductionId: {{ raw.ID }}, version: {{ raw.Version }}, checkedCount: {{ review.CheckedCount }}, checkedLoserCount: {{ review.CheckedLoserCount }} })"
     data-testid="reduction-page"
     data-reduction-id="{{ raw.ID }}"
     data-reduction-version="{{ raw.Version }}"
     data-reduction-checked="{{ review.CheckedCount }}"
     data-reduction-checked-losers="{{ review.CheckedLoserCount }}">

    {# Stated plainly and on the page rather than only in a confirm dialog: there #}
    {# is no restore path in this system, so the review is the safety mechanism.  #}
    <div class="bg-yellow-100 border-l-4 border-yellow-500 text-yellow-700 p-4 mb-4" role="note">
        <p class="font-bold">Applying a reduction cannot be undone</p>
        <p class="text-sm mt-1">
            Applying merges each checked Cluster's Losers into its Winner and deletes them. Review each Cluster before checking it.
        </p>
    </div>

    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Details</h2>
        </div>
        <div class="detail-panel-body">
            <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-sm">
                <div>
                    <dt class="text-xs text-stone-500 font-mono">Status</dt>
                    <dd class="text-stone-800" data-testid="reduction-status">{{ reduction.StatusLabel }}</dd>
                </div>
                <div>
                    <dt class="text-xs text-stone-500 font-mono">Matching</dt>
                    <dd class="text-stone-800">{{ reduction.MatchingLabel }}</dd>
                </div>
                <div>
                    <dt class="text-xs text-stone-500 font-mono">Created</dt>
                    <dd class="text-stone-800">{{ raw.CreatedAt|date:"2006-01-02 15:04" }}</dd>
                </div>
                <div>
                    <dt class="text-xs text-stone-500 font-mono">Last computed</dt>
                    <dd class="text-stone-800" data-testid="reduction-computed-at">{{ reduction.ComputedAtLabel }}</dd>
                </div>
            </dl>
            {% if raw.ComputeError %}
            <p class="mt-2 text-sm text-red-700" data-testid="reduction-compute-error">{{ raw.ComputeError }}</p>
            {% endif %}
        </div>
    </div>

    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Extent</h2>
        </div>
        <div class="detail-panel-body">
            <p class="text-sm text-stone-700" data-testid="reduction-extent">
                {{ extent.resourceCount }} Resource{% if extent.resourceCount != 1 %}s{% endif %}
                and {{ extent.groupCount }} Group{% if extent.groupCount != 1 %}s{% endif %} selected.
                {% if raw.ComputedAt and coverage.trusted %}{{ extent.resolvedSize }} Resource{% if extent.resolvedSize != 1 %}s{% endif %} included at last compute.{% endif %}
                {% if extent.groupCount %}Resources from all subgroups are included at each compute.{% endif %}
            </p>
            {% if extent.enteredSince %}
            <p class="text-sm text-amber-800 mt-1" data-testid="reduction-drift">
                {{ extent.enteredSince }} Resource{% if extent.enteredSince != 1 %}s have{% else %} has{% endif %}
                been added to the Extent since the last compute. Recompute to include {% if extent.enteredSince != 1 %}them{% else %}it{% endif %}.
            </p>
            {% endif %}
            <p class="text-sm text-stone-700 mt-1" data-testid="reduction-external-resources">
                {% if raw.ExcludeExternalResources %}
                External resources are excluded from matching.
                {% else %}
                Resources outside the Extent can be Winners.
                {% endif %}
            </p>
            <p class="text-sm text-stone-500 mt-1">
                Add resources or groups using the bulk actions on
                <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/resources">the resources list</a>
                or
                <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/groups">the groups list</a>.
            </p>
        </div>
    </div>

    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Coverage</h2>
        </div>
        <div class="detail-panel-body">
            {% if not coverage.trusted %}
            {# These figures were measured under whatever access the reviewer who       #}
            {# computed the plan had. Printing them to somebody who can now reach less  #}
            {# of the selection states how much is out there, which is what every other #}
            {# surface here refuses to state.                                           #}
            <p class="text-sm text-stone-600" data-testid="reduction-coverage">
                Coverage is unavailable because you do not have access to some resources.
            </p>
            {% else %}
            <p class="text-sm text-stone-700" data-testid="reduction-coverage">
                {{ coverage.contentHashed }} of {{ coverage.extentSize }} resources have a content hash{% if coverage.hashless %}; {{ coverage.hashless }} cannot be checked for identical content{% endif %}.
                {{ coverage.perceptualHashed }} of {{ coverage.perceptualEligible }} eligible resources have a perceptual hash for Near-Identical matching.
            </p>
            {% if coverage.poor %}
            <p class="text-sm text-amber-800 mt-1" data-testid="reduction-coverage-warning">
                {{ coverage.perceptualMissing }} resource{% if coverage.perceptualMissing != 1 %}s are{% else %} is{% endif %} missing a perceptual hash. Near-Identical results may be incomplete.
                <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/admin/overview">Recompute similarity</a>.
            </p>
            {% endif %}
            {% endif %}
        </div>
    </div>

    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">
                Clusters <span class="detail-panel-count" data-reduction-count="{{ review.ClusterCount }}">({{ review.ClusterCount }})</span>
            </h2>
        </div>
        <div class="detail-panel-body">
            <p class="text-sm text-red-700 mb-2" x-show="$store.reductionReview.error" x-cloak x-text="$store.reductionReview.error" role="alert" data-testid="reduction-error"></p>
            <div class="items-container" data-reduction-clusters data-testid="reduction-clusters">
                {% for cluster in clusters %}
                    {% include "/partials/reductionCluster.tpl" %}
                {% empty %}
                    <div class="detail-empty">
                        {% if reviewFilterActive %}
                            No Clusters match these filters.
                        {% elif raw.ComputedAt %}
                            No matching resources found. See Coverage for resources that could not be checked.
                        {% else %}
                            Not computed yet. Click Compute Clusters to find matching resources.
                        {% endif %}
                    </div>
                {% endfor %}
            </div>
            {# No pagination include here: base.tpl already renders one for the page,   #}
            {# and a second nav with the same label is a duplicate landmark.            #}
        </div>
    </div>

    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Winner Rule</h2>
        </div>
        <div class="detail-panel-body">
            <p class="text-sm text-stone-600 mb-2">
                Criteria are applied in order. If resources tie, the next criterion is used.
            </p>
            <ol class="list-decimal list-inside text-sm text-stone-800" data-testid="reduction-winner-rule">
                {% for criterion in winnerRule %}
                    <li>{{ criterion.label }}</li>
                {% endfor %}
            </ol>
        </div>
    </div>
</div>
{% endblock %}

{% block sidebar %}
<div x-data>
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter Clusters" action="/reduction" method="get">
        <input type="hidden" name="id" value="{{ raw.ID }}">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            <fieldset class="w-full mt-2">
                <legend class="block text-sm font-medium font-mono text-stone-700">Status</legend>
                {% for status in clusterStatusOptions %}
                    {% if status.Link %}
                    <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                        <input type="checkbox" name="Status" value="{{ status.Link }}" {% if status.Active %}checked{% endif %}
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        {{ status.Title }}
                    </label>
                    {% endif %}
                {% endfor %}
            </fieldset>

            <fieldset class="w-full mt-3">
                <legend class="block text-sm font-medium font-mono text-stone-700">Tier</legend>
                {% for tier in clusterTierOptions %}
                    {% if tier.Link %}
                    <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                        <input type="checkbox" name="Tier" value="{{ tier.Link }}" {% if tier.Active %}checked{% endif %}
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        {{ tier.Title }}
                    </label>
                    {% endif %}
                {% endfor %}
            </fieldset>

            <label class="flex items-center gap-2 mt-3 text-sm text-stone-700">
                <input type="checkbox" name="Attention" value="1" {% if attentionActive %}checked{% endif %}
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Needs attention
            </label>
            <p class="text-xs text-stone-500 mt-1">
                Clusters with potential data loss or unusually many resources.
            </p>

            {% include "/partials/form/searchButton.tpl" %}
        </div>
    </form>

    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Apply" %}
        <button type="button"
                data-testid="reduction-apply"
                :disabled="$store.reductionReview.busy"
                @click="$store.reductionReview.apply()"
                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600 disabled:opacity-50 disabled:cursor-not-allowed">
            Apply checked Clusters
        </button>
        <p class="text-xs text-stone-600 mt-2" data-testid="reduction-checked-summary"
           x-text="$store.reductionReview.checkedCount + ' Cluster' + ($store.reductionReview.checkedCount === 1 ? '' : 's') + ' checked across all pages, ' + $store.reductionReview.checkedLoserCount + ' Resource' + ($store.reductionReview.checkedLoserCount === 1 ? '' : 's') + ' to delete.'">
            {{ review.CheckedCount }} Cluster{% if review.CheckedCount != 1 %}s{% endif %} checked across all pages,
            {{ review.CheckedLoserCount }} Resource{% if review.CheckedLoserCount != 1 %}s{% endif %} to delete.
        </p>

        {# The report of the last apply. A refused Cluster is named here as well as #}
        {# marked on the row, because "one of your four hundred went stale" is not   #}
        {# something anybody can act on.                                             #}
        <div class="mt-4" x-show="$store.reductionReview.applyResult" x-cloak data-testid="reduction-apply-result" role="status">
            <p class="text-sm text-stone-800">
                <span x-text="$store.reductionReview.applyResult?.applied?.length || 0"></span> Cluster(s) applied,
                <span x-text="$store.reductionReview.applyResult?.destroyed || 0"></span> Resource(s) deleted.
            </p>
            <template x-if="$store.reductionReview.applyResult?.stale?.length">
                <div class="mt-2">
                    <p class="text-sm text-amber-800">
                        <span x-text="$store.reductionReview.applyResult.stale.length"></span> Cluster(s) could not be applied:
                    </p>
                    <ul class="list-disc list-inside text-sm text-stone-700">
                        <template x-for="(outcome, i) in $store.reductionReview.applyResult.stale" :key="outcome.clusterId || ('withheld-' + i)">
                            <li>
                                {# A withheld outcome names nothing, because its id is derived  #}
                                {# from the member ids and would recover them.                  #}
                                <template x-if="outcome.withheld">
                                    <span x-text="outcome.reason"></span>
                                </template>
                                <template x-if="!outcome.withheld">
                                    <span>
                                        <a :href="'#cluster-' + outcome.clusterId + '-heading'"
                                           class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700"
                                           x-text="outcome.clusterId"></a>
                                        &mdash; <span x-text="outcome.reason"></span>
                                    </span>
                                </template>
                            </li>
                        </template>
                    </ul>
                </div>
            </template>
        </div>
    </div>

    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Compute" %}
        {# Bound, not server-rendered: an in-page review action moves the version,  #}
        {# and this form is a native post that never re-renders. Left static it      #}
        {# would send a version the row passed some clicks ago and be refused —      #}
        {# correctly, but for a conflict the reviewer did not cause and cannot see.  #}
        <form method="post" action="/v1/reduction/compute">
            <input type="hidden" name="id" value="{{ raw.ID }}">
            <input type="hidden" name="version" :value="$store.reductionReview.version" value="{{ raw.Version }}">
            <button type="submit"
                    data-testid="reduction-compute"
                    {% if reduction.StatusEffective == "computing" %}disabled{% endif %}
                    class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
                {% if raw.ComputedAt %}Recompute{% else %}Compute Clusters{% endif %}
            </button>
        </form>
        {% if reduction.StatusEffective == "computing" %}
        <p class="text-sm text-stone-600 mt-1" data-testid="reduction-computing">
            Computing Clusters. View progress in the jobs panel.
        </p>
        {% endif %}
    </div>

    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Settings" %}
        <form method="post" action="/v1/reduction/edit" class="space-y-3" data-testid="reduction-settings-form">
            <input type="hidden" name="id" value="{{ raw.ID }}">
            {# Same reason as the recompute form above. #}
            <input type="hidden" name="version" :value="$store.reductionReview.version" value="{{ raw.Version }}">
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
                <p class="text-xs text-stone-500 mt-1">
                    Near-Identical matching supports JPEG, PNG, GIF and WebP. Other formats are checked for identical content only.
                </p>
            </fieldset>

            <fieldset>
                <legend class="block text-sm font-medium font-mono text-stone-700">Winner Rule</legend>
                <p class="text-xs text-stone-500 mt-1">
                    Choose criteria in priority order. Leave unused fields empty.
                </p>
                {# Ordered selects rather than a drag-and-drop list: this has to be   #}
                {# operable from the keyboard and announced by a screen reader, and a #}
                {# reorderable list is neither without a great deal of machinery. The #}
                {# server drops empties and duplicates, so "1st and 3rd only" and     #}
                {# "the same criterion twice" both resolve to what the reviewer meant.#}
                {% for slot in winnerRuleSlots %}
                <div class="mt-1">
                    <label class="block text-xs font-mono text-stone-600" for="winner-rule-{{ slot.position }}">{{ slot.label }}</label>
                    <select id="winner-rule-{{ slot.position }}" name="winnerRule"
                            data-testid="winner-rule-slot"
                            class="reduction-control mt-1 block w-full rounded-md border-stone-300 shadow-sm text-sm focus:border-amber-600 focus:ring-amber-600">
                        <option value="">None</option>
                        {% for criterion in winnerCriteria %}
                        <option value="{{ criterion.token }}" {% if criterion.token == slot.selected %}selected{% endif %}>{{ criterion.label }}</option>
                        {% endfor %}
                    </select>
                </div>
                {% endfor %}
            </fieldset>

            <fieldset>
                <legend class="block text-sm font-medium font-mono text-stone-700">Keep Loser files as versions</legend>
                {# Not partials/form/checkboxInput.tpl: that renders a 14px box, which is #}
                {# under the WCAG 2.2 SC 2.5.8 24px floor. These two govern whether a     #}
                {# Loser's pixels are recoverable, so they are not controls to make       #}
                {# fiddly.                                                                #}
                {# The hidden 0 in front of each box is what makes unchecking work.  #}
                {# A browser omits an unchecked checkbox entirely, which decodes to a #}
                {# nil *bool and therefore "leave it as it was" — so a flag that was  #}
                {# on could never be turned off. With both present the decoder takes  #}
                {# the last value, so checked submits 0 then 1 and unchecked submits  #}
                {# 0 alone. Verified against gorilla/schema, not assumed.             #}
                <label for="keepAsVersionNear" class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input type="hidden" name="keepAsVersionNear" value="0">
                    <input id="keepAsVersionNear" name="keepAsVersionNear" type="checkbox" value="1"
                           {% if raw.KeepAsVersionNear %}checked{% endif %}
                           class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    For Near-Identical Clusters
                </label>
                <label for="keepAsVersionIdentical" class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input type="hidden" name="keepAsVersionIdentical" value="0">
                    <input id="keepAsVersionIdentical" name="keepAsVersionIdentical" type="checkbox" value="1"
                           {% if raw.KeepAsVersionIdentical %}checked{% endif %}
                           class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    For Identical Clusters
                </label>
                <p class="text-xs text-stone-500 mt-1">
                    Existing versions always transfer to the Winner. Files retained by those versions still use disk space when these options are off.
                </p>
            </fieldset>

            {% include "/partials/form/createFormSubmit.tpl" with cancelUrl="/reductions" %}
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

{% extends "/layouts/base.tpl" %}

{% block body %}
<div data-testid="reduction-page" data-reduction-id="{{ raw.ID }}" data-reduction-version="{{ raw.Version }}"
     x-data="reductionReview({ reductionId: {{ raw.ID }}, version: {{ raw.Version }} })">

    {# Stated plainly and on the page rather than only in a confirm dialog: there #}
    {# is no restore path in this system, so the review is the safety mechanism.  #}
    <div class="bg-yellow-100 border-l-4 border-yellow-500 text-yellow-700 p-4 mb-4" role="note">
        <p class="font-bold">A Resource Reduction cannot be undone</p>
        <p>
            Applying a Cluster merges its Losers into its Winner and deletes them. There is no restore.
            Review each Cluster before you check it.
        </p>
    </div>

    <section class="mb-6" aria-labelledby="reduction-state-heading">
        <h2 id="reduction-state-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">State</h2>
        <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <div>
                <dt class="font-mono text-stone-500">Status</dt>
                <dd class="text-stone-800" data-testid="reduction-status">{{ reduction.StatusLabel }}</dd>
            </div>
            <div>
                <dt class="font-mono text-stone-500">Matching</dt>
                <dd class="text-stone-800">{{ reduction.MatchingLabel }}</dd>
            </div>
            <div>
                <dt class="font-mono text-stone-500">Created</dt>
                <dd class="text-stone-800">{{ raw.CreatedAt|date:"2006-01-02 15:04" }}</dd>
            </div>
            <div>
                <dt class="font-mono text-stone-500">Last computed</dt>
                <dd class="text-stone-800" data-testid="reduction-computed-at">{{ reduction.ComputedAtLabel }}</dd>
            </div>
        </dl>
        {% if raw.ComputeError %}
        <p class="mt-2 text-sm text-red-700" data-testid="reduction-compute-error">{{ raw.ComputeError }}</p>
        {% endif %}
    </section>

    <section class="mb-6" aria-labelledby="reduction-extent-heading">
        <h2 id="reduction-extent-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">Extent</h2>
        <p class="text-sm text-stone-600" data-testid="reduction-extent">
            {{ extent.resourceCount }} Resource{% if extent.resourceCount != 1 %}s{% endif %}
            and {{ extent.groupCount }} Group{% if extent.groupCount != 1 %}s{% endif %} selected,
            reaching {{ extent.resolvedSize }} Resource{% if extent.resolvedSize != 1 %}s{% endif %} right now.
            {% if extent.groupCount %}A Group's descendants are included, and are resolved each time the Reduction is computed.{% endif %}
        </p>
        {% if extent.enteredSince %}
        <p class="text-sm text-amber-800 mt-1" data-testid="reduction-drift">
            {{ extent.enteredSince }} Resource{% if extent.enteredSince != 1 %}s have{% else %} has{% endif %}
            entered the Extent since it was last computed. Recompute to include {% if extent.enteredSince != 1 %}them{% else %}it{% endif %}.
        </p>
        {% endif %}
        <p class="text-sm text-stone-500 mt-1">
            Add more from the bulk bar on <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/resources">the resources list</a>
            or <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/groups">the groups list</a>.
        </p>

        <form method="post" action="/v1/reduction/compute" class="mt-3 no-ajax">
            <input type="hidden" name="id" value="{{ raw.ID }}">
            <input type="hidden" name="version" value="{{ raw.Version }}">
            <button type="submit"
                    data-testid="reduction-compute"
                    {% if reduction.StatusEffective == "computing" %}disabled{% endif %}
                    class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
                {% if raw.ComputedAt %}Recompute{% else %}Compute Clusters{% endif %}
            </button>
        </form>
        {% if reduction.StatusEffective == "computing" %}
        <p class="text-sm text-stone-600 mt-1" data-testid="reduction-computing">
            Clustering is running. Its progress is in the jobs panel; this page shows the result when it lands.
        </p>
        {% endif %}
    </section>

    <section class="mb-6" aria-labelledby="reduction-coverage-heading">
        <h2 id="reduction-coverage-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">Coverage</h2>
        <p class="text-sm text-stone-600" data-testid="reduction-coverage">
            {{ coverage.contentHashed }} of {{ coverage.extentSize }} Resources in the Extent carry a content hash{% if coverage.hashless %}; {{ coverage.hashless }} do not and cannot be matched byte-identically{% endif %}.
            {{ coverage.perceptualHashed }} of {{ coverage.perceptualEligible }} Resources of a perceptually hashable type have a perceptual hash.
        </p>
        {% if coverage.poor %}
        <p class="text-sm text-amber-800 mt-1" data-testid="reduction-coverage-warning">
            {{ coverage.perceptualMissing }} eligible Resource{% if coverage.perceptualMissing != 1 %}s are{% else %} is{% endif %} unhashed or failed, so Near-Identical matching is incomplete.
            <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/admin/overview">Recompute similarity</a> to fix the cause.
        </p>
        {% endif %}
    </section>

    <section class="mb-6" aria-labelledby="reduction-clusters-heading">
        <h2 id="reduction-clusters-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">
            Clusters <span class="text-stone-500 font-normal">({{ review.ClusterCount }})</span>
        </h2>
        <p class="text-sm text-red-700 mb-2" x-show="error" x-cloak x-text="error" role="alert" data-testid="reduction-error"></p>
        <div class="items-container" data-reduction-clusters data-testid="reduction-clusters">
            {% for cluster in clusters %}
                {% include "/partials/reductionCluster.tpl" %}
            {% empty %}
                <div class="detail-empty">
                    {% if raw.ComputedAt %}
                        No Clusters. Nothing in this Extent repeats, as far as what could be examined &mdash; see Coverage above.
                    {% else %}
                        Not computed yet. Compute the Clusters to see what repeats.
                    {% endif %}
                </div>
            {% endfor %}
        </div>
        {# No pagination include here: base.tpl already renders one for the page,   #}
        {# and a second nav with the same label is a duplicate landmark.            #}

        {% if review.ClusterCount %}
        <div class="mt-4 flex flex-col gap-2">
            <button type="button"
                    data-testid="reduction-apply"
                    :disabled="busy"
                    @click="apply()"
                    class="self-start inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600 disabled:opacity-50 disabled:cursor-not-allowed">
                Apply the checked Clusters
            </button>
            <p class="text-xs text-stone-600">
                Only the Clusters checked on this page and every other page are applied. The rest stay open.
            </p>
        </div>

        {# The report of the last apply. A refused Cluster is named here as well as #}
        {# marked on the row, because "one of your four hundred went stale" is not   #}
        {# something anybody can act on.                                             #}
        <div class="mt-4" x-show="applyResult" x-cloak data-testid="reduction-apply-result" role="status">
            <p class="text-sm text-stone-800">
                <span x-text="applyResult?.applied?.length || 0"></span> Cluster(s) applied,
                <span x-text="applyResult?.destroyed || 0"></span> Resource(s) deleted.
            </p>
            <template x-if="applyResult?.stale?.length">
                <div class="mt-2">
                    <p class="text-sm text-amber-800">
                        <span x-text="applyResult.stale.length"></span> Cluster(s) were refused and kept in this Reduction:
                    </p>
                    <ul class="list-disc list-inside text-sm text-stone-700">
                        <template x-for="outcome in applyResult.stale" :key="outcome.clusterId">
                            <li>
                                <a :href="'#cluster-' + outcome.clusterId + '-heading'"
                                   class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700"
                                   x-text="outcome.clusterId"></a>
                                &mdash; <span x-text="outcome.reason"></span>
                            </li>
                        </template>
                    </ul>
                </div>
            </template>
        </div>
        {% endif %}
    </section>

    <section class="mb-6" aria-labelledby="reduction-rule-heading">
        <h2 id="reduction-rule-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">Winner Rule</h2>
        <p class="text-sm text-stone-600 mb-2">
            Each criterion breaks the ties the one before it left. A criterion that cannot tell a Cluster's
            members apart falls through to the next.
        </p>
        <ol class="list-decimal list-inside text-sm text-stone-800" data-testid="reduction-winner-rule">
            {% for criterion in winnerRule %}
                <li>{{ criterion.label }}</li>
            {% endfor %}
        </ol>
    </section>

    <section class="mb-6" aria-labelledby="reduction-settings-heading">
        <h2 id="reduction-settings-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">Settings</h2>
        <form method="post" action="/v1/reduction/edit" class="space-y-3" data-testid="reduction-settings-form">
            <input type="hidden" name="id" value="{{ raw.ID }}">
            <input type="hidden" name="version" value="{{ raw.Version }}">
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
                    Perceptual hashes exist for JPEG, PNG, GIF and WebP only. A library of video, PDFs or
                    audio can only ever be matched byte-identically, and the Identical-only sweep is far cheaper.
                </p>
            </fieldset>

            <fieldset>
                <legend class="block text-sm font-medium font-mono text-stone-700">Keep a Loser's file as a version of the Winner</legend>
                {# Not partials/form/checkboxInput.tpl: that renders a 14px box, which is #}
                {# under the WCAG 2.2 SC 2.5.8 24px floor. These two govern whether a     #}
                {# Loser's pixels are recoverable, so they are not controls to make       #}
                {# fiddly.                                                                #}
                <label for="keepAsVersionNear" class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input id="keepAsVersionNear" name="keepAsVersionNear" type="checkbox" value="1"
                           {% if raw.KeepAsVersionNear %}checked{% endif %}
                           class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    For Near-Identical Clusters
                </label>
                <label for="keepAsVersionIdentical" class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input id="keepAsVersionIdentical" name="keepAsVersionIdentical" type="checkbox" value="1"
                           {% if raw.KeepAsVersionIdentical %}checked{% endif %}
                           class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    For Identical Clusters
                </label>
                <p class="text-xs text-stone-500 mt-1">
                    Turning these off does not by itself reclaim disk space. A merge moves every Loser version onto
                    the Winner, and an upload always creates one, so the Loser's file stays referenced either way.
                </p>
            </fieldset>

            {% include "/partials/form/createFormSubmit.tpl" with cancelUrl="/reductions" %}
        </form>
    </section>

    <section aria-labelledby="reduction-danger-heading">
        <h2 id="reduction-danger-heading" class="text-lg font-mono font-medium text-stone-700 mb-2">Delete</h2>
        <p class="text-sm text-stone-600 mb-2">
            Deleting a Resource Reduction removes the workspace and its review. The Resources it named are untouched.
        </p>
        {% include "/partials/form/deleteButton.tpl" with action="/v1/reduction/delete" id=raw.ID text="Delete this Resource Reduction" confirmMessage="Delete this Resource Reduction? Its review is lost. The Resources it named are untouched." %}
    </section>
</div>
{% endblock %}

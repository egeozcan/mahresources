{% extends "/layouts/base.tpl" %}

{% block body %}
<div data-testid="reduction-page" data-reduction-id="{{ raw.ID }}" data-reduction-version="{{ raw.Version }}">

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
                <dd class="text-stone-800" data-testid="reduction-computed-at">
                    {% if raw.ComputedAt %}{{ raw.ComputedAt|date:"2006-01-02 15:04" }}{% else %}Never{% endif %}
                </dd>
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
            and {{ extent.groupCount }} Group{% if extent.groupCount != 1 %}s{% endif %} selected.
            {% if extent.groupCount %}A Group's descendants are included, and are resolved each time the Reduction is computed.{% endif %}
        </p>
        <p class="text-sm text-stone-500 mt-1">
            Add more from the bulk bar on <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/resources">the resources list</a>
            or <a class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" href="/groups">the groups list</a>.
        </p>
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
                           class="border-stone-300 text-amber-700 focus:ring-amber-600">
                    Identical and Near-Identical Resources
                </label>
                <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                    <input type="radio" name="matchingMode" value="identical" {% if raw.MatchingMode == "identical" %}checked{% endif %}
                           class="border-stone-300 text-amber-700 focus:ring-amber-600">
                    Identical Resources only
                </label>
                <p class="text-xs text-stone-500 mt-1">
                    Perceptual hashes exist for JPEG, PNG, GIF and WebP only. A library of video, PDFs or
                    audio can only ever be matched byte-identically, and the Identical-only sweep is far cheaper.
                </p>
            </fieldset>

            <fieldset>
                <legend class="block text-sm font-medium font-mono text-stone-700">Keep a Loser's file as a version of the Winner</legend>
                {% include "/partials/form/checkboxInput.tpl" with id='keepAsVersionNear' name='keepAsVersionNear' label='For Near-Identical Clusters' value=raw.KeepAsVersionNear %}
                {% include "/partials/form/checkboxInput.tpl" with id='keepAsVersionIdentical' name='keepAsVersionIdentical' label='For Identical Clusters' value=raw.KeepAsVersionIdentical %}
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

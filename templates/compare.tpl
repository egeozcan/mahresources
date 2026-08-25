{% extends "/layouts/base.tpl" %}

{% block body %}
{# See groupCompare.tpl: base.tpl already renders errorMessage, and the copy #}
{# that lived here said a second, different thing about the same failure. #}
{% if errorMessage %}
{% else %}
<div class="compare-page" x-data="compareView({
    r1: {{ query.Resource1ID|json }},
    v1: {{ query.Version1|default:0|json }},
    r2: {{ query.Resource2ID|json }},
    v2: {{ query.Version2|default:0|json }}
})">
    {# Names what is being compared and gives a route back to it; nothing else on #}
    {# the page does either. #}
    <nav class="compare-breadcrumb" aria-label="Breadcrumb">
        <a href="/resources">Resources</a>
        <span aria-hidden="true">/</span>
        <a href="/resource?id={{ resource1.ID }}">{{ name1 }}</a>
        {% if crossResource %}
        <span aria-hidden="true">/</span>
        <a href="/resource?id={{ resource2.ID }}">{{ name2 }}</a>
        {% endif %}
    </nav>

    <!-- Picker Toolbar -->
    {# Each side is one block so the two wrap as units. Wrapping them field by field #}
    {# breaks the left/right symmetry the control depends on. #}
    <div class="compare-toolbar">
        <div class="compare-toolbar-side">
            <span class="compare-side-label--old">{{ label1 }}</span>
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='resource' elName='r1' selectedItems=resource1Picker max=1 id='compare-left-resource' title='Left resource' placeholder='Search resources...' onChange='onResource1Selected' %}
            {# A resource whose history has not been migrated has no version rows to #}
            {# offer. An empty select is a control that cannot be used and does not #}
            {# say so. #}
            {% if versions1 %}
            <select x-model.number="v1" @change="updateUrl()" class="compare-version-select" aria-label="Left version">
                {% for v in versions1 %}
                <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version1 %}selected{% endif %}>{{ v.Label }}</option>
                {% endfor %}
            </select>
            {% else %}
            <select class="compare-version-select" aria-label="Left version" disabled>
                <option>No version history</option>
            </select>
            {% endif %}
        </div>

        <button type="button" class="compare-swap-btn" @click="swapSides()"
                aria-label="Swap the two sides and reload">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 16V4m0 0L3 8m4-4l4 4M17 8v12m0 0l4-4m-4 4l-4-4"/></svg>
        </button>

        <div class="compare-toolbar-side">
            <span class="compare-side-label--new">{{ label2 }}</span>
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='resource' elName='r2' selectedItems=resource2Picker max=1 id='compare-right-resource' title='Right resource' placeholder='Search resources...' onChange='onResource2Selected' %}
            {% if versions2 %}
            <select x-model.number="v2" @change="updateUrl()" class="compare-version-select" aria-label="Right version">
                {% for v in versions2 %}
                <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version2 %}selected{% endif %}>{{ v.Label }}</option>
                {% endfor %}
            </select>
            {% else %}
            <select class="compare-version-select" aria-label="Right version" disabled>
                <option>No version history</option>
            </select>
            {% endif %}
        </div>
    </div>

    {# The picker can put the same version on both sides, which otherwise renders a #}
    {# full "Files are identical" report of a file against itself. #}
    {% if comparison and not crossResource and query.Version1 == query.Version2 %}
    <p class="compare-notice" role="status">
        Both sides are showing v{{ query.Version1 }}, so there is nothing to compare.
        Pick a different version on one side.
    </p>
    {% endif %}

    {% if comparison %}
    <!-- Summary Banner -->
    <div class="compare-summary mb-4">
        {% if comparison.SameHash %}
        <span class="compare-verdict--identical">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6L9 17l-5-5"/></svg>
            Files are identical
        </span>
        {% else %}
        <span class="compare-verdict--different">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6L6 18M6 6l12 12"/></svg>
            Files differ
        </span>
        {% endif %}
        <span class="compare-stat">
            <span class="compare-stat-label">Type</span>
            {% if comparison.SameType %}Match{% else %}Changed{% endif %}
        </span>
        <span class="compare-stat">
            <span class="compare-stat-label">Size</span>
            {% if comparison.SizeDelta == 0 %}Match
            {% elif comparison.SizeDelta > 0 %}+{{ comparison.SizeDelta|humanReadableSize }}
            {% else %}{{ comparison.SizeDelta|humanReadableSize }}{% endif %}
        </span>
        {# A dimension of 0 means "this file has no dimensions", not a measurement: #}
        {# "0x0 -> 800x600" reports a resize that never happened. #}
        {% if comparison.DimensionsDiff and comparison.Version1.Width > 0 and comparison.Version2.Width > 0 %}
        <span class="compare-stat">
            <span class="compare-stat-label">Dimensions</span>
            {{ comparison.Version1.Width }}&times;{{ comparison.Version1.Height }} &rarr; {{ comparison.Version2.Width }}&times;{{ comparison.Version2.Height }}
        </span>
        {% endif %}
        {% if crossResource %}
        <span class="compare-stat compare-stat--flag">
            <span class="compare-stat-label">Cross-resource</span>
        </span>
        {% endif %}
    </div>

    <!-- Metadata -->
    <details open class="mb-6">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 mb-3 select-none font-mono">Metadata</summary>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            <!-- Content Type -->
            <div class="compare-meta-card{% if not comparison.SameType %} compare-meta-card--diff{% endif %}"{% if not comparison.SameType %} aria-label="Changed: Content Type"{% endif %}>
                <div class="compare-meta-card-label">Content Type</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SameType %}
                        {{ comparison.Version1.ContentType }}
                    {% else %}
                        {{ comparison.Version1.ContentType }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.ContentType }}
                    {% endif %}
                </div>
            </div>
            <!-- File Size -->
            <div class="compare-meta-card{% if comparison.SizeDelta != 0 %} compare-meta-card--diff{% endif %}"{% if comparison.SizeDelta != 0 %} aria-label="Changed: File Size"{% endif %}>
                <div class="compare-meta-card-label">File Size</div>
                <div class="compare-meta-card-value">
                    {% if comparison.SizeDelta == 0 %}
                        {{ comparison.Version1.FileSize|humanReadableSize }}
                    {% else %}
                        {{ comparison.Version1.FileSize|humanReadableSize }} <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.FileSize|humanReadableSize }}
                        <span class="text-xs text-amber-800">
                            ({% if comparison.SizeDelta > 0 %}+{% endif %}{{ comparison.SizeDelta|humanReadableSize }})
                        </span>
                    {% endif %}
                </div>
            </div>
            <!-- Dimensions (only for files that have them) -->
            {% if comparison.Version1.Width > 0 or comparison.Version2.Width > 0 %}
            <div class="compare-meta-card{% if comparison.DimensionsDiff %} compare-meta-card--diff{% endif %}"{% if comparison.DimensionsDiff %} aria-label="Changed: Dimensions"{% endif %}>
                <div class="compare-meta-card-label">Dimensions</div>
                <div class="compare-meta-card-value">
                    {% if comparison.DimensionsDiff %}
                        {% if comparison.Version1.Width > 0 %}{{ comparison.Version1.Width }}&times;{{ comparison.Version1.Height }}{% else %}&mdash;{% endif %}
                        <span class="text-stone-400" aria-hidden="true">&rarr;</span>
                        {% if comparison.Version2.Width > 0 %}{{ comparison.Version2.Width }}&times;{{ comparison.Version2.Height }}{% else %}&mdash;{% endif %}
                    {% else %}
                        {{ comparison.Version1.Width }}&times;{{ comparison.Version1.Height }}
                    {% endif %}
                </div>
            </div>
            {% endif %}
            <!-- Hash -->
            <div class="compare-meta-card{% if not comparison.SameHash %} compare-meta-card--diff{% endif %}"{% if not comparison.SameHash %} aria-label="Changed: Hash"{% endif %}>
                <div class="compare-meta-card-label">Hash</div>
                <div class="compare-meta-card-value">
                    {# Shown whole: a truncated hash cannot be checked against anything, #}
                    {# which is what the copy control is for. #}
                    {% if comparison.SameHash %}
                        <span class="text-amber-800 font-medium">Match</span>
                        {# The value is read out of the element, never written into the click #}
                        {# expression. A hash arrives from an imported manifest unvalidated, #}
                        {# and an attribute is HTML-decoded before Alpine parses what is left, #}
                        {# so escaping for HTML is not escaping for JavaScript. #}
                        <span class="compare-hash" x-data="{ copied: false }">
                            <code x-ref="hash">{{ comparison.Version1.Hash }}</code>
                            <button type="button" class="compare-copy-btn"
                                    @click="copied = await copyText($refs.hash.textContent.trim()); setTimeout(() => copied = false, 1600)"
                                    :aria-label="copied ? 'Hash copied' : 'Copy hash'">
                                <span x-text="copied ? 'Copied' : 'Copy'"></span>
                            </button>
                        </span>
                    {% else %}
                        <span class="text-red-800 font-medium">Different</span>
                        <span class="compare-hash">
                            <code>{{ comparison.Version1.Hash }}</code>
                            <span class="text-stone-400" aria-hidden="true">&rarr;</span>
                            <code>{{ comparison.Version2.Hash }}</code>
                        </span>
                    {% endif %}
                </div>
            </div>
            <!-- Created -->
            {# Compare the rendered strings, not the raw times: to-the-minute formatting #}
            {# otherwise prints the same value twice with an arrow between them. #}
            <div class="compare-meta-card">
                <div class="compare-meta-card-label">Created</div>
                <div class="compare-meta-card-value">
                    {{ comparison.Version1.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    {% if comparison.Version1.CreatedAt|date:"Jan 02, 2006 15:04" != comparison.Version2.CreatedAt|date:"Jan 02, 2006 15:04" %}
                        <span class="text-stone-400" aria-hidden="true">&rarr;</span> {{ comparison.Version2.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    {% elif comparison.Version1.CreatedAt != comparison.Version2.CreatedAt %}
                        <span class="text-xs text-stone-500">(less than a minute apart)</span>
                    {% endif %}
                </div>
            </div>
            <!-- Resource (cross-resource only) -->
            {% if crossResource %}
            <div class="compare-meta-card compare-meta-card--diff" aria-label="Changed: Resource">
                <div class="compare-meta-card-label">Resource</div>
                <div class="compare-meta-card-value">
                    <a href="/resource?id={{ resource1.ID }}" class="text-teal-800 hover:underline">{{ resource1.Name }}</a>
                    <span class="text-stone-400" aria-hidden="true">&rarr;</span>
                    <a href="/resource?id={{ resource2.ID }}" class="text-teal-800 hover:underline">{{ resource2.Name }}</a>
                </div>
            </div>
            {% endif %}
            <!-- Comment (only if either has one) -->
            {% if comparison.Version1.Comment or comparison.Version2.Comment %}
            <div class="compare-meta-card sm:col-span-2 lg:col-span-3{% if comparison.Version1.Comment != comparison.Version2.Comment %} compare-meta-card--diff{% endif %}"{% if comparison.Version1.Comment != comparison.Version2.Comment %} aria-label="Changed: Comment"{% endif %}>
                <div class="compare-meta-card-label">Comment</div>
                <div class="compare-meta-card-value italic text-stone-600">
                    {% if comparison.Version1.Comment == comparison.Version2.Comment %}
                        &ldquo;{{ comparison.Version1.Comment }}&rdquo;
                    {% else %}
                        &ldquo;{{ comparison.Version1.Comment }}&rdquo; <span class="text-stone-400 not-italic" aria-hidden="true">&rarr;</span> &ldquo;{{ comparison.Version2.Comment }}&rdquo;
                    {% endif %}
                </div>
            </div>
            {% endif %}
        </div>
    </details>

    <!-- Content Comparison Area -->
    {% if contentCategory == "image" %}
        {% include "/partials/compareImage.tpl" %}
    {% elif contentCategory == "text" %}
        {% include "/partials/compareText.tpl" %}
    {% elif contentCategory == "pdf" %}
        {% include "/partials/comparePdf.tpl" %}
    {% else %}
        {% include "/partials/compareBinary.tpl" %}
    {% endif %}

    {# Always rendered, disabled with the reason attached when merging is unavailable: #}
    {# a control that disappears when you pick an older version explains nothing. #}
    <details class="mt-6 bg-white shadow rounded-lg" x-data="{ keepAsVersion: false }">
        <summary class="cursor-pointer text-sm font-medium text-stone-600 p-4 select-none font-mono">Merge</summary>
        <div class="p-4 pt-0">
            {% if not canMerge %}
            <p class="compare-merge-blocked" id="compare-merge-blocked">{{ mergeBlockedReason }}</p>
            {% endif %}
            <div class="mb-4">
                <label class="flex items-center gap-2 text-sm text-stone-600 cursor-pointer">
                    <input type="checkbox" x-model="keepAsVersion" {% if not canMerge %}disabled{% endif %}
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Keep the other file as an earlier version
                </label>
            </div>
            {# Both the buttons and the confirmation name the resources. An irreversible #}
            {# merge phrased as "Left Wins" never says what it is about to destroy. The #}
            {# sentence arrives built, so a name reaches script as one JSON string. #}
            <div class="compare-merge-actions">
                <form
                    x-data="confirmAction({ message: {{ mergeConfirm1|json }} })"
                    action="/v1/resources/merge?redirect=%2Fresource%3Fid%3D{{ resource1.ID }}"
                    method="post"
                    x-bind="events"
                >
                    <input type="hidden" name="winner" value="{{ resource1.ID }}">
                    <input type="hidden" name="losers" value="{{ resource2.ID }}">
                    <input type="hidden" name="KeepAsVersion" :value="keepAsVersion">
                    <button type="submit" class="compare-merge-btn" {% if not canMerge %}disabled aria-describedby="compare-merge-blocked"{% endif %}>
                        &larr; Keep {{ label1 }}
                    </button>
                </form>
                <form
                    x-data="confirmAction({ message: {{ mergeConfirm2|json }} })"
                    action="/v1/resources/merge?redirect=%2Fresource%3Fid%3D{{ resource2.ID }}"
                    method="post"
                    x-bind="events"
                >
                    <input type="hidden" name="winner" value="{{ resource2.ID }}">
                    <input type="hidden" name="losers" value="{{ resource1.ID }}">
                    <input type="hidden" name="KeepAsVersion" :value="keepAsVersion">
                    <button type="submit" class="compare-merge-btn" {% if not canMerge %}disabled aria-describedby="compare-merge-blocked"{% endif %}>
                        Keep {{ label2 }} &rarr;
                    </button>
                </form>
            </div>
        </div>
    </details>

    {% else %}
    <!-- Empty State -->
    <div class="compare-empty-state">
        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <rect x="2" y="3" width="8" height="14" rx="1"/>
            <rect x="14" y="7" width="8" height="14" rx="1"/>
            <path d="M12 10l2-2m0 0l-2-2m2 2H8"/>
        </svg>
        {% if compareUnavailableReason %}
        <p class="text-lg font-medium text-stone-700">Nothing to compare</p>
        <p class="text-sm max-w-xs">{{ compareUnavailableReason }}</p>
        {% else %}
        <p class="text-lg font-medium text-stone-700">Ready to Compare</p>
        <p class="text-sm max-w-xs">Select resources and versions above to see a detailed comparison.</p>
        {% endif %}
        <div class="flex items-center gap-2 text-xs mt-1">
            <span class="compare-side-label--old">{{ label1 }}</span>
            <span class="text-stone-400" aria-hidden="true">&harr;</span>
            <span class="compare-side-label--new">{{ label2 }}</span>
        </div>
    </div>
    {% endif %}
</div>
{% endif %}
{% endblock %}

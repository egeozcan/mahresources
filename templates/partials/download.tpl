{# One download, as a card in the shared entity-list shape (.card, card-header,   #}
{# card-meta, card-badges), so /downloads reads like /notes and /queries rather    #}
{# than like a table of its own invention.                                         #}
{#                                                                                 #}
{# `download` is a downloadRow from download_template_context.go: a stored history #}
{# row, a live job, or a stored row a live job has moved past. Only a stored row   #}
{# has an ID, and the ID is what every action addresses — so a live-only row is    #}
{# rendered unselectable rather than registered with a missing id, which would put #}
{# `undefined` in the shared selection store's options map.                        #}
<article class="card download-card{% if download.ID %} card--selectable{% endif %}"
         data-testid="downloads-row"
         data-job-id="{{ download.JobID }}"
         {% if download.ID %}x-data="selectableItem({ itemId: {{ download.ID }} })"{% else %}x-data{% endif %}>
    {% if download.ID %}
    <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events"
           aria-label="Select {{ download.Name|default:download.URL }}"
           data-testid="downloads-row-checkbox"
           class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    {% endif %}

    <header class="card-header card-header--compact">
        <div class="card-title-section">
            <h2 class="card-title card-title--simple">{{ download.Name|default:download.URL }}</h2>
            <span class="card-url" title="{{ download.URL }}">{{ download.URL }}</span>
            <div class="card-meta">
                <span class="card-meta-item">
                    <span class="card-meta-label">Size:</span>
                    {% if download.TotalSize > 0 %}{{ download.TotalSize|filesizeformat }}{% elif download.Progress > 0 %}{{ download.Progress|filesizeformat }}{% else %}unknown{% endif %}
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">Started:</span>
                    <time datetime="{{ download.CreatedAt|date:"2006-01-02T15:04:05Z07:00" }}">{{ download.CreatedAt|date:"2006-01-02 15:04" }}</time>
                </span>
                {% if download.Attempts > 1 %}
                <span class="card-meta-item">{{ download.Attempts }} attempts</span>
                {% endif %}
            </div>
        </div>
    </header>

    <div class="card-badges">
        {% if download.Live %}
            <span class="card-badge card-badge--live" data-testid="downloads-status">{{ download.Status }}</span>
        {% elif download.Status == "failed" %}
            <span class="card-badge card-badge--danger" data-testid="downloads-status">failed</span>
        {% elif download.Status == "cancelled" %}
            <span class="card-badge card-badge--muted" data-testid="downloads-status">cancelled</span>
        {% else %}
            <span class="card-badge card-badge--success" data-testid="downloads-status">completed</span>
        {% endif %}
        {% if download.ResourceID %}
        <a href="/resource?id={{ download.ResourceID }}" class="card-badge card-badge--category" data-testid="downloads-resource-link">View resource</a>
        {% endif %}
        {% if download.LastRetryJobID and not download.Live %}
        {# The row records how this download ended; a resubmission is a new job #}
        {# with a new id, so without this a failed row that has already been     #}
        {# retried looks untouched and invites a second Retry.                   #}
        <span class="card-badge" data-testid="downloads-retry-link">retried as {{ download.LastRetryJobID }}</span>
        {% endif %}
    </div>

    {% if download.Error %}
    {# The shared component: it previews the first 30 characters, offers an  #}
    {# accessible "show more", and adds a copy button — which is what a       #}
    {# reader wants from a download error, since the useful thing to do with  #}
    {# one is paste it somewhere. It takes no length attribute; passing one   #}
    {# would look like configuration and do nothing.                          #}
    <expandable-text class="block mt-2 text-xs text-red-700" data-testid="downloads-error-text">{{ download.Error }}</expandable-text>
    {% endif %}

    {% if download.Retryable || (download.Deletable && download.ID) || download.Live %}
    {# Both buttons are inside the `download.ID` branch by construction: Retryable #}
    {# and Deletable are both false while a row is Live, and only a stored row is  #}
    {# ever not Live. They still read `$store.downloads`, which is why the article #}
    {# above always carries an x-data — an @click with no Alpine scope above it is #}
    {# silently inert.                                                             #}
    <div class="card-actions">
        {# aria-disabled rather than disabled, on all four of this page's action    #}
        {# buttons. `busy` flips the moment the confirm resolves, and disabling a    #}
        {# focused element blurs it — so the confirm dialog's focus return (a        #}
        {# macrotask, by design) landed on a button it could no longer focus and     #}
        {# dropped the reader on <body> for the length of the request. The store's   #}
        {# own `busy` guard is what actually refuses a second click; this is the     #}
        {# affordance for it, and it keeps the button focusable.                     #}
        {#                                                                           #}
        {# The name carries the download, because a page of cards otherwise offers   #}
        {# a reader listing buttons nothing but "Retry, Delete, Retry, Delete".      #}
        {% if download.Retryable %}
        <button type="button"
                @click="$store.downloads.retry([{{ download.ID }}])"
                :aria-disabled="$store.downloads.busy"
                aria-label="Retry {{ download.Name|default:download.URL }}"
                class="card-badge card-badge--action aria-disabled:opacity-50 aria-disabled:cursor-not-allowed"
                data-testid="downloads-retry">Retry</button>
        {% endif %}
        {% if download.Deletable && download.ID %}
        <button type="button"
                @click="$store.downloads.remove([{{ download.ID }}])"
                :aria-disabled="$store.downloads.busy"
                aria-label="Delete {{ download.Name|default:download.URL }}"
                class="card-badge card-badge--danger-action aria-disabled:opacity-50 aria-disabled:cursor-not-allowed"
                data-testid="downloads-delete">Delete</button>
        {% endif %}
        {% if download.Live %}
        <span class="card-meta-item text-xs text-stone-500" data-testid="downloads-live-note">in progress</span>
        {% endif %}
    </div>
    {% endif %}
</article>

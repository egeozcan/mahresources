{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="dashboard">
    {# Recent Resources #}
    <section class="dashboard-section" aria-label="Recent resources">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Resources</h2>
            <a href="/resources" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentResources %}
        <div class="dashboard-grid">
            {% for entity in recentResources %}
                {% include "partials/resource.tpl" %}
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No resources yet &mdash; <a href="/resource/new">upload your first file</a>.</p>
        {% endif %}
    </section>

    {# Recent Notes #}
    <section class="dashboard-section" aria-label="Recent notes">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Notes</h2>
            <a href="/notes" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentNotes %}
        <div class="dashboard-grid">
            {% for entity in recentNotes %}
                {% include "partials/note.tpl" %}
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No notes yet &mdash; <a href="/note/new">create your first note</a>.</p>
        {% endif %}
    </section>

    {# Recent Groups #}
    <section class="dashboard-section" aria-label="Recent groups">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Groups</h2>
            <a href="/groups" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentGroups %}
        <div class="dashboard-grid">
            {% for entity in recentGroups %}
                {% include "partials/group.tpl" %}
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No groups yet &mdash; <a href="/group/new">create your first group</a>.</p>
        {% endif %}
    </section>

    {# Recent Tags #}
    <section class="dashboard-section" aria-label="Recent tags">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Tags</h2>
            <a href="/tags" class="dashboard-view-all">View All &rarr;</a>
        </header>
        {% if recentTags %}
        <div class="dashboard-tags">
            {% for tag in recentTags %}
                <a href="/tag?id={{ tag.ID }}" class="dashboard-tag-pill">
                    {{ tag.Name }}
                </a>
            {% endfor %}
        </div>
        {% else %}
        <p class="dashboard-empty">No tags yet &mdash; <a href="/tag/new">create your first tag</a>.</p>
        {% endif %}
    </section>

    {# Activity Timeline #}
    <section class="dashboard-section" aria-label="Recent activity">
        <header class="dashboard-section-header">
            <h2 class="dashboard-section-title">Recent Activity</h2>
        </header>
        {% if activityFeed %}
        <ol class="dashboard-activity">
            {% for entry in activityFeed %}
            <li class="dashboard-activity-item">
                <span class="dashboard-activity-dot dashboard-activity-dot--{{ entry.EntityType }}" role="img" aria-label="{{ entry.EntityType }}"></span>
                <span class="dashboard-activity-type">{{ entry.EntityType }}</span>
                <a href="{{ entry.EntityType|entityPath }}?id={{ entry.EntityID }}" class="dashboard-activity-name">{{ entry.Name }}</a>
                <span class="dashboard-activity-action">{{ entry.Action }}</span>
                <time class="dashboard-activity-time" datetime="{{ entry.Timestamp|date:"2006-01-02T15:04:05Z" }}">
                    {{ entry.Timestamp|timeago }}
                </time>
            </li>
            {% endfor %}
        </ol>
        {% else %}
        <p class="dashboard-empty">No activity yet. Start by creating some content!</p>
        {% endif %}
    </section>
</div>
{% endblock %}

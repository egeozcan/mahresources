{% extends "/layouts/base.tpl" %}

{% block head %}
<style>
    /* Tree chart layout */
    .tree-chart {
        overflow-x: auto;
        padding: 1rem;
    }

    .tree-chart-list {
        display: flex;
        justify-content: center;
        gap: 0;
        padding-top: 1.5rem;
        position: relative;
        list-style: none;
        margin: 0;
        padding-left: 0;
    }

    .tree-chart-list li {
        display: flex;
        flex-direction: column;
        align-items: center;
        position: relative;
        padding: 1.5rem 0.5rem 0 0.5rem;
    }

    /* Vertical line from horizontal rail down to node */
    .tree-chart-list li::before {
        content: '';
        position: absolute;
        top: 0;
        left: 50%;
        width: 1px;
        height: 1.5rem;
        background: #cbd5e1;
    }

    /* Horizontal line connecting siblings */
    .tree-chart-list li::after {
        content: '';
        position: absolute;
        top: 0;
        left: 0;
        right: 0;
        height: 1px;
        background: #cbd5e1;
    }

    /* First child: hide left half of horizontal line */
    .tree-chart-list li:first-child::after {
        left: 50%;
    }

    /* Last child: hide right half of horizontal line */
    .tree-chart-list li:last-child::after {
        right: 50%;
    }

    /* Only child: hide horizontal line */
    .tree-chart-list li:only-child::after {
        display: none;
    }

    /* Root nodes: no connectors above */
    .tree-chart-list > .tree-root-node::before,
    .tree-chart-list > .tree-root-node::after {
        display: none;
    }
    .tree-chart-list > .tree-root-node {
        padding-top: 0;
    }

    /* Vertical line from parent box down to child rail */
    .tree-chart-list li > .tree-chart-list::before {
        content: '';
        position: absolute;
        top: -0.05rem;
        left: 50%;
        width: 1px;
        height: 1.5rem;
        background: #cbd5e1;
    }
    .tree-chart-list li > .tree-chart-list {
        position: relative;
    }

    /* Node box */
    .tree-node-box {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 0.25rem;
        padding: 0.5rem 0.75rem;
        background: white;
        border: 1px solid #e2e8f0;
        border-radius: 0.375rem;
        text-decoration: none;
        color: inherit;
        white-space: nowrap;
        max-width: 12rem;
        transition: box-shadow 0.15s, border-color 0.15s;
    }

    .tree-node-box:hover {
        border-color: #94a3b8;
        box-shadow: 0 1px 3px rgba(0,0,0,0.1);
    }

    .tree-node-box--path {
        border-color: #5eead4;
        background: #f0fdfa;
    }

    .tree-node-box--focused {
        border-color: #14b8a6;
        background: #ccfbf1;
        box-shadow: 0 0 0 3px rgba(20, 184, 166, 0.2);
    }

    .tree-node-name {
        font-size: 0.8125rem;
        font-weight: 500;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: 11rem;
    }

    .tree-node-category {
        font-size: 0.6875rem;
        background: #f1f5f9;
        color: #64748b;
        padding: 0.0625rem 0.375rem;
        border-radius: 9999px;
    }

    /* Expand/collapse button */
    .tree-node-expand {
        margin-top: 0.25rem;
        font-size: 0.6875rem;
        color: #64748b;
        background: #f8fafc;
        border: 1px solid #e2e8f0;
        border-radius: 0.25rem;
        padding: 0.125rem 0.5rem;
        cursor: pointer;
        display: inline-flex;
        align-items: center;
        gap: 0.25rem;
    }

    .tree-node-expand:hover {
        background: #f1f5f9;
        border-color: #94a3b8;
    }

    .tree-node-expand:disabled {
        cursor: wait;
        opacity: 0.6;
    }

    .tree-node-arrow {
        display: inline-block;
        width: 0;
        height: 0;
        border-left: 0.25rem solid transparent;
        border-right: 0.25rem solid transparent;
        border-top: 0.3rem solid #64748b;
        transition: transform 0.15s;
    }

    .tree-node-arrow--down {
        transform: rotate(180deg);
    }

    .tree-node-more {
        font-size: 0.75rem;
        color: #6366f1;
        text-decoration: none;
        padding: 0.25rem 0.5rem;
    }

    .tree-node-more:hover {
        text-decoration: underline;
    }

    /* Root list (no root selected) */
    .tree-roots-list {
        list-style: none;
        padding: 0;
        margin: 0;
    }

    .tree-roots-list li {
        border-bottom: 1px solid #f1f5f9;
    }

    .tree-roots-list a {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 0.75rem 1rem;
        text-decoration: none;
        color: inherit;
        transition: background 0.1s;
    }

    .tree-roots-list a:hover {
        background: #f8fafc;
    }
</style>
{% endblock %}

{% block body %}
    {% if rootId %}
    <div
        x-data="groupTree({
            initialRows: {{ treeRowsJSON }},
            highlightedPath: {{ highlightedPathJSON }},
            containingId: {{ containingId }},
            rootId: {{ rootId }}
        })"
        @click="handleClick($event)"
    >
        <div class="tree-chart" x-ref="treeContainer">
            <p class="text-stone-400 p-4">Loading tree...</p>
        </div>
    </div>
    {% else %}
    <div>
        {% if roots %}
        <p class="text-sm text-stone-500 mb-4">Select a root group to view its tree:</p>
        <ul class="tree-roots-list bg-white rounded-lg shadow overflow-hidden">
            {% for root in roots %}
            <li>
                <a href="/group/tree?root={{ root.ID }}">
                    <span>
                        <span class="font-medium">{{ root.Name }}</span>
                        {% if root.CategoryName %}
                        <span class="text-xs text-stone-400 ml-2">{{ root.CategoryName }}</span>
                        {% endif %}
                    </span>
                    {% if root.ChildCount > 0 %}
                    <span class="text-xs text-stone-400">{{ root.ChildCount }} {% if root.ChildCount == 1 %}child{% else %}children{% endif %}</span>
                    {% endif %}
                </a>
            </li>
            {% endfor %}
        </ul>
        {% else %}
        <p class="text-stone-500 p-4">No root groups found.</p>
        {% endif %}
    </div>
    {% endif %}
{% endblock %}

{% block sidebar %}
    {% if rootGroup %}
        <a href="/group?id={{ rootGroup.ID }}" class="block text-sm text-amber-700 hover:text-amber-900 mb-2">View Root Group</a>
    {% endif %}

    {% if containingId %}
        <div class="mb-4">
            {% include "/partials/sideTitle.tpl" with title="Highlighted Path" %}
            <p class="text-xs text-stone-500">
                Teal nodes show the path from root to the selected group.
            </p>
        </div>
    {% endif %}

    <div class="mb-4">
        {% include "/partials/sideTitle.tpl" with title="Navigation" %}
        <p class="text-xs text-stone-500 mb-2">Click a node to view it. Click the arrow button to expand/collapse children.</p>
        <a href="/groups" class="text-sm text-amber-700 hover:text-amber-900">Back to Groups List</a>
    </div>
{% endblock %}

<div class="compare-bucket-grid">
    <div class="compare-bucket">
        <div class="compare-panel-header--old">{{ label1 }} only ({{ diff.OnlyLeftCount }})</div>
        {% if diff.OnlyLeftCount > 0 %}
        <ul class="compare-item-list">
            {% for item in diff.OnlyLeft %}
            <li class="compare-item">
                {% if item.URL %}
                <a href="{{ item.URL }}" class="text-teal-700 hover:underline">{{ item.Label }}</a>
                {% else %}
                <span>{{ item.Label }}</span>
                {% endif %}
                {% if item.SecondaryText and item.SecondaryText != "None" %}
                <div class="compare-item-secondary">{{ item.SecondaryText }}</div>
                {% endif %}
            </li>
            {% endfor %}
        </ul>
        {% else %}
        <div class="compare-list-empty">None</div>
        {% endif %}
    </div>

    <div class="compare-bucket">
        <div class="compare-panel-header--shared">Shared ({{ diff.SharedCount }})</div>
        {% if diff.SharedCount > 0 %}
        <ul class="compare-item-list">
            {% for item in diff.Shared %}
            <li class="compare-item">
                {% if item.URL %}
                <a href="{{ item.URL }}" class="text-teal-700 hover:underline">{{ item.Label }}</a>
                {% else %}
                <span>{{ item.Label }}</span>
                {% endif %}
                {% if item.SecondaryText and item.SecondaryText != "None" %}
                <div class="compare-item-secondary">{{ item.SecondaryText }}</div>
                {% endif %}
            </li>
            {% endfor %}
        </ul>
        {% else %}
        <div class="compare-list-empty">No shared items</div>
        {% endif %}
    </div>

    <div class="compare-bucket">
        <div class="compare-panel-header--new">{{ label2 }} only ({{ diff.OnlyRightCount }})</div>
        {% if diff.OnlyRightCount > 0 %}
        <ul class="compare-item-list">
            {% for item in diff.OnlyRight %}
            <li class="compare-item">
                {% if item.URL %}
                <a href="{{ item.URL }}" class="text-teal-700 hover:underline">{{ item.Label }}</a>
                {% else %}
                <span>{{ item.Label }}</span>
                {% endif %}
                {% if item.SecondaryText and item.SecondaryText != "None" %}
                <div class="compare-item-secondary">{{ item.SecondaryText }}</div>
                {% endif %}
            </li>
            {% endfor %}
        </ul>
        {% else %}
        <div class="compare-list-empty">None</div>
        {% endif %}
    </div>
</div>

{# with block= shareToken= resourceHashMap= groupNameMap= #}
{% if block.Type == "text" %}
    <div class="prose prose-sm max-w-none">
        {{ block.Content.text|default:""|markdown2|safe }}
    </div>
{% elif block.Type == "heading" %}
    {% if block.Content.level == 1 %}
        <h2 class="text-xl font-bold text-gray-900">{{ block.Content.text }}</h2>
    {% elif block.Content.level == 2 %}
        <h3 class="text-lg font-semibold text-gray-900">{{ block.Content.text }}</h3>
    {% else %}
        <h4 class="text-base font-medium text-gray-900">{{ block.Content.text }}</h4>
    {% endif %}
{% elif block.Type == "divider" %}
    <hr class="border-gray-200">
{% elif block.Type == "todos" %}
    <div class="space-y-2" x-data="sharedTodos({{ block.ID }}, {{ block.State|json }}, '{{ shareToken }}')">
        {% for item in block.Content.items %}
        <label class="flex items-center gap-2 cursor-pointer">
            <input
                type="checkbox"
                :checked="isChecked('{{ item.id }}')"
                @change="toggleItem('{{ item.id }}')"
                class="w-4 h-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500"
            >
            <span :class="{ 'line-through text-gray-400': isChecked('{{ item.id }}') }">
                {{ item.label }}
            </span>
        </label>
        {% endfor %}
    </div>
{% elif block.Type == "gallery" %}
    <div class="grid grid-cols-2 md:grid-cols-3 gap-4 shared-gallery" data-gallery-id="{{ block.ID }}">
        {% for resourceId in block.Content.resourceIds %}
        <a href="/s/{{ shareToken }}/resource/{{ resourceHashMap|lookup:resourceId }}"
           class="block aspect-square bg-gray-100 rounded-lg overflow-hidden cursor-pointer hover:opacity-90 transition-opacity gallery-item">
            <img
                src="/s/{{ shareToken }}/resource/{{ resourceHashMap|lookup:resourceId }}"
                alt="Gallery image"
                class="w-full h-full object-cover"
                loading="lazy"
            >
        </a>
        {% endfor %}
    </div>
{% elif block.Type == "table" %}
    {# Check for query-based table data first, then fall back to static content #}
    {% if block.QueryData && block.QueryData.columns|length > 0 %}
    <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    {% for col in block.QueryData.columns %}
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        {{ col.label }}
                    </th>
                    {% endfor %}
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {% for row in block.QueryData.rows %}
                <tr>
                    {% for col in block.QueryData.columns %}
                    <td class="px-3 py-2 text-sm text-gray-900">
                        {{ row|lookup:col.id }}
                    </td>
                    {% endfor %}
                </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
    {% elif block.Content.columns && block.Content.columns|length > 0 %}
    <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    {% for col in block.Content.columns %}
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        {{ col.label }}
                    </th>
                    {% endfor %}
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {% for row in block.Content.rows %}
                <tr>
                    {% for col in block.Content.columns %}
                    <td class="px-3 py-2 text-sm text-gray-900">
                        {{ row|lookup:col.id }}
                    </td>
                    {% endfor %}
                </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
    {% endif %}
{% elif block.Type == "references" %}
    {# References block - show as simple list in shared view #}
    {% if block.Content.groupIds && block.Content.groupIds|length > 0 %}
    <div class="text-sm text-gray-500">
        <span class="font-medium">References:</span>
        {% for gId in block.Content.groupIds %}
        <span class="inline-flex items-center px-2 py-0.5 bg-gray-100 rounded text-gray-600 ml-1">
            {{ groupNameMap|lookup:gId|default:"Group" }}
        </span>
        {% endfor %}
    </div>
    {% endif %}
{% endif %}

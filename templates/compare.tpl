{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="max-w-7xl mx-auto" x-data="compareView({
    r1: {{ query.Resource1ID }},
    v1: {{ query.Version1|default:0 }},
    r2: {{ query.Resource2ID }},
    v2: {{ query.Version2|default:0 }}
})" @resource1-selected.window="onResource1Change($event.detail.item.ID)" @resource2-selected.window="onResource2Change($event.detail.item.ID)">
    <!-- Resource/Version Pickers -->
    <div class="grid grid-cols-2 gap-6 mb-6">
        <!-- Left Side Picker -->
        <div class="bg-white shadow rounded-lg p-4">
            <label class="block text-sm font-medium text-gray-700 mb-2">Resource</label>
            <div x-data="autocompleter({
                url: '/v1/resources',
                selectedResults: [{{ resource1|json }}],
                elName: 'r1',
                max: 1,
                standalone: true,
                dispatchOnSelect: 'resource1-selected'
            })" class="mb-3 relative">
                <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                       class="w-full border rounded px-3 py-2"
                       placeholder="Search resources...">
                <div x-show="dropdownActive" x-ref="list" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto w-full">
                    <template x-for="(item, index) in results" :key="item.ID">
                        <div @mousedown.prevent="selectedIndex = index; pushVal($event)"
                             class="px-3 py-2 hover:bg-gray-100 cursor-pointer"
                             :class="{ 'bg-indigo-100': selectedIndex === index }"
                             x-text="item.Name"></div>
                    </template>
                </div>
            </div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Version</label>
            <select x-model="v1" @change="updateUrl()" class="w-full border rounded px-3 py-2">
                {% for v in versions1 %}
                <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version1 %}selected{% endif %}>
                    v{{ v.VersionNumber }} - {{ v.CreatedAt|date:"Jan 02, 2006" }}
                </option>
                {% endfor %}
            </select>
        </div>

        <!-- Right Side Picker -->
        <div class="bg-white shadow rounded-lg p-4">
            <label class="block text-sm font-medium text-gray-700 mb-2">Resource</label>
            <div x-data="autocompleter({
                url: '/v1/resources',
                selectedResults: [{{ resource2|json }}],
                elName: 'r2',
                max: 1,
                standalone: true,
                dispatchOnSelect: 'resource2-selected'
            })" class="mb-3 relative">
                <input type="text" x-ref="autocompleter" x-bind="inputEvents"
                       class="w-full border rounded px-3 py-2"
                       placeholder="Search resources...">
                <div x-show="dropdownActive" x-ref="list" class="absolute z-10 bg-white border rounded shadow-lg mt-1 max-h-60 overflow-auto w-full">
                    <template x-for="(item, index) in results" :key="item.ID">
                        <div @mousedown.prevent="selectedIndex = index; pushVal($event)"
                             class="px-3 py-2 hover:bg-gray-100 cursor-pointer"
                             :class="{ 'bg-indigo-100': selectedIndex === index }"
                             x-text="item.Name"></div>
                    </template>
                </div>
            </div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Version</label>
            <select x-model="v2" @change="updateUrl()" class="w-full border rounded px-3 py-2">
                {% for v in versions2 %}
                <option value="{{ v.VersionNumber }}" {% if v.VersionNumber == query.Version2 %}selected{% endif %}>
                    v{{ v.VersionNumber }} - {{ v.CreatedAt|date:"Jan 02, 2006" }}
                </option>
                {% endfor %}
            </select>
        </div>
    </div>

    {% if comparison %}
    <!-- Metadata Comparison Table -->
    <div class="bg-white shadow rounded-lg p-4 mb-6">
        <h3 class="text-lg font-medium mb-4">Metadata Comparison</h3>
        <table class="w-full">
            <thead>
                <tr class="text-left text-gray-600 border-b">
                    <th class="py-2">Property</th>
                    <th class="py-2">Left</th>
                    <th class="py-2">Right</th>
                    <th class="py-2 text-center">Status</th>
                </tr>
            </thead>
            <tbody>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Content Type</td>
                    <td class="py-2">{{ comparison.Version1.ContentType }}</td>
                    <td class="py-2">{{ comparison.Version2.ContentType }}</td>
                    <td class="py-2 text-center">
                        {% if comparison.SameType %}
                        <span class="text-green-600">=</span>
                        {% else %}
                        <span class="text-red-600">≠</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">File Size</td>
                    <td class="py-2">{{ comparison.Version1.FileSize|humanReadableSize }}</td>
                    <td class="py-2">{{ comparison.Version2.FileSize|humanReadableSize }}</td>
                    <td class="py-2 text-center">
                        {% if comparison.SizeDelta == 0 %}
                        <span class="text-green-600">=</span>
                        {% elif comparison.SizeDelta > 0 %}
                        <span class="text-blue-600">+{{ comparison.SizeDelta|humanReadableSize }}</span>
                        {% else %}
                        <span class="text-orange-600">{{ comparison.SizeDelta|humanReadableSize }}</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Dimensions</td>
                    <td class="py-2">{{ comparison.Version1.Width }}×{{ comparison.Version1.Height }}</td>
                    <td class="py-2">{{ comparison.Version2.Width }}×{{ comparison.Version2.Height }}</td>
                    <td class="py-2 text-center">
                        {% if comparison.DimensionsDiff %}
                        <span class="text-red-600">≠</span>
                        {% else %}
                        <span class="text-green-600">=</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Hash Match</td>
                    <td class="py-2 font-mono text-xs">{{ comparison.Version1.Hash|truncatechars:16 }}...</td>
                    <td class="py-2 font-mono text-xs">{{ comparison.Version2.Hash|truncatechars:16 }}...</td>
                    <td class="py-2 text-center">
                        {% if comparison.SameHash %}
                        <span class="text-green-600 text-xl">✓</span>
                        {% else %}
                        <span class="text-red-600 text-xl">✗</span>
                        {% endif %}
                    </td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Created</td>
                    <td class="py-2">{{ comparison.Version1.CreatedAt|date:"Jan 02, 2006 15:04" }}</td>
                    <td class="py-2">{{ comparison.Version2.CreatedAt|date:"Jan 02, 2006 15:04" }}</td>
                    <td class="py-2"></td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Comment</td>
                    <td class="py-2 italic text-gray-500">"{{ comparison.Version1.Comment }}"</td>
                    <td class="py-2 italic text-gray-500">"{{ comparison.Version2.Comment }}"</td>
                    <td class="py-2"></td>
                </tr>
                <tr class="border-b">
                    <td class="py-2 text-gray-600">Resource</td>
                    <td class="py-2"><a href="/resource?id={{ resource1.ID }}" class="text-indigo-600 hover:underline">{{ resource1.Name }}</a></td>
                    <td class="py-2"><a href="/resource?id={{ resource2.ID }}" class="text-indigo-600 hover:underline">{{ resource2.Name }}</a></td>
                    <td class="py-2 text-center">
                        {% if crossResource %}<span class="text-orange-600">≠</span>{% else %}<span class="text-green-600">=</span>{% endif %}
                    </td>
                </tr>
            </tbody>
        </table>
    </div>

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

    {% else %}
    <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-4 text-yellow-800">
        Select versions to compare using the dropdowns above.
    </div>
    {% endif %}
</div>
{% endblock %}

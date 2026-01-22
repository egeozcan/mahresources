{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="bg-white shadow overflow-hidden sm:rounded-lg">
        <div class="px-4 py-5 sm:px-6">
            <h3 class="text-lg leading-6 font-medium text-gray-900">Log Entry Details</h3>
            <p class="mt-1 max-w-2xl text-sm text-gray-500">
                {% if log.Level == "error" %}
                    <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-red-100 text-red-800">error</span>
                {% elif log.Level == "warning" %}
                    <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-yellow-100 text-yellow-800">warning</span>
                {% else %}
                    <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-green-100 text-green-800">info</span>
                {% endif %}
                {{ log.Action }} operation
            </p>
        </div>
        <div class="border-t border-gray-200">
            <dl>
                <div class="bg-gray-50 px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Created At</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">{{ log.CreatedAt|date:"2006-01-02 15:04:05" }}</dd>
                </div>
                <div class="bg-white px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Level</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">{{ log.Level }}</dd>
                </div>
                <div class="bg-gray-50 px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Action</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">{{ log.Action }}</dd>
                </div>
                <div class="bg-white px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Entity</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">
                        {% if log.EntityType and log.EntityID %}
                            {% if log.Action == "delete" %}
                                <span class="text-gray-500">{{ log.EntityType }} #{{ log.EntityID }}</span>
                                <span class="text-red-500 text-xs ml-1">(deleted)</span>
                            {% else %}
                                <a href="/{{ log.EntityType }}?id={{ log.EntityID }}" class="text-indigo-600 hover:underline">
                                    {{ log.EntityType }} #{{ log.EntityID }}
                                </a>
                            {% endif %}
                            {% if log.EntityName %}
                                <span class="text-gray-500">({{ log.EntityName }})</span>
                            {% endif %}
                        {% elif log.EntityType %}
                            {{ log.EntityType }}
                        {% else %}
                            <span class="text-gray-400">N/A</span>
                        {% endif %}
                    </dd>
                </div>
                <div class="bg-gray-50 px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Message</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">{{ log.Message }}</dd>
                </div>
                {% if log.Details %}
                <div class="bg-white px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Details</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">
                        <pre class="bg-gray-100 p-2 rounded text-xs overflow-x-auto" x-data x-init="try { $el.textContent = JSON.stringify(JSON.parse($el.textContent), null, 2) } catch(e) {}">{{ log.Details }}</pre>
                    </dd>
                </div>
                {% endif %}
                {% if log.RequestPath %}
                <div class="bg-gray-50 px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">Request Path</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">{{ log.RequestPath }}</dd>
                </div>
                {% endif %}
                {% if log.IPAddress %}
                <div class="bg-white px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">IP Address</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">{{ log.IPAddress }}</dd>
                </div>
                {% endif %}
                {% if log.UserAgent %}
                <div class="bg-gray-50 px-4 py-3 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">
                    <dt class="text-sm font-medium text-gray-500">User Agent</dt>
                    <dd class="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2 break-words">{{ log.UserAgent }}</dd>
                </div>
                {% endif %}
            </dl>
        </div>
    </div>
{% endblock %}

{% block sidebar %}
    <div class="flex flex-col gap-2">
        <a href="/logs" class="text-indigo-600 hover:underline text-sm">Back to Logs</a>
        {% if log.EntityType and log.EntityID %}
        <a href="/logs?EntityType={{ log.EntityType }}&EntityID={{ log.EntityID }}" class="text-indigo-600 hover:underline text-sm">
            View {{ log.EntityType }} history
        </a>
        {% endif %}
    </div>
{% endblock %}

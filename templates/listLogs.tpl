{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-stone-200" aria-label="Log entries">
            <thead class="bg-stone-50">
                <tr>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Time</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Level</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Action</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Entity</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Message</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-stone-200">
                {% for log in logs %}
                <tr class="hover:bg-stone-50">
                    <td class="px-3 py-2 whitespace-nowrap text-sm text-stone-500 font-mono">
                        <a href="/log?id={{ log.ID }}" class="hover:underline">
                            {{ log.CreatedAt|date:"2006-01-02 15:04:05" }}
                        </a>
                    </td>
                    <td class="px-3 py-2 whitespace-nowrap text-sm">
                        {% if log.Level == "error" %}
                            <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-red-100 text-red-800">error</span>
                        {% elif log.Level == "warning" %}
                            <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-yellow-100 text-yellow-800">warning</span>
                        {% else %}
                            <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-amber-100 text-amber-800">info</span>
                        {% endif %}
                    </td>
                    <td class="px-3 py-2 whitespace-nowrap text-sm text-stone-500 font-mono">
                        {{ log.Action }}
                    </td>
                    <td class="px-3 py-2 whitespace-nowrap text-sm text-stone-500 font-mono">
                        {% if log.EntityType and log.EntityID %}
                            {% if log.Action == "delete" or log.EntityType == "resource_version" %}
                                <span class="text-stone-500">{{ log.EntityType }} #{{ log.EntityID }}</span>
                            {% else %}
                                <a href="/{{ log.EntityType }}?id={{ log.EntityID }}" class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700">
                                    {{ log.EntityType }} #{{ log.EntityID }}
                                </a>
                            {% endif %}
                            {% if log.EntityName %}
                                <span class="text-stone-500">({{ log.EntityName|truncatechars:30 }})</span>
                            {% endif %}
                        {% elif log.EntityType %}
                            {{ log.EntityType }}
                        {% else %}
                            <span class="text-stone-500">-</span>
                        {% endif %}
                    </td>
                    <td class="px-3 py-2 text-sm text-stone-500 font-sans">
                        {{ log.Message|truncatechars:80 }}
                    </td>
                </tr>
                {% empty %}
                <tr>
                    <td colspan="5" class="px-3 py-4 text-sm text-stone-500 text-center font-mono">No log entries found</td>
                </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
{% endblock %}

{% block sidebar %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter logs">
            <label for="Level" class="block text-sm font-medium font-mono text-stone-700 mt-2">Level</label>
            <select name="Level" id="Level" class="mt-1 focus:ring-amber-600 focus:border-amber-600 block w-full shadow-sm sm:text-sm border-stone-300 rounded-md">
                {% for level in logLevels %}
                <option value="{{ level.Link }}" {% if level.Active %}selected{% endif %}>{{ level.Title }}</option>
                {% endfor %}
            </select>

            <label for="Action" class="block text-sm font-medium font-mono text-stone-700 mt-2">Action</label>
            <select name="Action" id="Action" class="mt-1 focus:ring-amber-600 focus:border-amber-600 block w-full shadow-sm sm:text-sm border-stone-300 rounded-md">
                {% for action in logActions %}
                <option value="{{ action.Link }}" {% if action.Active %}selected{% endif %}>{{ action.Title }}</option>
                {% endfor %}
            </select>

            <label for="EntityType" class="block text-sm font-medium font-mono text-stone-700 mt-2">Entity Type</label>
            <select name="EntityType" id="EntityType" class="mt-1 focus:ring-amber-600 focus:border-amber-600 block w-full shadow-sm sm:text-sm border-stone-300 rounded-md">
                {% for entityType in entityTypes %}
                <option value="{{ entityType.Link }}" {% if entityType.Active %}selected{% endif %}>{{ entityType.Title }}</option>
                {% endfor %}
            </select>

            {% include "/partials/form/textInput.tpl" with name='EntityID' label='Entity ID' value=queryValues.EntityID.0 type='number' %}
            {% include "/partials/form/textInput.tpl" with name='Message' label='Message' value=queryValues.Message.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Before' value=queryValues.CreatedBefore.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='After' value=queryValues.CreatedAfter.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </form>
    </div>
{% endblock %}

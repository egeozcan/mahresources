{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200">
            <thead class="bg-gray-50">
                <tr>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Time</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Level</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Action</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Entity</th>
                    <th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Message</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-gray-200">
                {% for log in logs %}
                <tr class="hover:bg-gray-50">
                    <td class="px-3 py-2 whitespace-nowrap text-sm text-gray-500">
                        <a href="/log?id={{ log.ID }}" class="hover:underline">
                            {{ log.CreatedAt|date:"2006-01-02 15:04:05" }}
                        </a>
                    </td>
                    <td class="px-3 py-2 whitespace-nowrap text-sm">
                        {% if log.Level == "error" %}
                            <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-red-100 text-red-800">error</span>
                        {% elif log.Level == "warning" %}
                            <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-yellow-100 text-yellow-800">warning</span>
                        {% else %}
                            <span class="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-green-100 text-green-800">info</span>
                        {% endif %}
                    </td>
                    <td class="px-3 py-2 whitespace-nowrap text-sm text-gray-500">
                        {{ log.Action }}
                    </td>
                    <td class="px-3 py-2 whitespace-nowrap text-sm text-gray-500">
                        {% if log.EntityType and log.EntityID %}
                            {% if log.Action == "delete" %}
                                <span class="text-gray-500">{{ log.EntityType }} #{{ log.EntityID }}</span>
                            {% else %}
                                <a href="/{{ log.EntityType }}?id={{ log.EntityID }}" class="text-indigo-600 hover:underline">
                                    {{ log.EntityType }} #{{ log.EntityID }}
                                </a>
                            {% endif %}
                            {% if log.EntityName %}
                                <span class="text-gray-400">({{ log.EntityName|truncatechars:30 }})</span>
                            {% endif %}
                        {% elif log.EntityType %}
                            {{ log.EntityType }}
                        {% else %}
                            <span class="text-gray-400">-</span>
                        {% endif %}
                    </td>
                    <td class="px-3 py-2 text-sm text-gray-500">
                        {{ log.Message|truncatechars:80 }}
                    </td>
                </tr>
                {% empty %}
                <tr>
                    <td colspan="5" class="px-3 py-4 text-sm text-gray-500 text-center">No log entries found</td>
                </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        <label for="Level" class="block text-sm font-medium text-gray-700 mt-2">Level</label>
        <select name="Level" id="Level" class="mt-1 focus:ring-indigo-500 focus:border-indigo-500 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md">
            {% for level in logLevels %}
            <option value="{{ level.Link }}" {% if level.Active %}selected{% endif %}>{{ level.Title }}</option>
            {% endfor %}
        </select>

        <label for="Action" class="block text-sm font-medium text-gray-700 mt-2">Action</label>
        <select name="Action" id="Action" class="mt-1 focus:ring-indigo-500 focus:border-indigo-500 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md">
            {% for action in logActions %}
            <option value="{{ action.Link }}" {% if action.Active %}selected{% endif %}>{{ action.Title }}</option>
            {% endfor %}
        </select>

        <label for="EntityType" class="block text-sm font-medium text-gray-700 mt-2">Entity Type</label>
        <select name="EntityType" id="EntityType" class="mt-1 focus:ring-indigo-500 focus:border-indigo-500 block w-full shadow-sm sm:text-sm border-gray-300 rounded-md">
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
{% endblock %}

{# with block= shareToken= resourceHashMap= groupDataMap= #}
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
    {# References block - show as list with tooltips in shared view #}
    {% if block.Content.groupIds && block.Content.groupIds|length > 0 %}
    <div class="text-sm text-gray-500">
        <span class="font-medium">References:</span>
        {% for gId in block.Content.groupIds %}
        {% with groupData=groupDataMap|lookup:gId %}
        {% if groupData %}
        <span class="group-reference-tooltip inline-flex items-center px-2 py-0.5 bg-gray-100 rounded text-gray-600 ml-1 cursor-default relative"
              tabindex="0"
              data-group-name="{{ groupData.Name }}"
              data-group-description="{{ groupData.Description|default:'' }}"
              data-group-category="{{ groupData.CategoryName|default:'' }}">
            {{ groupData.Name }}
            <div class="tooltip-content hidden absolute z-50 bottom-full left-1/2 -translate-x-1/2 mb-2 w-64 p-3 bg-gray-900 text-white text-xs rounded-lg shadow-lg">
                <div class="font-semibold text-sm mb-1">{{ groupData.Name }}</div>
                {% if groupData.CategoryName %}
                <div class="text-gray-400 text-xs mb-1">{{ groupData.CategoryName }}</div>
                {% endif %}
                {% if groupData.Description %}
                <div class="text-gray-300 mt-1">{{ groupData.Description|truncatechars:150 }}</div>
                {% endif %}
                <div class="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-gray-900"></div>
            </div>
        </span>
        {% else %}
        <span class="inline-flex items-center px-2 py-0.5 bg-gray-100 rounded text-gray-600 ml-1">
            Group
        </span>
        {% endif %}
        {% endwith %}
        {% endfor %}
    </div>
    {% endif %}
{% elif block.Type == "calendar" %}
    {# Calendar block - read-only view with month/agenda toggle #}
    <div x-data="sharedCalendar({{ block.ID }}, {{ block.Content|json }}, {{ block.State|json }}, '{{ shareToken }}')" x-init="init()">
        {# Header #}
        <div class="flex items-center justify-between mb-4">
            <div class="flex items-center gap-2">
                {# Month navigation - only shown in month view #}
                <template x-if="view === 'month'">
                    <div class="flex items-center gap-2">
                        <button @click="prevMonth()" class="p-1 hover:bg-gray-100 rounded" title="Previous">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
                            </svg>
                        </button>
                        <span class="text-lg font-semibold w-36 text-center" x-text="currentMonth + ' ' + currentYear"></span>
                        <button @click="nextMonth()" class="p-1 hover:bg-gray-100 rounded" title="Next">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                            </svg>
                        </button>
                    </div>
                </template>
                {# Agenda title #}
                <template x-if="view === 'agenda'">
                    <span class="text-lg font-semibold">Upcoming Events</span>
                </template>
            </div>
            <div class="flex items-center gap-2">
                <template x-if="isRefreshing">
                    <span class="text-xs text-gray-400 flex items-center">
                        <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                        </svg>
                    </span>
                </template>
                <div class="flex border border-gray-200 rounded overflow-hidden text-sm">
                    <button @click="setView('month')" class="px-3 py-1" :class="view === 'month' ? 'bg-blue-500 text-white' : 'bg-white hover:bg-gray-50'">Month</button>
                    <button @click="setView('agenda')" class="px-3 py-1" :class="view === 'agenda' ? 'bg-blue-500 text-white' : 'bg-white hover:bg-gray-50'">Agenda</button>
                </div>
            </div>
        </div>

        {# Error #}
        <template x-if="error">
            <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm mb-4">
                <span x-text="error"></span>
                <button @click="fetchEvents(true)" class="ml-2 underline">Retry</button>
            </div>
        </template>

        {# Month view - show even while loading to prevent layout jump #}
        <template x-if="view === 'month'">
            <div class="relative" :class="{ 'opacity-60': loading }">
                <div class="grid grid-cols-7 gap-px bg-gray-200 rounded overflow-hidden">
                    <template x-for="day in ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']">
                        <div class="bg-gray-50 py-2 text-center text-xs font-medium text-gray-500" x-text="day"></div>
                    </template>
                    <template x-for="day in monthDays" :key="day.date.toISOString()">
                        <div class="bg-white min-h-[80px] p-1 relative"
                             :class="{ 'bg-gray-50': !day.isCurrentMonth, 'ring-2 ring-blue-500 ring-inset': isToday(day.date) }">
                            <span class="text-xs" :class="day.isCurrentMonth ? 'text-gray-700' : 'text-gray-400'" x-text="day.date.getDate()"></span>
                            <div class="mt-1 space-y-0.5">
                                <template x-for="event in getEventsForDay(day.date).slice(0, 3)" :key="event.id">
                                    <div class="text-xs px-1 py-0.5 rounded truncate"
                                         :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                         :title="event.title + (event.location ? ' @ ' + event.location : '')"
                                         x-text="event.allDay ? event.title : formatEventTime(event) + ' ' + event.title">
                                    </div>
                                </template>
                                <template x-if="getEventsForDay(day.date).length > 3">
                                    <div class="text-xs text-gray-400 px-1" x-text="'+' + (getEventsForDay(day.date).length - 3) + ' more'"></div>
                                </template>
                            </div>
                        </div>
                    </template>
                </div>
            </div>
        </template>

        {# Agenda view - show even while loading to prevent layout jump #}
        <template x-if="view === 'agenda'">
            <div class="space-y-4 relative" :class="{ 'opacity-60': loading }">
                <template x-if="agendaEvents.length === 0">
                    <div class="text-center py-8 text-gray-400">No upcoming events</div>
                </template>
                <template x-for="group in agendaEvents" :key="group.date.toISOString()">
                    <div>
                        <div class="text-sm font-medium text-gray-600 mb-2" x-text="formatAgendaDate(group.date)"></div>
                        <div class="space-y-2">
                            <template x-for="event in group.events" :key="event.id">
                                <div @click="goToEventMonth(event)"
                                     class="flex items-start gap-3 p-2 rounded hover:bg-gray-50 cursor-pointer"
                                     title="Click to view in month">
                                    <div class="w-1 h-full min-h-[40px] rounded" :style="'background-color: ' + getCalendarColor(event.calendarId)"></div>
                                    <div class="flex-1 min-w-0">
                                        <div class="font-medium text-sm" x-text="event.title"></div>
                                        <div class="text-xs text-gray-500">
                                            <span x-text="formatEventTime(event)"></span>
                                            <span x-show="event.location" class="ml-2">@ <span x-text="event.location"></span></span>
                                        </div>
                                        <div x-show="event.description" class="text-xs text-gray-400 mt-1 line-clamp-2" x-text="event.description"></div>
                                    </div>
                                    <div class="text-xs px-2 py-0.5 rounded"
                                         :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                         x-text="getCalendarName(event.calendarId)">
                                    </div>
                                </div>
                            </template>
                        </div>
                    </div>
                </template>
            </div>
        </template>

        {# Empty state #}
        <template x-if="calendars.length === 0 && !loading">
            <div class="text-center py-8 text-gray-400">
                <p>No calendars configured.</p>
            </div>
        </template>
    </div>
{% endif %}

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
                <button @click="openEventModalForDay(currentDate)"
                        class="px-3 py-1 text-sm bg-blue-500 text-white rounded hover:bg-blue-600">
                    + Add Event
                </button>
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
                        <div class="bg-white min-h-[80px] p-1 relative cursor-pointer hover:bg-blue-50 transition-colors"
                             @click="openEventModalForDay(day.date)"
                             :class="{ 'bg-gray-50 hover:bg-gray-100': !day.isCurrentMonth, 'ring-2 ring-blue-500 ring-inset': isToday(day.date) }">
                            <span class="text-xs" :class="day.isCurrentMonth ? 'text-gray-700' : 'text-gray-400'" x-text="day.date.getDate()"></span>
                            <div class="mt-1 space-y-0.5">
                                <template x-for="event in getEventsForDay(day.date).slice(0, 3)" :key="event.id">
                                    <div @click.stop="isCustomEvent(event) ? openEventModalForEdit(event) : null"
                                         class="text-xs px-1 py-0.5 rounded truncate"
                                         :class="isCustomEvent(event) ? 'cursor-pointer hover:opacity-80' : ''"
                                         :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)"
                                         :title="event.title + (event.location ? ' @ ' + event.location : '') + (isCustomEvent(event) ? ' (click to edit)' : '')"
                                         x-text="event.allDay ? event.title : formatEventTime(event) + ' ' + event.title">
                                    </div>
                                </template>
                                <template x-if="getEventsForDay(day.date).length > 3">
                                    <div @click.stop="toggleExpandedDay(day.date)"
                                         class="text-xs text-blue-500 hover:text-blue-700 px-1 cursor-pointer"
                                         x-text="'+' + (getEventsForDay(day.date).length - 3) + ' more'"></div>
                                </template>
                            </div>
                            {# Expanded events popover #}
                            <template x-if="isExpanded(day.date)">
                                <div class="absolute z-20 left-0 top-full mt-1 w-64 bg-white border border-gray-200 rounded-lg shadow-lg p-2"
                                     @click.stop
                                     @click.away="closeExpandedDay()">
                                    <div class="flex justify-between items-center mb-2 pb-1 border-b">
                                        <span class="text-sm font-medium" x-text="day.date.toLocaleDateString('default', { weekday: 'short', month: 'short', day: 'numeric' })"></span>
                                        <button @click="closeExpandedDay()" class="text-gray-400 hover:text-gray-600">
                                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                                            </svg>
                                        </button>
                                    </div>
                                    <div class="space-y-1 max-h-48 overflow-y-auto">
                                        <template x-for="event in getEventsForDay(day.date)" :key="event.id">
                                            <div @click.stop="if (isCustomEvent(event)) { openEventModalForEdit(event); closeExpandedDay(); }"
                                                 class="text-xs px-2 py-1 rounded"
                                                 :class="isCustomEvent(event) ? 'cursor-pointer hover:opacity-80' : ''"
                                                 :style="'background-color: ' + getCalendarColor(event.calendarId) + '20; color: ' + getCalendarColor(event.calendarId)">
                                                <div class="font-medium" x-text="event.title"></div>
                                                <div class="opacity-75" x-text="formatEventTime(event)"></div>
                                            </div>
                                        </template>
                                    </div>
                                    <button @click="openEventModalForDay(day.date); closeExpandedDay()"
                                            class="w-full mt-2 pt-1 border-t text-xs text-blue-500 hover:text-blue-700">
                                        + Add event
                                    </button>
                                </div>
                            </template>
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
                                <div @click="isCustomEvent(event) ? openEventModalForEdit(event) : goToEventMonth(event)"
                                     class="flex items-start gap-3 p-2 rounded hover:bg-gray-50 cursor-pointer"
                                     :title="isCustomEvent(event) ? 'Click to edit' : 'Click to view in month'">
                                    <div class="w-1 h-full min-h-[40px] rounded" :style="'background-color: ' + getCalendarColor(event.calendarId)"></div>
                                    <div class="flex-1 min-w-0">
                                        <div class="font-medium text-sm flex items-center gap-1">
                                            <span x-text="event.title"></span>
                                            <template x-if="isCustomEvent(event)">
                                                <svg class="w-3 h-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"/>
                                                </svg>
                                            </template>
                                        </div>
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
        <template x-if="calendars.length === 0 && customEvents.length === 0 && !loading">
            <div class="text-center py-8 text-gray-400">
                <p>No calendars or events yet.</p>
                <p class="text-sm mt-1">Click "+ Add Event" to create an event.</p>
            </div>
        </template>

        {# Event Modal #}
        <template x-if="showEventModal">
            <div class="fixed inset-0 z-50 flex items-center justify-center" @keydown.escape.window="closeEventModal()">
                <div class="absolute inset-0 bg-black/50" @click="closeEventModal()"></div>
                <div class="relative bg-white rounded-lg shadow-xl w-full max-w-md mx-4 p-6">
                    <h3 class="text-lg font-semibold mb-4" x-text="editingEvent ? 'Edit Event' : 'New Event'"></h3>

                    <form @submit.prevent="saveEvent()">
                        {# Title #}
                        <div class="mb-4">
                            <label class="block text-sm font-medium text-gray-700 mb-1">Title</label>
                            <input type="text" x-model="eventForm.title" required
                                   class="w-full px-3 py-2 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500">
                        </div>

                        {# All Day toggle #}
                        <label class="flex items-center gap-2 mb-4 cursor-pointer">
                            <input type="checkbox" x-model="eventForm.allDay" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500">
                            <span class="text-sm">All day event</span>
                        </label>

                        {# Start date/time #}
                        <div class="grid grid-cols-2 gap-3 mb-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 mb-1">Start Date</label>
                                <input type="date" x-model="eventForm.startDate" required
                                       class="w-full px-3 py-2 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500">
                            </div>
                            <div x-show="!eventForm.allDay">
                                <label class="block text-sm font-medium text-gray-700 mb-1">Start Time</label>
                                <input type="time" x-model="eventForm.startTime"
                                       class="w-full px-3 py-2 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500">
                            </div>
                        </div>

                        {# End date/time #}
                        <div class="grid grid-cols-2 gap-3 mb-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 mb-1">End Date</label>
                                <input type="date" x-model="eventForm.endDate" required
                                       class="w-full px-3 py-2 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500">
                            </div>
                            <div x-show="!eventForm.allDay">
                                <label class="block text-sm font-medium text-gray-700 mb-1">End Time</label>
                                <input type="time" x-model="eventForm.endTime"
                                       class="w-full px-3 py-2 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500">
                            </div>
                        </div>

                        {# Location #}
                        <div class="mb-4">
                            <label class="block text-sm font-medium text-gray-700 mb-1">Location (optional)</label>
                            <input type="text" x-model="eventForm.location"
                                   class="w-full px-3 py-2 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500">
                        </div>

                        {# Description #}
                        <div class="mb-4">
                            <label class="block text-sm font-medium text-gray-700 mb-1">Description (optional)</label>
                            <textarea x-model="eventForm.description" rows="2"
                                      class="w-full px-3 py-2 border border-gray-300 rounded resize-none focus:ring-blue-500 focus:border-blue-500"></textarea>
                        </div>

                        {# Actions #}
                        <div class="flex justify-between pt-2">
                            <div>
                                <button x-show="editingEvent" type="button" @click="deleteEvent()"
                                        class="px-4 py-2 text-red-600 hover:text-red-800 text-sm">
                                    Delete
                                </button>
                            </div>
                            <div class="flex gap-2">
                                <button type="button" @click="closeEventModal()"
                                        class="px-4 py-2 border border-gray-300 rounded hover:bg-gray-50 text-sm">Cancel</button>
                                <button type="submit"
                                        class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 text-sm">Save</button>
                            </div>
                        </div>
                    </form>
                </div>
            </div>
        </template>
    </div>
{% endif %}

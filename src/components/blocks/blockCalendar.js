// src/components/blocks/blockCalendar.js
// Calendar block component with month/agenda views and stale-while-revalidate caching
//
// ## Tiered Caching Strategy
//
// This component uses a multi-tier caching approach for calendar events:
//
// 1. **Frontend (this file)**: Stale-while-revalidate pattern
//    - STALE_THRESHOLD (5 min): Data is served immediately from cache, background refresh triggered
//    - Beyond threshold: Cache miss, full loading state shown
//    - This provides instant UI feedback for recently viewed calendars
//
// 2. **Backend (block_context.go)**: HTTP cache with conditional requests
//    - Default TTL: 30 minutes (configurable via ICSCacheTTL)
//    - Uses ETag/Last-Modified headers for conditional fetching
//    - Returns stale data if conditional fetch fails (resilience)
//
// The frontend cache is intentionally shorter than the backend cache. This means:
// - Users get instant feedback from frontend cache (5 min)
// - Backend cache reduces external HTTP requests (30 min)
// - Background refreshes keep data reasonably fresh without blocking UI

// Module-level cache for calendar events
const eventCache = new Map();
const STALE_THRESHOLD = 5 * 60 * 1000; // 5 minutes - data still usable but triggers background refresh
const MAX_CACHE_SIZE = 50;

function getCacheKey(blockId, start, end) {
  return `${blockId}:${start}:${end}`;
}

function getCacheEntry(key) {
  return eventCache.get(key);
}

function setCacheEntry(key, data) {
  // LRU eviction if cache is full
  if (eventCache.size >= MAX_CACHE_SIZE) {
    const oldestKey = eventCache.keys().next().value;
    eventCache.delete(oldestKey);
  }
  eventCache.set(key, {
    data,
    timestamp: Date.now()
  });
}

function isCacheFresh(entry) {
  return entry && (Date.now() - entry.timestamp) < STALE_THRESHOLD;
}

function isCacheStale(entry) {
  return entry && (Date.now() - entry.timestamp) >= STALE_THRESHOLD;
}

// Color palette for auto-assigning calendar colors
const COLOR_PALETTE = [
  '#3b82f6', // blue
  '#10b981', // green
  '#f59e0b', // amber
  '#ef4444', // red
  '#8b5cf6', // violet
  '#ec4899', // pink
  '#06b6d4', // cyan
  '#f97316', // orange
];

export function blockCalendar(block, saveContentFn, saveStateFn, getEditMode, noteId) {
  return {
    block,
    saveContentFn,
    saveStateFn,
    getEditMode,
    noteId,

    // Calendar sources from content
    calendars: JSON.parse(JSON.stringify(block?.content?.calendars || [])),

    // View state
    view: block?.state?.view || 'month',
    currentDate: block?.state?.currentDate ? new Date(block.state.currentDate) : new Date(),

    // Custom events stored in state
    customEvents: JSON.parse(JSON.stringify(block?.state?.customEvents || [])),

    // Events data
    events: [],
    calendarMeta: {}, // id -> {name, color}
    loading: false,
    error: null,
    isRefreshing: false,
    lastFetchTime: null,

    // Edit mode state
    newUrl: '',
    showColorPicker: null, // calendar ID being edited

    // Event modal state
    showEventModal: false,
    editingEvent: null,
    eventForm: {
      title: '',
      startDate: '',
      startTime: '09:00',
      endDate: '',
      endTime: '10:00',
      allDay: false,
      location: '',
      description: ''
    },

    // Expanded day popover state (stores date string of expanded day)
    expandedDay: null,

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    // Current month/year for display
    get currentMonth() {
      return this.currentDate.toLocaleString('default', { month: 'long' });
    },
    get currentYear() {
      return this.currentDate.getFullYear();
    },

    // Date range for current view
    get dateRange() {
      const d = new Date(this.currentDate);
      if (this.view === 'month') {
        const start = new Date(d.getFullYear(), d.getMonth(), 1);
        const end = new Date(d.getFullYear(), d.getMonth() + 1, 0);
        return { start, end };
      } else {
        // Agenda: fetch 1 year ahead to ensure we have enough events
        const start = new Date();
        start.setHours(0, 0, 0, 0);
        const end = new Date(start);
        end.setFullYear(end.getFullYear() + 1);
        return { start, end };
      }
    },

    // Format date for API (using local time, not UTC)
    formatDate(date) {
      const year = date.getFullYear();
      const month = String(date.getMonth() + 1).padStart(2, '0');
      const day = String(date.getDate()).padStart(2, '0');
      return `${year}-${month}-${day}`;
    },

    async init() {

      // Build calendar metadata map
      this.calendars.forEach(cal => {
        this.calendarMeta[cal.id] = { name: cal.name, color: cal.color };
      });

      // Add custom calendar metadata if there are custom events
      if (this.customEvents.length > 0) {
        this.calendarMeta['custom'] = { name: 'My Events', color: '#6366f1' };
      }

      if (this.calendars.length > 0 || this.customEvents.length > 0) {
        await this.fetchEvents();
      }
    },

    async fetchEvents(forceRefresh = false) {
      // Only skip fetch if there are no calendars AND no custom events
      if (this.calendars.length === 0 && this.customEvents.length === 0) {
        this.events = [];
        return;
      }

      const { start, end } = this.dateRange;
      const cacheKey = getCacheKey(this.block.id, this.formatDate(start), this.formatDate(end));
      const cacheEntry = getCacheEntry(cacheKey);

      if (!forceRefresh) {
        if (isCacheFresh(cacheEntry)) {
          this.applyEventData(cacheEntry.data);
          return;
        }
        if (isCacheStale(cacheEntry)) {
          this.applyEventData(cacheEntry.data);
          this.backgroundRefresh(cacheKey, start, end);
          return;
        }
      }

      // Cache miss or force refresh
      this.loading = true;
      this.error = null;

      try {
        const data = await this.fetchFromServer(start, end);
        setCacheEntry(cacheKey, data);
        this.applyEventData(data);
      } catch (err) {
        this.error = err.message || 'Failed to load events';
        console.error('Calendar fetch error:', err);
      } finally {
        this.loading = false;
      }
    },

    async backgroundRefresh(cacheKey, start, end) {
      if (this.isRefreshing) return;
      this.isRefreshing = true;
      try {
        const data = await this.fetchFromServer(start, end);
        setCacheEntry(cacheKey, data);
        this.applyEventData(data);
      } catch (err) {
        console.error('Background refresh failed:', err);
      } finally {
        this.isRefreshing = false;
      }
    },

    async fetchFromServer(start, end) {
      const params = new URLSearchParams({
        blockId: this.block.id,
        start: this.formatDate(start),
        end: this.formatDate(end)
      });
      const response = await fetch(`/v1/note/block/calendar/events?${params}`);
      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || `HTTP ${response.status}`);
      }
      return response.json();
    },

    applyEventData(data) {
      this.events = data.events || [];
      this.lastFetchTime = data.cachedAt ? new Date(data.cachedAt) : new Date();
      // Update calendar metadata
      (data.calendars || []).forEach(cal => {
        this.calendarMeta[cal.id] = { name: cal.name, color: cal.color };
      });
      // Display any calendar-specific errors
      const errors = data.errors || [];
      if (errors.length > 0) {
        const errorMessages = errors.map(e => `${e.calendarId}: ${e.error}`).join('; ');
        this.error = `Some calendars failed to load: ${errorMessages}`;
      }
    },

    // Navigation
    prevMonth() {
      const d = new Date(this.currentDate);
      d.setMonth(d.getMonth() - 1);
      this.currentDate = d;
      this.saveState();
      this.fetchEvents();
    },

    nextMonth() {
      const d = new Date(this.currentDate);
      d.setMonth(d.getMonth() + 1);
      this.currentDate = d;
      this.saveState();
      this.fetchEvents();
    },

    setView(v) {
      this.view = v;
      this.saveState();
      this.fetchEvents();
    },

    async saveState() {
      return this.saveStateFn(this.block.id, {
        view: this.view,
        currentDate: this.currentDate.toISOString().split('T')[0],
        customEvents: this.customEvents
      });
    },

    saveContent() {
      this.saveContentFn(this.block.id, { calendars: this.calendars });
    },

    // Calendar management
    addCalendarFromUrl() {
      const trimmedUrl = this.newUrl.trim();
      if (!trimmedUrl) return;

      // Validate URL format
      try {
        new URL(trimmedUrl);
      } catch {
        this.error = 'Invalid URL format. Please enter a valid URL starting with http:// or https://';
        return;
      }

      const id = crypto.randomUUID();
      const colorIndex = this.calendars.length % COLOR_PALETTE.length;
      const newCal = {
        id,
        name: 'Calendar ' + (this.calendars.length + 1),
        color: COLOR_PALETTE[colorIndex],
        source: { type: 'url', url: trimmedUrl }
      };
      this.calendars.push(newCal);
      this.calendarMeta[id] = { name: newCal.name, color: newCal.color };
      this.newUrl = '';
      this.error = null;
      this.saveContent();
      this.fetchEvents(true);
    },

    addCalendarFromResource(resourceId, resourceName) {
      const id = crypto.randomUUID();
      const colorIndex = this.calendars.length % COLOR_PALETTE.length;
      const newCal = {
        id,
        name: resourceName || 'Calendar ' + (this.calendars.length + 1),
        color: COLOR_PALETTE[colorIndex],
        source: { type: 'resource', resourceId }
      };
      this.calendars.push(newCal);
      this.calendarMeta[id] = { name: newCal.name, color: newCal.color };
      this.saveContent();
      this.fetchEvents(true);
    },

    removeCalendar(calId) {
      this.calendars = this.calendars.filter(c => c.id !== calId);
      delete this.calendarMeta[calId];
      this.saveContent();
      this.fetchEvents(true);
    },

    updateCalendarName(calId, name) {
      const cal = this.calendars.find(c => c.id === calId);
      if (cal) {
        cal.name = name;
        this.calendarMeta[calId].name = name;
        this.saveContent();
      }
    },

    updateCalendarColor(calId, color) {
      const cal = this.calendars.find(c => c.id === calId);
      if (cal) {
        cal.color = color;
        this.calendarMeta[calId].color = color;
        this.saveContent();
      }
      this.showColorPicker = null;
    },

    openResourcePicker() {
      const picker = Alpine.store('entityPicker');
      if (!picker) {
        console.error('entityPicker store not found');
        return;
      }
      picker.open({
        entityType: 'resource',
        noteId: this.noteId,
        existingIds: [],
        contentTypeFilter: 'text/calendar',
        onConfirm: async (selectedIds) => {
          // Fetch resource info and add calendars
          const results = await Promise.allSettled(
            selectedIds.map(async (id) => {
              const res = await fetch(`/v1/resource?id=${id}`);
              if (!res.ok) {
                throw new Error(`Failed to fetch resource ${id}: HTTP ${res.status}`);
              }
              const resource = await res.json();
              return { id, name: resource.Name };
            })
          );

          // Process successful fetches - add all at once, then save+fetch once
          let added = false;
          for (const result of results) {
            if (result.status === 'fulfilled') {
              const id = crypto.randomUUID();
              const colorIndex = this.calendars.length % COLOR_PALETTE.length;
              const newCal = {
                id,
                name: result.value.name || 'Calendar ' + (this.calendars.length + 1),
                color: COLOR_PALETTE[colorIndex],
                source: { type: 'resource', resourceId: result.value.id }
              };
              this.calendars.push(newCal);
              this.calendarMeta[id] = { name: newCal.name, color: newCal.color };
              added = true;
            } else {
              console.error('Failed to fetch resource:', result.reason);
            }
          }
          if (added) {
            this.saveContent();
            this.fetchEvents(true);
          }
        }
      });
    },

    // Month view helpers
    get monthDays() {
      const { start, end } = this.dateRange;
      const days = [];
      const firstDay = start.getDay(); // 0-6

      // Pad with previous month days
      for (let i = 0; i < firstDay; i++) {
        const d = new Date(start);
        d.setDate(d.getDate() - (firstDay - i));
        days.push({ date: d, isCurrentMonth: false });
      }

      // Current month days
      for (let d = new Date(start); d <= end; d.setDate(d.getDate() + 1)) {
        days.push({ date: new Date(d), isCurrentMonth: true });
      }

      // Pad to complete weeks
      while (days.length % 7 !== 0) {
        const lastDate = days[days.length - 1].date;
        const d = new Date(lastDate);
        d.setDate(d.getDate() + 1);
        days.push({ date: d, isCurrentMonth: false });
      }

      return days;
    },

    getEventsForDay(date) {
      const dayStart = new Date(date);
      dayStart.setHours(0, 0, 0, 0);
      const dayEnd = new Date(date);
      dayEnd.setHours(23, 59, 59, 999);

      return this.events.filter(e => {
        const eventStart = new Date(e.start);
        const eventEnd = new Date(e.end);
        return eventStart <= dayEnd && eventEnd >= dayStart;
      });
    },

    isToday(date) {
      const today = new Date();
      return date.getDate() === today.getDate() &&
             date.getMonth() === today.getMonth() &&
             date.getFullYear() === today.getFullYear();
    },

    // Expanded day popover helpers
    getDayKey(date) {
      return this.formatDateInput(date);
    },

    isExpanded(date) {
      return this.expandedDay === this.getDayKey(date);
    },

    toggleExpandedDay(date) {
      const key = this.getDayKey(date);
      this.expandedDay = this.expandedDay === key ? null : key;
    },

    closeExpandedDay() {
      this.expandedDay = null;
    },

    // Agenda view helpers
    get agendaEvents() {
      // Take next 30 events and group by date
      const upcomingEvents = this.events.slice(0, 30);
      const groups = {};
      upcomingEvents.forEach(e => {
        const dateKey = new Date(e.start).toDateString();
        if (!groups[dateKey]) {
          groups[dateKey] = { date: new Date(e.start), events: [] };
        }
        groups[dateKey].events.push(e);
      });
      return Object.values(groups).sort((a, b) => a.date - b.date);
    },

    // Navigate to event's month in month view
    // Note: Don't refetch - we already have events loaded from agenda (1 year range)
    // The month view's getEventsForDay() will filter from existing events
    goToEventMonth(event) {
      const eventDate = new Date(event.start);
      this.currentDate = eventDate;
      this.view = 'month';
      this.saveState();
    },

    formatEventTime(event) {
      if (event.allDay) return 'All day';
      const start = new Date(event.start);
      return start.toLocaleTimeString('default', { hour: 'numeric', minute: '2-digit' });
    },

    formatAgendaDate(date) {
      return date.toLocaleDateString('default', { weekday: 'short', month: 'short', day: 'numeric' });
    },

    getCalendarColor(calId) {
      return this.calendarMeta[calId]?.color || '#6b7280';
    },

    getCalendarName(calId) {
      return this.calendarMeta[calId]?.name || 'Unknown';
    },

    // Custom event management
    isCustomEvent(event) {
      return event.calendarId === 'custom';
    },

    // Open modal to create event for a specific day
    openEventModalForDay(date) {
      const dateStr = this.formatDateInput(date);
      this.editingEvent = null;
      this.eventForm = {
        title: '',
        startDate: dateStr,
        startTime: '09:00',
        endDate: dateStr,
        endTime: '10:00',
        allDay: false,
        location: '',
        description: ''
      };
      this.showEventModal = true;
    },

    // Open modal to edit an existing custom event
    openEventModalForEdit(event) {
      if (!this.isCustomEvent(event)) return;

      const start = new Date(event.start);
      const end = new Date(event.end);
      this.editingEvent = event;
      this.eventForm = {
        title: event.title,
        startDate: this.formatDateInput(start),
        startTime: this.formatTimeInput(start),
        endDate: this.formatDateInput(end),
        endTime: this.formatTimeInput(end),
        allDay: event.allDay || false,
        location: event.location || '',
        description: event.description || ''
      };
      this.showEventModal = true;
    },

    closeEventModal() {
      this.showEventModal = false;
      this.editingEvent = null;
    },

    // Save event (create or update)
    async saveEvent() {
      if (!this.eventForm.title.trim()) return;

      let startDateTime, endDateTime;
      if (this.eventForm.allDay) {
        startDateTime = new Date(this.eventForm.startDate + 'T00:00:00');
        endDateTime = new Date(this.eventForm.endDate + 'T23:59:59');
      } else {
        startDateTime = new Date(this.eventForm.startDate + 'T' + this.eventForm.startTime);
        endDateTime = new Date(this.eventForm.endDate + 'T' + this.eventForm.endTime);
      }

      // Auto-adjust end time if needed
      if (endDateTime <= startDateTime && !this.eventForm.allDay) {
        endDateTime = new Date(startDateTime.getTime() + 60 * 60 * 1000);
      }

      const eventData = {
        id: this.editingEvent?.id || crypto.randomUUID(),
        title: this.eventForm.title.trim(),
        start: startDateTime.toISOString(),
        end: endDateTime.toISOString(),
        allDay: this.eventForm.allDay,
        location: this.eventForm.location.trim() || undefined,
        description: this.eventForm.description.trim() || undefined,
        calendarId: 'custom'
      };

      if (this.editingEvent) {
        // Update existing event
        const idx = this.customEvents.findIndex(e => e.id === eventData.id);
        if (idx >= 0) {
          this.customEvents[idx] = eventData;
        }
      } else {
        // Add new event
        this.customEvents.push(eventData);
      }

      // Wait for state to be saved before fetching
      await this.saveState();
      this.closeEventModal();
      // Refresh events from server to get merged list
      await this.fetchEvents(true);
    },

    // Delete a custom event
    async deleteEvent() {
      if (!this.editingEvent) return;

      this.customEvents = this.customEvents.filter(e => e.id !== this.editingEvent.id);
      // Wait for state to be saved before fetching
      await this.saveState();
      this.closeEventModal();
      // Refresh events from server to get updated list
      await this.fetchEvents(true);
    },

    // Format date as YYYY-MM-DD for input[type="date"]
    formatDateInput(date) {
      const year = date.getFullYear();
      const month = String(date.getMonth() + 1).padStart(2, '0');
      const day = String(date.getDate()).padStart(2, '0');
      return `${year}-${month}-${day}`;
    },

    // Format time as HH:MM for input[type="time"]
    formatTimeInput(date) {
      const hours = String(date.getHours()).padStart(2, '0');
      const minutes = String(date.getMinutes()).padStart(2, '0');
      return `${hours}:${minutes}`;
    }
  };
}

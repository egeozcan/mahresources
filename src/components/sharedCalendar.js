// src/components/sharedCalendar.js
// A read-only calendar component for shared notes that displays events from the share server.

// Color palette for auto-assigning calendar colors (same as blockCalendar.js)
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

export function sharedCalendar(blockId, initialContent, initialState, shareToken) {
  return {
    blockId,
    shareToken,

    // Calendar sources from content
    calendars: initialContent?.calendars || [],

    // View state
    view: initialState?.view || 'month',
    currentDate: initialState?.currentDate ? new Date(initialState.currentDate) : new Date(),

    // Events data
    events: [],
    calendarMeta: {}, // id -> {name, color}
    loading: false,
    error: null,
    isRefreshing: false,

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

    // Format date for API
    formatDate(date) {
      return date.toISOString().split('T')[0];
    },

    async init() {
      // Build calendar metadata map
      this.calendars.forEach((cal, index) => {
        this.calendarMeta[cal.id] = {
          name: cal.name || `Calendar ${index + 1}`,
          color: cal.color || COLOR_PALETTE[index % COLOR_PALETTE.length]
        };
      });

      if (this.calendars.length > 0) {
        await this.fetchEvents();
      }
    },

    async fetchEvents(forceRefresh = false) {
      if (this.calendars.length === 0) {
        this.events = [];
        return;
      }

      this.loading = true;
      this.error = null;

      try {
        const { start, end } = this.dateRange;
        const params = new URLSearchParams({
          start: this.formatDate(start),
          end: this.formatDate(end)
        });
        const response = await fetch(`/s/${this.shareToken}/block/${this.blockId}/calendar/events?${params}`);
        if (!response.ok) {
          const err = await response.json().catch(() => ({}));
          throw new Error(err.error || `HTTP ${response.status}`);
        }
        const data = await response.json();
        this.applyEventData(data);
      } catch (err) {
        this.error = err.message || 'Failed to load events';
        console.error('Calendar fetch error:', err);
      } finally {
        this.loading = false;
      }
    },

    applyEventData(data) {
      this.events = data.events || [];
      // Update calendar metadata from response
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

    // Save state to server
    async saveState() {
      try {
        await fetch(`/s/${this.shareToken}/block/${this.blockId}/state`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            view: this.view,
            currentDate: this.formatDate(this.currentDate)
          })
        });
      } catch (err) {
        console.error('Failed to save calendar state:', err);
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

    // Navigate to event's month in month view (from agenda)
    goToEventMonth(event) {
      const eventDate = new Date(event.start);
      this.currentDate = eventDate;
      this.view = 'month';
      this.saveState();
      // Don't refetch - events from agenda (1 year) are still valid
    },

    // Month view helpers
    get monthDays() {
      const d = new Date(this.currentDate);
      const start = new Date(d.getFullYear(), d.getMonth(), 1);
      const end = new Date(d.getFullYear(), d.getMonth() + 1, 0);
      const days = [];
      const firstDay = start.getDay();

      // Pad with previous month days
      for (let i = 0; i < firstDay; i++) {
        const pd = new Date(start);
        pd.setDate(pd.getDate() - (firstDay - i));
        days.push({ date: pd, isCurrentMonth: false });
      }

      // Current month days
      for (let cd = new Date(start); cd <= end; cd.setDate(cd.getDate() + 1)) {
        days.push({ date: new Date(cd), isCurrentMonth: true });
      }

      // Pad to complete weeks
      while (days.length % 7 !== 0) {
        const lastDate = days[days.length - 1].date;
        const nd = new Date(lastDate);
        nd.setDate(nd.getDate() + 1);
        days.push({ date: nd, isCurrentMonth: false });
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
    }
  };
}

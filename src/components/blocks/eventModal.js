// src/components/blocks/eventModal.js
// Reusable modal component for creating and editing calendar events

export function eventModal() {
  return {
    isOpen: false,
    mode: 'create', // 'create' or 'edit'
    event: null,    // The event being edited (if mode is 'edit')

    // Form fields
    title: '',
    startDate: '',
    startTime: '09:00',
    endDate: '',
    endTime: '10:00',
    allDay: false,
    location: '',
    description: '',

    // Callbacks
    onSave: null,
    onDelete: null,

    // Open the modal for creating or editing an event
    open(options = {}) {
      this.mode = options.mode || 'create';
      this.event = options.event || null;
      this.onSave = options.onSave || null;
      this.onDelete = options.onDelete || null;

      if (this.event) {
        // Populate form for editing
        const start = new Date(this.event.start);
        const end = new Date(this.event.end);
        this.title = this.event.title;
        this.startDate = this.formatDateInput(start);
        this.startTime = this.formatTimeInput(start);
        this.endDate = this.formatDateInput(end);
        this.endTime = this.formatTimeInput(end);
        this.allDay = this.event.allDay || false;
        this.location = this.event.location || '';
        this.description = this.event.description || '';
      } else {
        // Defaults for new event
        const targetDate = options.date || this.formatDateInput(new Date());
        this.title = '';
        this.startDate = targetDate;
        this.startTime = '09:00';
        this.endDate = targetDate;
        this.endTime = '10:00';
        this.allDay = false;
        this.location = '';
        this.description = '';
      }

      this.isOpen = true;
    },

    close() {
      this.isOpen = false;
      this.event = null;
      this.onSave = null;
      this.onDelete = null;
    },

    save() {
      if (!this.title.trim()) {
        return;
      }

      let startDateTime, endDateTime;
      if (this.allDay) {
        // For all-day events, use the date at midnight UTC
        startDateTime = new Date(this.startDate + 'T00:00:00');
        endDateTime = new Date(this.endDate + 'T23:59:59');
      } else {
        startDateTime = new Date(this.startDate + 'T' + this.startTime);
        endDateTime = new Date(this.endDate + 'T' + this.endTime);
      }

      // Validate end is after start
      if (endDateTime <= startDateTime && !this.allDay) {
        // Auto-adjust end time to be 1 hour after start
        endDateTime = new Date(startDateTime.getTime() + 60 * 60 * 1000);
      }

      const eventData = {
        id: this.event?.id || crypto.randomUUID(),
        title: this.title.trim(),
        start: startDateTime.toISOString(),
        end: endDateTime.toISOString(),
        allDay: this.allDay,
        location: this.location.trim() || undefined,
        description: this.description.trim() || undefined,
        calendarId: 'custom',
      };

      if (this.onSave) {
        this.onSave(eventData);
      }
      this.close();
    },

    deleteEvent() {
      if (this.onDelete && this.event) {
        this.onDelete(this.event.id);
      }
      this.close();
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
    },

    // Handle all-day toggle - adjust end date if needed
    onAllDayChange() {
      if (this.allDay && this.endDate < this.startDate) {
        this.endDate = this.startDate;
      }
    }
  };
}

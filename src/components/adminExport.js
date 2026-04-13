export function adminExport(initial = {}) {
  return {
    selectedGroups: [],
    groupQuery: '',
    groupResults: [],
    scope: {
      subtree: true,
      owned_resources: true,
      owned_notes: true,
      related_m2m: true,
      group_relations: true,
    },
    fidelity: {
      resource_blobs: true,
      resource_versions: false,
      resource_previews: false,
      resource_series: true,
    },
    schemaDefs: {
      categories_and_types: true,
      tags: true,
      group_relation_types: true,
    },
    relatedDepth: 0,
    estimateResult: null,
    job: null,
    jobInProgress: false,
    downloadUrl: '',
    eventSource: null,

    init() {
      const ids = (initial.preselectedIds || '').split(',').map(s => s.trim()).filter(Boolean);
      if (ids.length === 0) return;
      Promise.all(ids.map(id => fetch('/v1/group?id=' + encodeURIComponent(id))
        .then(r => r.ok ? r.json() : null)
        .catch(() => null)))
        .then(results => {
          this.selectedGroups = results
            .filter(g => g)
            .map(g => ({ id: g.ID || g.id, name: g.Name || g.name }));
        });
    },

    addGroup(g) {
      if (!this.selectedGroups.some(sel => sel.id === g.id)) {
        this.selectedGroups.push(g);
      }
      this.groupQuery = '';
      this.groupResults = [];
    },

    removeGroup(id) {
      this.selectedGroups = this.selectedGroups.filter(g => g.id !== id);
    },

    async searchGroups() {
      if (!this.groupQuery) {
        this.groupResults = [];
        return;
      }
      const url = '/v1/groups?name=' + encodeURIComponent(this.groupQuery) + '&maxResults=10';
      try {
        const res = await fetch(url);
        if (!res.ok) return;
        const data = await res.json();
        const list = Array.isArray(data) ? data : (data.items || []);
        this.groupResults = list.map(g => ({ id: g.ID || g.id, name: g.Name || g.name }));
      } catch (e) {
        this.groupResults = [];
      }
    },

    requestBody() {
      return {
        rootGroupIds: this.selectedGroups.map(g => g.id),
        scope: this.scope,
        fidelity: this.fidelity,
        schemaDefs: this.schemaDefs,
        relatedDepth: this.relatedDepth,
      };
    },

    async estimate() {
      const res = await fetch('/v1/groups/export/estimate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(this.requestBody()),
      });
      if (!res.ok) {
        this.estimateResult = null;
        return;
      }
      this.estimateResult = await res.json();
    },

    async submit() {
      this.jobInProgress = true;
      const res = await fetch('/v1/groups/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(this.requestBody()),
      });
      if (!res.ok) {
        this.jobInProgress = false;
        return;
      }
      const data = await res.json();
      this.job = { id: data.jobId, status: 'pending', phase: 'queued' };
      this.downloadUrl = '/v1/exports/' + encodeURIComponent(data.jobId) + '/download';
      this.subscribeProgress(data.jobId);
    },

    subscribeProgress(jobId) {
      if (this.eventSource) {
        this.eventSource.close();
      }
      this.eventSource = new EventSource('/v1/jobs/events');
      const handleJobPayload = (payload) => {
        if (!payload.job || payload.job.id !== jobId) return;
        this.job = payload.job;
        if (payload.job.status === 'completed') {
          this.jobInProgress = false;
          this.triggerDownload();
          this.eventSource.close();
          this.eventSource = null;
        } else if (payload.job.status === 'failed' || payload.job.status === 'cancelled') {
          this.jobInProgress = false;
          this.eventSource.close();
          this.eventSource = null;
        }
      };
      const handler = (event) => {
        try {
          handleJobPayload(JSON.parse(event.data));
        } catch (e) { /* ignore parse errors */ }
      };
      // Handle init event: if job already completed before we subscribed, pick it up.
      this.eventSource.addEventListener('init', (event) => {
        try {
          const payload = JSON.parse(event.data);
          const jobs = payload.jobs || [];
          const found = jobs.find(j => j.id === jobId);
          if (found) handleJobPayload({ job: found });
        } catch (e) { /* ignore parse errors */ }
      });
      this.eventSource.addEventListener('added', handler);
      this.eventSource.addEventListener('updated', handler);
      this.eventSource.addEventListener('removed', handler);
    },

    triggerDownload() {
      const a = document.createElement('a');
      a.href = this.downloadUrl;
      a.download = '';
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
    },

    humanBytes(bytes) {
      if (!bytes || bytes < 0) return '0 B';
      const units = ['B', 'KB', 'MB', 'GB', 'TB'];
      let n = bytes;
      let i = 0;
      while (n >= 1024 && i < units.length - 1) {
        n /= 1024;
        i++;
      }
      return n.toFixed(n >= 10 || i === 0 ? 0 : 1) + ' ' + units[i];
    },

    danglingEntries() {
      if (!this.estimateResult || !this.estimateResult.danglingByKind) return [];
      return Object.entries(this.estimateResult.danglingByKind).map(([kind, count]) => ({ kind, count }));
    },

    canCancel() {
      if (!this.job) return false;
      return ['pending', 'processing', 'downloading', 'running', 'queued'].includes(this.job.status);
    },

    async cancel() {
      if (!this.job) return;
      try {
        await fetch('/v1/jobs/cancel?id=' + encodeURIComponent(this.job.id), { method: 'POST' });
      } catch (e) { /* ignore */ }
    },
  };
}

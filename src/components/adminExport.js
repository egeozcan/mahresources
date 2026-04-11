export function adminExport(initial = {}) {
  return {
    selectedGroups: [],
    groupQuery: '',
    groupResults: [],
    scope: {
      subtree: true,
      ownedResources: true,
      ownedNotes: true,
      relatedM2M: true,
      groupRelations: true,
    },
    fidelity: {
      resourceBlobs: true,
      resourceVersions: false,
      resourcePreviews: false,
      resourceSeries: true,
    },
    schemaDefs: {
      categoriesAndTypes: true,
      tags: true,
      groupRelationTypes: true,
    },
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
      const handler = (event) => {
        try {
          const payload = JSON.parse(event.data);
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
        } catch (e) { /* ignore parse errors */ }
      };
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

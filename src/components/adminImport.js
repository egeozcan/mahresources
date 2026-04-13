export function adminImport() {
  return {
    selectedFile: null,
    uploading: false,
    jobId: null,
    job: null,
    plan: null,
    error: null,
    eventSource: null,

    // Decision state — collected from interactive review controls
    decisions: {
      parent_group_id: null,
      resource_collision_policy: 'skip',
      guid_collision_policy: 'merge',
      acknowledge_missing_hashes: false,
      mapping_actions: {},    // keyed by source_export_id or source_key
      dangling_actions: {},   // keyed by dangling ref id
      excluded_items: [],     // export IDs unchecked in the item tree
      shell_group_actions: {},
    },

    // UI helpers
    parentGroupQuery: '',
    parentGroupResults: [],
    parentGroupName: '',
    flattenedItems: [],  // pre-computed flat list with depth for rendering

    // Apply state
    applying: false,
    applyJobId: null,
    applyJob: null,
    applyPhase: '',
    applyResult: null,
    applyEventSource: null,

    destroy() {
      if (this.eventSource) {
        this.eventSource.close();
        this.eventSource = null;
      }
      this.closeApplySSE();
    },

    async upload() {
      if (!this.selectedFile) return;
      this.uploading = true;
      this.error = null;
      this.plan = null;
      this.jobId = null;

      try {
        const formData = new FormData();
        formData.append('file', this.selectedFile);
        const resp = await fetch('/v1/groups/import/parse', {
          method: 'POST',
          body: formData,
        });
        if (!resp.ok) {
          const text = await resp.text();
          throw new Error(text || `HTTP ${resp.status}`);
        }
        const data = await resp.json();
        this.jobId = data.jobId;
        this.subscribeProgress(data.jobId);
      } catch (err) {
        this.error = err.message;
      } finally {
        this.uploading = false;
      }
    },

    // SSE subscription — matches existing adminExport.js pattern exactly
    subscribeProgress(jobId) {
      if (this.eventSource) {
        this.eventSource.close();
      }
      this.eventSource = new EventSource('/v1/jobs/events');

      const handleJobPayload = (payload) => {
        if (!payload.job || payload.job.id !== jobId) return;
        this.job = payload.job;
        if (payload.job.status === 'completed') {
          this.onParseComplete(jobId);
          this.closeSSE();
        } else if (payload.job.status === 'failed' || payload.job.status === 'cancelled') {
          this.error = payload.job.error || `Job ${payload.job.status}`;
          this.closeSSE();
        }
      };

      const handler = (event) => {
        try {
          handleJobPayload(JSON.parse(event.data));
        } catch (e) { /* ignore parse errors */ }
      };

      // init event: payload is {jobs: [...], actionJobs: [...]}
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

    async onParseComplete(jobId) {
      try {
        const resp = await fetch(`/v1/imports/${encodeURIComponent(jobId)}/plan`);
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        this.plan = await resp.json();
        this.initDecisionsFromPlan();
      } catch (err) {
        this.error = 'Failed to load import plan: ' + err.message;
      }
    },

    // Pre-fill decisions from the plan's suggestions
    initDecisionsFromPlan() {
      if (!this.plan) return;
      const allMappings = [
        ...(this.plan.mappings.categories || []),
        ...(this.plan.mappings.note_types || []),
        ...(this.plan.mappings.resource_categories || []),
        ...(this.plan.mappings.tags || []),
        ...(this.plan.mappings.group_relation_types || []),
      ];
      for (const entry of allMappings) {
        const key = entry.decision_key;
        if (entry.suggestion && !entry.ambiguous) {
          this.decisions.mapping_actions[key] = {
            include: true,
            action: entry.suggestion,
            destination_id: entry.destination_id || null,
          };
        } else {
          // Ambiguous or no suggestion — include by default but no action pre-set
          this.decisions.mapping_actions[key] = {
            include: true,
            action: '',
            destination_id: null,
          };
        }
      }
      for (const d of (this.plan.dangling_refs || [])) {
        this.decisions.dangling_actions[d.id] = { action: 'drop' };
      }

      // Default all shell groups to "create"
      const walkShells = (items) => {
        for (const item of items) {
          if (item.shell) {
            this.decisions.shell_group_actions[item.export_id] = {
              action: 'create',
              destination_id: null,
            };
          }
          if (item.children?.length) walkShells(item.children);
        }
      };
      walkShells(this.plan.items || []);

      // Flatten the hierarchical item tree for rendering with depth-based indent
      this.flattenedItems = [];
      const flatten = (items, depth) => {
        for (const item of items) {
          this.flattenedItems.push({
            export_id: item.export_id,
            name: item.name,
            depth,
            shell: item.shell || false,
            descendant_resource_count: item.descendant_resource_count || 0,
            descendant_note_count: item.descendant_note_count || 0,
            item, // keep reference for toggleItem recursive walk
          });
          if (item.children?.length) flatten(item.children, depth + 1);
        }
      };
      flatten(this.plan.items || [], 0);
    },

    closeSSE() {
      if (this.eventSource) {
        this.eventSource.close();
        this.eventSource = null;
      }
    },

    // --- Mapping decision helpers ---

    getMappingAction(entry) {
      const key = entry.decision_key;
      const stored = this.decisions.mapping_actions[key]?.action;
      if (stored) return stored;
      // Ambiguous entries have empty suggestion — return '' so the UI
      // shows "-- choose --" instead of silently defaulting to 'create'.
      // The apply button checks hasIncompleteDecisions() and stays
      // disabled until the user makes an explicit choice.
      if (entry.ambiguous) return '';
      return entry.suggestion || 'create';
    },

    // Returns true if any included decision is incomplete — gates the apply button.
    // Catches three cases:
    //  1. Ambiguous mapping with no action chosen
    //  2. Any mapping with action=map but no destination_id
    //  3. Any dangling ref with action=map but no destination_id
    hasIncompleteDecisions() {
      if (!this.plan) return false;

      // Check mappings
      const allMappings = [
        ...(this.plan.mappings.categories || []),
        ...(this.plan.mappings.note_types || []),
        ...(this.plan.mappings.resource_categories || []),
        ...(this.plan.mappings.tags || []),
        ...(this.plan.mappings.group_relation_types || []),
      ];
      const hasBadMapping = allMappings.some(entry => {
        const stored = this.decisions.mapping_actions[entry.decision_key];
        if (stored?.include === false) return false; // excluded, skip
        // Ambiguous with no action
        if (entry.ambiguous && !stored?.action) return true;
        // Any "map" without a destination
        if (stored?.action === 'map' && !stored.destination_id) return true;
        return false;
      });
      if (hasBadMapping) return true;

      // Check dangling refs
      for (const d of (this.plan.dangling_refs || [])) {
        const stored = this.decisions.dangling_actions[d.id];
        if (stored?.action === 'map' && !stored.destination_id) return true;
      }

      // Check shell group decisions (skip excluded items)
      for (const [exportId, action] of Object.entries(this.decisions.shell_group_actions)) {
        if (this.decisions.excluded_items.includes(exportId)) continue;
        if (action.action === 'map_to_existing' && !action.destination_id) return true;
      }

      // Check missing-hash acknowledgement
      if (this.plan.manifest_only_missing_hashes > 0 && !this.decisions.acknowledge_missing_hashes) {
        return true;
      }

      return false;
    },

    setMappingAction(entry, action) {
      const key = entry.decision_key;
      if (!this.decisions.mapping_actions[key]) {
        this.decisions.mapping_actions[key] = {};
      }
      this.decisions.mapping_actions[key].action = action;
      if (action === 'map' && entry.destination_id) {
        this.decisions.mapping_actions[key].destination_id = entry.destination_id;
      } else if (action === 'create') {
        this.decisions.mapping_actions[key].destination_id = null;
      }
    },

    setMappingDest(entry, destIdStr) {
      const key = entry.decision_key;
      if (!this.decisions.mapping_actions[key]) {
        this.decisions.mapping_actions[key] = { action: 'map' };
      }
      this.decisions.mapping_actions[key].destination_id = destIdStr ? parseInt(destIdStr, 10) : null;
    },

    isMappingIncluded(entry) {
      const ma = this.decisions.mapping_actions[entry.decision_key];
      return ma ? ma.include !== false : true;
    },

    toggleMappingInclude(entry, checked) {
      const key = entry.decision_key;
      if (!this.decisions.mapping_actions[key]) {
        this.decisions.mapping_actions[key] = { include: checked, action: entry.suggestion || 'create', destination_id: null };
      } else {
        this.decisions.mapping_actions[key].include = checked;
      }
    },

    mappingSearchResults: {},  // {decisionKey: [{id, name}]}

    mappingDestOverride(entry) {
      const key = entry.decision_key;
      const dest = this.decisions.mapping_actions[key]?.destination_id;
      return dest && dest !== entry.destination_id;
    },

    getMappingDestId(entry) {
      return this.decisions.mapping_actions[entry.decision_key]?.destination_id;
    },

    async searchMappingDest(entry, query) {
      if (!query) { this.mappingSearchResults[entry.decision_key] = []; return; }
      // Determine search endpoint by mapping type context.
      // The entry lives in one of the plan.mappings arrays — search the
      // matching entity type. The plan's key tells us which.
      const typeEndpoints = {
        categories: '/v1/categories',
        note_types: '/v1/note/noteTypes',
        resource_categories: '/v1/resourceCategories',
        tags: '/v1/tags',
        group_relation_types: '/v1/relationTypes',
      };
      // Find which mapping array this entry belongs to
      let endpoint = '/v1/categories'; // fallback
      for (const [mapKey, ep] of Object.entries(typeEndpoints)) {
        if ((this.plan.mappings[mapKey] || []).some(e => e.decision_key === entry.decision_key)) {
          endpoint = ep;
          break;
        }
      }
      try {
        const res = await fetch(endpoint + '?name=' + encodeURIComponent(query) + '&maxResults=8');
        if (!res.ok) return;
        const data = await res.json();
        const list = Array.isArray(data) ? data : (data.items || []);
        this.mappingSearchResults[entry.decision_key] = list.map(e => ({
          id: e.ID || e.id, name: e.Name || e.name,
        }));
      } catch (e) {
        this.mappingSearchResults[entry.decision_key] = [];
      }
    },

    // --- Dangling ref decision helpers ---

    danglingSearchResults: {},  // {danglingId: [{id, name}]}
    danglingDestNames: {},      // {danglingId: name} for display
    shellGroupSearchResults: {},
    shellGroupDestNames: {},

    setDanglingAction(danglingId, action, destId) {
      this.decisions.dangling_actions[danglingId] = {
        action,
        destination_id: destId || null,
      };
      if (action === 'drop') {
        delete this.danglingDestNames[danglingId];
      }
    },

    getDanglingAction(danglingId) {
      return this.decisions.dangling_actions[danglingId]?.action || 'drop';
    },

    getDanglingDest(danglingId) {
      return this.decisions.dangling_actions[danglingId]?.destination_id;
    },

    getDanglingDestName(danglingId) {
      return this.danglingDestNames[danglingId] || '';
    },

    setDanglingDest(danglingId, destId, destName) {
      if (!this.decisions.dangling_actions[danglingId]) {
        this.decisions.dangling_actions[danglingId] = { action: 'map' };
      }
      this.decisions.dangling_actions[danglingId].destination_id = destId;
      this.danglingDestNames[danglingId] = destName;
      this.danglingSearchResults[danglingId] = [];
    },

    async searchDanglingDest(d, query) {
      // Determine the right entity type to search based on dangling kind
      const kindToEndpoint = {
        'related_group': '/v1/groups',
        'group_relation': '/v1/groups',
        'related_resource': '/v1/resources',
        'related_note': '/v1/notes',
        'resource_series_sibling': '/v1/resources',
      };
      const endpoint = kindToEndpoint[d.kind] || '/v1/groups';
      if (!query) { this.danglingSearchResults[d.id] = []; return; }
      try {
        const res = await fetch(endpoint + '?name=' + encodeURIComponent(query) + '&maxResults=8');
        if (!res.ok) return;
        const data = await res.json();
        const list = Array.isArray(data) ? data : (data.items || []);
        this.danglingSearchResults[d.id] = list.map(e => ({ id: e.ID || e.id, name: e.Name || e.name }));
      } catch (e) {
        this.danglingSearchResults[d.id] = [];
      }
    },

    // --- Shell group decision helpers ---

    getShellAction(exportId) {
      return this.decisions.shell_group_actions[exportId]?.action || 'create';
    },

    setShellAction(exportId, action) {
      if (!this.decisions.shell_group_actions[exportId]) {
        this.decisions.shell_group_actions[exportId] = {};
      }
      this.decisions.shell_group_actions[exportId].action = action;
      if (action === 'create') {
        this.decisions.shell_group_actions[exportId].destination_id = null;
        delete this.shellGroupDestNames[exportId];
      }
    },

    setShellDest(exportId, destId, destName) {
      if (!this.decisions.shell_group_actions[exportId]) {
        this.decisions.shell_group_actions[exportId] = { action: 'map_to_existing' };
      }
      this.decisions.shell_group_actions[exportId].destination_id = destId;
      this.shellGroupDestNames[exportId] = destName;
      this.shellGroupSearchResults[exportId] = [];
    },

    async searchShellDest(exportId, query) {
      if (!query) { this.shellGroupSearchResults[exportId] = []; return; }
      try {
        const res = await fetch('/v1/groups?name=' + encodeURIComponent(query) + '&maxResults=8');
        if (!res.ok) return;
        const data = await res.json();
        const list = Array.isArray(data) ? data : (data.items || []);
        this.shellGroupSearchResults[exportId] = list.map(g => ({ id: g.ID || g.id, name: g.Name || g.name }));
      } catch (e) {
        this.shellGroupSearchResults[exportId] = [];
      }
    },

    // --- Item tree pruning helpers ---

    isExcluded(exportId) {
      return this.decisions.excluded_items.includes(exportId);
    },

    toggleItem(item, checked) {
      if (checked) {
        // Include: remove from excluded list (and all descendants)
        this.includeItemRecursive(item);
      } else {
        // Exclude: add to excluded list (and all descendants)
        this.excludeItemRecursive(item);
      }
    },

    excludeItemRecursive(item) {
      if (!this.decisions.excluded_items.includes(item.export_id)) {
        this.decisions.excluded_items.push(item.export_id);
      }
      for (const child of (item.children || [])) {
        this.excludeItemRecursive(child);
      }
    },

    includeItemRecursive(item) {
      this.decisions.excluded_items = this.decisions.excluded_items.filter(id => id !== item.export_id);
      for (const child of (item.children || [])) {
        this.includeItemRecursive(child);
      }
    },

    // --- Parent group search ---

    async searchParentGroups() {
      if (!this.parentGroupQuery) {
        this.parentGroupResults = [];
        return;
      }
      try {
        const res = await fetch('/v1/groups?name=' + encodeURIComponent(this.parentGroupQuery) + '&maxResults=10');
        if (!res.ok) return;
        const data = await res.json();
        const list = Array.isArray(data) ? data : (data.items || []);
        this.parentGroupResults = list.map(g => ({ id: g.ID || g.id, name: g.Name || g.name }));
      } catch (e) {
        this.parentGroupResults = [];
      }
    },

    // --- Utilities ---

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

    async apply() {
      if (this.hasIncompleteDecisions() || this.applying) return;
      this.applying = true;
      this.applyResult = null;
      this.applyJob = null;
      this.applyPhase = '';
      this.error = null;

      try {
        const resp = await fetch(`/v1/imports/${encodeURIComponent(this.jobId)}/apply`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(this.decisions),
        });
        if (!resp.ok) {
          const text = await resp.text();
          throw new Error(text || `HTTP ${resp.status}`);
        }
        const data = await resp.json();
        this.applyJobId = data.jobId;
        this.subscribeApplyProgress(data.jobId);
      } catch (err) {
        this.error = err.message;
        this.applying = false;
      }
    },

    subscribeApplyProgress(jobId) {
      this.closeApplySSE();
      this.applyEventSource = new EventSource('/v1/jobs/events');

      const handleJobPayload = (payload) => {
        if (!payload.job || payload.job.id !== jobId) return;
        this.applyJob = payload.job;
        this.applyPhase = payload.job.phase || '';
        if (payload.job.status === 'completed') {
          this.applying = false;
          this.fetchApplyResult();
          this.closeApplySSE();
        } else if (payload.job.status === 'failed' || payload.job.status === 'cancelled') {
          this.applying = false;
          this.error = payload.job.error || `Apply job ${payload.job.status}`;
          this.fetchApplyResult(); // partial-failure may have result
          this.closeApplySSE();
        }
      };

      const handler = (event) => {
        try {
          handleJobPayload(JSON.parse(event.data));
        } catch (e) { /* ignore parse errors */ }
      };

      this.applyEventSource.addEventListener('init', (event) => {
        try {
          const payload = JSON.parse(event.data);
          const jobs = payload.jobs || [];
          const found = jobs.find(j => j.id === jobId);
          if (found) handleJobPayload({ job: found });
        } catch (e) { /* ignore parse errors */ }
      });

      this.applyEventSource.addEventListener('added', handler);
      this.applyEventSource.addEventListener('updated', handler);
      this.applyEventSource.addEventListener('removed', handler);
    },

    async fetchApplyResult() {
      try {
        const resp = await fetch(`/v1/imports/${encodeURIComponent(this.jobId)}/result`);
        if (!resp.ok) return; // 404 means no result yet
        this.applyResult = await resp.json();
      } catch (e) { /* ignore */ }
    },

    closeApplySSE() {
      if (this.applyEventSource) {
        this.applyEventSource.close();
        this.applyEventSource = null;
      }
    },

    async cancelApply() {
      if (!this.applyJobId) return;
      try {
        await fetch('/v1/jobs/cancel?id=' + encodeURIComponent(this.applyJobId), { method: 'POST' });
      } catch (e) { /* ignore */ }
    },
  };
}

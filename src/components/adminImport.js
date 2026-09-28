import { errorMessageFromResponse } from '../index.js';

// The decisions a review starts from, before its plan's suggestions are applied.
function freshDecisions() {
  return {
    parent_group_id: null,
    resource_collision_policy: 'skip',
    guid_collision_policy: 'merge',
    acknowledge_missing_hashes: false,
    mapping_actions: {},    // keyed by source_export_id or source_key
    dangling_actions: {},   // keyed by dangling ref id
    excluded_items: [],     // export IDs unchecked in the item tree
    shell_group_actions: {},
  };
}

function terminalApplyFailure(job) {
  return job?.failure?.message || job?.error || (job?.status === 'cancelled' ? 'The apply was cancelled.' : 'The apply failed.');
}

export function adminImport() {
  return {
    selectedFile: null,
    uploading: false,
    jobId: null,
    canonicalParseJobId: null,
    job: null,
    plan: null,
    error: null,
    eventSource: null,
    // Every request and event callback belongs to the import that started it.
    // Closing a stream does not stop fetches that the stream already started.
    _importGeneration: 0,

    // Decision state — collected from interactive review controls
    decisions: freshDecisions(),

    // UI helpers
    parentGroupQuery: '',
    parentGroupResults: [],
    parentGroupName: '',
    // -1 = no option marked. Drives aria-activedescendant (findings 36/105).
    parentActiveIndex: -1,
    // Monotonic counter so a slow earlier search cannot overwrite a newer one.
    _parentSearchGeneration: 0,
    flattenedItems: [],  // pre-computed flat list with depth for rendering

    // Apply state
    applying: false,
    applyJobId: null,
    applyCanonicalJobId: null,
    applyJob: null,
    applyPhase: '',
    applyResult: null,
    // How the apply the report belongs to ended, as far as this viewer can tell:
    // 'succeeded', 'failed', or 'unknown' when its Job cannot be read (another
    // account applied the import, or its record is gone). An unknown outcome is
    // never shown as a success: a partial apply writes a report too.
    applyOutcome: '',
    applyEventSource: null,
    applyReadNotice: '',
    applyJobURL: '',

    // Set when /admin/import?job=<handle> names an import whose review cannot
    // be shown any more: what happened to it, and the page of the Job that
    // parsed it, where its apply and report are.
    resumeNotice: '',
    resumeJobURL: '',

    init() {
      // A parsed import is reviewed from its plan on the server, so the page can
      // be left and come back to: the parse's Job links here with its handle,
      // and an upload puts the handle in the address so a reload keeps it.
      const handle = new URLSearchParams(window.location.search).get('job');
      if (handle) void this.resume(handle);
    },

    destroy() {
      this._importGeneration++;
      this._parentSearchGeneration++;
      this.closeSSE();
      this.closeApplySSE();
    },

    ownsImport(generation) {
      return generation === this._importGeneration;
    },

    // Forgets everything one import left on the page, so the next one (an
    // upload, or a handle to resume) starts from nothing: its review, its
    // decisions, its apply and its report.
    resetImport() {
      this._importGeneration++;
      this._parentSearchGeneration++;
      this.closeSSE();
      this.closeApplySSE();
      this.uploading = false;
      this.jobId = null;
      this.canonicalParseJobId = null;
      this.job = null;
      this.plan = null;
      this.error = null;
      this.decisions = freshDecisions();
      this.parentGroupQuery = '';
      this.parentGroupResults = [];
      this.parentGroupName = '';
      this.parentActiveIndex = -1;
      this.flattenedItems = [];
      this.mappingSearchResults = {};
      this.danglingSearchResults = {};
      this.danglingDestNames = {};
      this.shellGroupSearchResults = {};
      this.shellGroupDestNames = {};
      this.applying = false;
      this.applyJobId = null;
      this.applyCanonicalJobId = null;
      this.applyJob = null;
      this.applyPhase = '';
      this.applyResult = null;
      this.applyOutcome = '';
      this.applyReadNotice = '';
      this.applyJobURL = '';
      this.resumeNotice = '';
      this.resumeJobURL = '';
    },

    async upload() {
      if (!this.selectedFile) return;
      this.resetImport();
      const generation = this._importGeneration;
      const file = this.selectedFile;
      this.uploading = true;

      try {
        const formData = new FormData();
        formData.append('file', file);
        const resp = await fetch('/v1/groups/import/parse', {
          method: 'POST',
          body: formData,
        });
        if (!this.ownsImport(generation)) return;
        if (!resp.ok) {
          // errorMessageFromResponse, not resp.text(): these endpoints answer JSON
          // for the errors a reader can actually provoke — a 403 from the CSRF
          // middleware is `{"error":"invalid or missing CSRF token"}` — and the raw
          // text of that is a JSON blob shown verbatim in the UI. It also handles
          // the plain-text bodies these two endpoints still return, and falls back
          // to the status line for an HTML error document.
          const message = await errorMessageFromResponse(resp);
          if (!this.ownsImport(generation)) return;
          throw new Error(message);
        }
        const data = await resp.json();
        if (!this.ownsImport(generation)) return;
        this.jobId = data.jobId;
        this.canonicalParseJobId = data.canonicalJobId || null;
        this.rememberHandle(data.jobId);
        this.subscribeProgress(data.jobId, generation, this.canonicalParseJobId);
      } catch (err) {
        if (this.ownsImport(generation)) this.error = err.message;
      } finally {
        if (this.ownsImport(generation)) this.uploading = false;
      }
    },

    // Puts the import's handle in the address without navigating, so a reload
    // or a bookmark comes back to this import.
    rememberHandle(handle) {
      try {
        const url = new URL(window.location.href);
        url.searchParams.set('job', handle);
        window.history.replaceState(window.history.state, '', url.pathname + url.search + url.hash);
      } catch (_) { /* the address is a convenience; the import goes on without it */ }
    },

    /**
     * Restores the import a handle names: its review while the plan is waiting,
     * its parse while that is running, and otherwise the apply that took the plan
     * (its report, the groups it created and how it ended).
     *
     * Every read here is authorized by the server for this viewer, and a handle
     * the viewer may not see answers exactly as one that does not exist, so the
     * page can tell nobody whether an import is there.
     */
    async resume(handle) {
      this.resetImport();
      const generation = this._importGeneration;
      this.jobId = handle;
      const encoded = encodeURIComponent(handle);
      try {
        const planState = await this.loadPlan(handle, generation);
        if (!this.ownsImport(generation) || planState === 'loaded') return;
        // No plan to review: the parse is still running, it failed, or an apply
        // took the plan (or the import's files were removed). The parse's own
        // record says which.
        const jobResp = await fetch(`/v1/jobs/get?id=${encoded}`);
        if (!this.ownsImport(generation)) return;
        if (jobResp.status === 404) {
          this.jobId = null;
          this.resumeNotice = 'This import could not be found. Its files may have been removed; upload the archive again to import it.';
          return;
        }
        if (!jobResp.ok) {
          const message = await errorMessageFromResponse(jobResp);
          if (!this.ownsImport(generation)) return;
          throw new Error(message);
        }
        const parse = await jobResp.json();
        if (!this.ownsImport(generation)) return;
        this.job = parse;
        this.canonicalParseJobId = parse.canonicalJobId || null;
        if (parse.canonicalJobId) {
          this.resumeJobURL = '/job?id=' + encodeURIComponent(parse.canonicalJobId);
        }
        if (parse.status === 'failed' || parse.status === 'cancelled') {
          this.error = parse.error || `Job ${parse.status}`;
          return;
        }
        if (parse.status !== 'completed') {
          this.subscribeProgress(handle, generation);
          return;
        }
        await this.reconcileParseCompletion(handle, parse.canonicalJobId, generation);
      } catch (err) {
        if (this.ownsImport(generation)) this.error = err.message;
      }
    },

    // Read and install a plan only while the request still belongs to the
    // current import.  A terminal parse can publish its plan between reads.
    async loadPlan(handle, generation) {
      if (!this.ownsImport(generation)) return 'stale';
      const resp = await fetch(`/v1/imports/${encodeURIComponent(handle)}/plan`);
      if (!this.ownsImport(generation)) return 'stale';
      if (resp.status === 404) return 'missing';
      if (!resp.ok) {
        const message = await errorMessageFromResponse(resp);
        if (!this.ownsImport(generation)) return 'stale';
        throw new Error(message);
      }
      const plan = await resp.json();
      if (!this.ownsImport(generation)) return 'stale';
      this.plan = plan;
      this.initDecisionsFromPlan();
      return 'loaded';
    },

    // A successful parse can race with plan publication or with an apply that
    // consumes the plan. Both resume and SSE completion use this one transition.
    async reconcileParseCompletion(handle, parseJobId, generation) {
      if (!this.ownsImport(generation)) return;
      const planState = await this.loadPlan(handle, generation);
      if (!this.ownsImport(generation) || planState === 'loaded') return;
      if (!parseJobId) {
        const parseResp = await fetch(`/v1/jobs/get?id=${encodeURIComponent(handle)}`);
        if (!this.ownsImport(generation)) return;
        if (!parseResp.ok) {
          if (parseResp.status === 404) return this.resumeApplied(handle, generation);
          const message = await errorMessageFromResponse(parseResp);
          if (!this.ownsImport(generation)) return;
          throw new Error(message);
        }
        const parse = await parseResp.json();
        if (!this.ownsImport(generation)) return;
        parseJobId = parse.canonicalJobId;
      }
      if (!this.ownsImport(generation)) return;
      this.jobId = null;
      await this.resumeApplied(handle, generation);
    },

    // Shows an apply report only with the outcome the server verified for the
    // exact Job that published that report. Visible lineage is history, not proof
    // that one of its Jobs produced the current report.
    async resumeApplied(handle, generation = this._importGeneration) {
      if (!this.ownsImport(generation)) return;
      const resultResp = await fetch(`/v1/imports/${encodeURIComponent(handle)}/result`);
      if (!this.ownsImport(generation)) return;
      // Only a 404 means there is no report; any other failure is a read that
      // answered nothing, and says so rather than looking like a removed import.
      if (!resultResp.ok && resultResp.status !== 404) {
        const message = await errorMessageFromResponse(resultResp);
        if (!this.ownsImport(generation)) return;
        throw new Error('The import report could not be read: ' + message);
      }
      const result = resultResp.ok ? await resultResp.json() : null;
      if (!this.ownsImport(generation)) return;
      if (!result) {
        this.applyOutcome = 'unknown';
        this.resumeNotice = 'The import report is not available, so its outcome could not be verified.';
        return;
      }
      const state = result.apply_outcome;
      this.applyOutcome = state === 'succeeded' || state === 'failed' || state === 'cancelled' ? state : 'unknown';
      this.applyResult = result;
      this.error = this.applyOutcome === 'failed' || this.applyOutcome === 'cancelled'
        ? (result.apply_failure || (this.applyOutcome === 'cancelled' ? 'The apply was cancelled.' : 'The apply failed.'))
        : null;
      this.resumeNotice = ['queued', 'running', 'scheduled', 'paused', 'blocked'].includes(state)
        ? 'This import is being applied. Follow it in the Jobs panel or on its Job page.'
        : '';
    },

    async latestApply(parseJobId, generation = this._importGeneration) {
      if (!this.ownsImport(generation) || !parseJobId) return null;
      // A 404 is a Job this viewer cannot see; any other failure is a read that
      // answered nothing, and is reported rather than read as invisibility.
      const read = async (id) => {
        if (!this.ownsImport(generation)) return null;
        const resp = await fetch(`/v1/jobs/${encodeURIComponent(id)}`);
        if (!this.ownsImport(generation)) return null;
        if (resp.status === 404) return null;
        if (!resp.ok) {
          const message = await errorMessageFromResponse(resp);
          if (!this.ownsImport(generation)) return null;
          throw new Error('The import\'s Jobs could not be read: ' + message);
        }
        const detail = await resp.json();
        return this.ownsImport(generation) ? detail : null;
      };
      // The API returns relatives newest-first by accepted instant, then Job ID.
      // Preserve that order: RFC3339 permits fractional seconds of varying width,
      // so comparing timestamp strings can put an older Job ahead of a newer one.
      const newest = (jobs) => (Array.isArray(jobs) ? jobs : [])
        .find(job => job?.kind === 'group-import-apply') || null;
      const parse = await read(parseJobId);
      if (!this.ownsImport(generation)) return null;
      let apply = newest(parse?.lineage?.children);
      // A Retry of an apply is a new Job linked to it; the newest one is the one
      // whose outcome the report describes. An apply this viewer cannot see, or
      // one retried by an account it cannot see, has an outcome it cannot know,
      // and answers null rather than a guess. Retry lineage is linear, so the walk
      // ends; the cap only guards a malformed answer.
      for (let hop = 0; apply && hop < 1000; hop++) {
        const detail = await read(apply.id);
        if (!this.ownsImport(generation)) return null;
        if (!detail || detail.lineage?.retriedElsewhere) return null;
        const next = newest(detail.lineage?.successors);
        if (!next) return detail;
        apply = next;
      }
      return null;
    },

    // SSE subscription — matches existing adminExport.js pattern exactly
    subscribeProgress(jobId, generation = this._importGeneration, parseJobId = this.canonicalParseJobId) {
      if (!this.ownsImport(generation)) return;
      this.closeSSE();
      const source = new EventSource('/v1/jobs/events');
      this.eventSource = source;

      const handleJobPayload = (payload) => {
        if (!this.ownsImport(generation) || this.eventSource !== source || !payload.job || payload.job.id !== jobId) return;
        this.job = payload.job;
        if (payload.job.status === 'completed') {
          void this.onParseComplete(jobId, parseJobId || payload.job.canonicalJobId, generation)
            .catch(err => { if (this.ownsImport(generation)) this.error = err.message; });
          this.closeSSE(source);
        } else if (payload.job.status === 'failed' || payload.job.status === 'cancelled') {
          this.error = payload.job.error || `Job ${payload.job.status}`;
          this.closeSSE(source);
        }
      };

      const handler = (event) => {
        try {
          handleJobPayload(JSON.parse(event.data));
        } catch (e) { /* ignore parse errors */ }
      };

      // init event: payload is {jobs: [...], actionJobs: [...]}
      source.addEventListener('init', (event) => {
        try {
          const payload = JSON.parse(event.data);
          const jobs = payload.jobs || [];
          const found = jobs.find(j => j.id === jobId);
          if (found) handleJobPayload({ job: found });
        } catch (e) { /* ignore parse errors */ }
      });

      source.addEventListener('added', handler);
      source.addEventListener('updated', handler);
      source.addEventListener('removed', handler);
    },

    async onParseComplete(jobId, parseJobId, generation = this._importGeneration) {
      await this.reconcileParseCompletion(jobId, parseJobId, generation);
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

    closeSSE(requestedSource = this.eventSource) {
      const source = requestedSource;
      if (!source) return;
      source.close();
      if (this.eventSource === source) this.eventSource = null;
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
      const generation = this._importGeneration;
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
        if (!this.ownsImport(generation)) return;
        if (!res.ok) return;
        const data = await res.json();
        if (!this.ownsImport(generation)) return;
        const list = Array.isArray(data) ? data : (data.items || []);
        this.mappingSearchResults[entry.decision_key] = list.map(e => ({
          id: e.ID || e.id, name: e.Name || e.name,
        }));
      } catch (e) {
        if (!this.ownsImport(generation)) return;
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
      const generation = this._importGeneration;
      try {
        const res = await fetch(endpoint + '?name=' + encodeURIComponent(query) + '&maxResults=8');
        if (!this.ownsImport(generation)) return;
        if (!res.ok) return;
        const data = await res.json();
        if (!this.ownsImport(generation)) return;
        const list = Array.isArray(data) ? data : (data.items || []);
        this.danglingSearchResults[d.id] = list.map(e => ({ id: e.ID || e.id, name: e.Name || e.name }));
      } catch (e) {
        if (!this.ownsImport(generation)) return;
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
      const generation = this._importGeneration;
      try {
        const res = await fetch('/v1/groups?name=' + encodeURIComponent(query) + '&maxResults=8');
        if (!this.ownsImport(generation)) return;
        if (!res.ok) return;
        const data = await res.json();
        if (!this.ownsImport(generation)) return;
        const list = Array.isArray(data) ? data : (data.items || []);
        this.shellGroupSearchResults[exportId] = list.map(g => ({ id: g.ID || g.id, name: g.Name || g.name }));
      } catch (e) {
        if (!this.ownsImport(generation)) return;
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

    /**
     * Move the active-descendant marker in the parent-group listbox.
     *
     * Findings 36/105: ArrowDown used to leave focus in the input and do nothing.
     * Focus stays on the input by design (the combobox pattern); the options carry
     * tabindex="-1" and only aria-activedescendant moves.
     */
    moveParentActive(delta) {
      if (!this.parentGroupResults.length) return;
      const last = this.parentGroupResults.length - 1;
      let next = this.parentActiveIndex + delta;
      if (next < 0) next = last;
      if (next > last) next = 0;
      this.parentActiveIndex = next;
    },

    commitParentActive() {
      const g = this.parentGroupResults[this.parentActiveIndex];
      if (g) this.selectParentGroup(g);
    },

    /** One place that applies a selection, so the click and Enter paths cannot drift. */
    selectParentGroup(g) {
      this.decisions.parent_group_id = g.id;
      this.parentGroupName = g.name;
      this.parentGroupQuery = '';
      this.parentGroupResults = [];
      this.parentActiveIndex = -1;
    },

    /**
     * Search groups for the parent picker.
     *
     * Generation-guarded for the same reason as adminExport.searchGroups: with two
     * requests in flight, a slow earlier one resolving last overwrote the newer
     * results. No abort, no ordering check — the race src/selector/ was built to
     * eliminate. Found while adding the ARIA for findings 36/105.
     */
    async searchParentGroups() {
      const generation = ++this._parentSearchGeneration;
      const importGeneration = this._importGeneration;
      if (!this.parentGroupQuery) {
        this.parentGroupResults = [];
        this.parentActiveIndex = -1;
        return;
      }
      try {
        const res = await fetch('/v1/groups?name=' + encodeURIComponent(this.parentGroupQuery) + '&maxResults=10');
        if (generation !== this._parentSearchGeneration || !this.ownsImport(importGeneration)) return;
        if (!res.ok) return;
        const data = await res.json();
        if (generation !== this._parentSearchGeneration || !this.ownsImport(importGeneration)) return;
        const list = Array.isArray(data) ? data : (data.items || []);
        this.parentGroupResults = list.map(g => ({ id: g.ID || g.id, name: g.Name || g.name }));
        this.parentActiveIndex = -1;
      } catch (e) {
        if (generation !== this._parentSearchGeneration || !this.ownsImport(importGeneration)) return;
        this.parentGroupResults = [];
        this.parentActiveIndex = -1;
      }
    },

    // --- Conflict outcomes ---

    // What the apply does with a resource whose content is already here, under the
    // resource collision policy currently chosen.
    resourceCollisionOutcome() {
      return this.decisions.resource_collision_policy === 'duplicate'
        ? 'will be imported as duplicate rows'
        : 'will be skipped, keeping the existing resource';
    },

    // What the apply does with an entity whose GUID is already here, under the GUID
    // policy currently chosen.
    guidPolicyOutcome() {
      switch (this.decisions.guid_collision_policy) {
        case 'skip': return 'will be skipped, keeping the existing rows';
        case 'replace': return 'will replace the existing rows';
        default: return 'will be merged into the existing rows';
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
      const generation = this._importGeneration;
      const handle = this.jobId;
      const decisions = JSON.stringify(this.decisions);
      this.applying = true;
      this.applyResult = null;
      this.applyOutcome = '';
      this.applyJob = null;
      this.applyPhase = '';
      this.error = null;
      this.applyReadNotice = '';
      this.applyJobURL = '';

      try {
        const resp = await fetch(`/v1/imports/${encodeURIComponent(handle)}/apply`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: decisions,
        });
        if (!this.ownsImport(generation)) return;
        if (!resp.ok) {
          // errorMessageFromResponse, not resp.text(): these endpoints answer JSON
          // for the errors a reader can actually provoke — a 403 from the CSRF
          // middleware is `{"error":"invalid or missing CSRF token"}` — and the raw
          // text of that is a JSON blob shown verbatim in the UI. It also handles
          // the plain-text bodies these two endpoints still return, and falls back
          // to the status line for an HTML error document.
          const message = await errorMessageFromResponse(resp);
          if (!this.ownsImport(generation)) return;
          throw new Error(message);
        }
        const data = await resp.json();
        if (!this.ownsImport(generation)) return;
        this.applyJobId = data.jobId;
        this.applyCanonicalJobId = data.canonicalJobId || null;
        this.applyJobURL = this.applyCanonicalJobId ? '/job?id=' + encodeURIComponent(this.applyCanonicalJobId) : '';
        this.subscribeApplyProgress(data.jobId, handle, generation, this.applyCanonicalJobId);
      } catch (err) {
        if (!this.ownsImport(generation)) return;
        this.error = err.message;
        this.applying = false;
      }
    },

    subscribeApplyProgress(jobId, importHandle = this.jobId, generation = this._importGeneration, canonicalApplyId = null) {
      if (!this.ownsImport(generation)) return;
      this.closeApplySSE();
      const source = new EventSource('/v1/jobs/events');
      this.applyEventSource = source;

      const handleJobPayload = (payload) => {
        if (!this.ownsImport(generation) || this.applyEventSource !== source || !payload.job || payload.job.id !== jobId) return;
        this.applyJob = payload.job;
        this.applyPhase = payload.job.phase || '';
        if (payload.job.status === 'completed') {
          this.applying = false;
          this.applyOutcome = '';
          this.applyReadNotice = '';
          this.error = null;
          void this.fetchApplyResult(importHandle, generation, canonicalApplyId, payload.job);
          this.closeApplySSE(source);
        } else if (payload.job.status === 'failed' || payload.job.status === 'cancelled') {
          this.applying = false;
          this.applyOutcome = '';
          this.applyReadNotice = '';
          this.error = terminalApplyFailure(payload.job);
          void this.fetchApplyResult(importHandle, generation, canonicalApplyId, payload.job); // partial-failure may have result
          this.closeApplySSE(source);
        }
      };

      const handler = (event) => {
        try {
          handleJobPayload(JSON.parse(event.data));
        } catch (e) { /* ignore parse errors */ }
      };

      source.addEventListener('init', (event) => {
        try {
          const payload = JSON.parse(event.data);
          const jobs = payload.jobs || [];
          const found = jobs.find(j => j.id === jobId);
          if (found) handleJobPayload({ job: found });
        } catch (e) { /* ignore parse errors */ }
      });

      source.addEventListener('added', handler);
      source.addEventListener('updated', handler);
      source.addEventListener('removed', handler);
    },

    async fetchApplyResult(importHandle = this.jobId, generation = this._importGeneration, expectedApplyId = null, terminalJob = null) {
      if (!this.ownsImport(generation)) return;
      const jobFailure = terminalJob && (terminalJob.status === 'failed' || terminalJob.status === 'cancelled')
        ? terminalApplyFailure(terminalJob)
        : null;
      try {
        const init = expectedApplyId ? { headers: { 'X-Expected-Import-Apply': expectedApplyId } } : undefined;
        const resp = await fetch(`/v1/imports/${encodeURIComponent(importHandle)}/result`, init);
        if (!this.ownsImport(generation)) return;
        if (!resp.ok) {
          this.applyOutcome = '';
          this.applyResult = null;
          if (resp.status === 404) {
            this.applyReadNotice = 'No Apply report is available. Check the accepted Apply Job for its status.';
          } else {
            this.applyReadNotice = 'The Apply report could not be read. Check the accepted Apply Job for its status and details.';
          }
          this.error = jobFailure;
          return; // An unreadable response does not establish a report outcome.
        }
        const result = await resp.json();
        if (!this.ownsImport(generation)) return;
        const outcome = result.apply_outcome === 'succeeded' || result.apply_outcome === 'failed' || result.apply_outcome === 'cancelled'
          ? result.apply_outcome
          : 'unknown';
        const failure = outcome === 'failed' || outcome === 'cancelled'
          ? (result.apply_failure || terminalApplyFailure(terminalJob))
          : null;
        this.applyResult = result;
        this.applyOutcome = outcome;
        this.applyReadNotice = '';
        this.error = failure;
      } catch (e) {
        if (!this.ownsImport(generation)) return;
        this.applyReadNotice = 'The Apply report could not be read. Check the accepted Apply Job for its status and details.';
        this.error = jobFailure;
      }
    },

    closeApplySSE(requestedSource = this.applyEventSource) {
      if (!requestedSource) return;
      requestedSource.close();
      if (this.applyEventSource === requestedSource) this.applyEventSource = null;
    },

    async cancelApply() {
      if (!this.applyJobId) return;
      try {
        await fetch('/v1/jobs/cancel?id=' + encodeURIComponent(this.applyJobId), { method: 'POST' });
      } catch (e) { /* ignore */ }
    },
  };
}

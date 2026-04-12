{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data="adminImport()" class="space-y-6 max-w-4xl mx-auto">
  <header>
    <h1 class="text-2xl font-semibold text-stone-900">Import Groups</h1>
    <p class="text-stone-600 mt-1">Upload an export tar to review and import groups into this instance.</p>
  </header>

  <!-- Upload Section -->
  <section aria-label="Upload" class="rounded-lg bg-white border border-stone-200 p-5 space-y-4">
    <h2 class="text-lg font-medium text-stone-800">Upload Archive</h2>
    <div class="space-y-3">
      <label class="block">
        <span class="text-sm text-stone-600">Select a .tar or .tar.gz file</span>
        <input type="file" accept=".tar,.tar.gz,.tgz"
               @change="selectedFile = $event.target.files[0]"
               class="mt-1 block w-full text-sm text-stone-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-stone-100 file:text-stone-700 hover:file:bg-stone-200"
               data-testid="import-file-input">
      </label>
      <div x-show="selectedFile" class="text-sm text-stone-600">
        Selected: <span x-text="selectedFile?.name"></span>
        (<span x-text="humanBytes(selectedFile?.size || 0)"></span>)
      </div>
      <button @click="upload()"
              :disabled="!selectedFile || uploading"
              class="px-4 py-2 bg-emerald-700 text-white rounded hover:bg-emerald-800 disabled:opacity-50 text-sm font-medium"
              data-testid="import-upload-button">
        <span x-show="!uploading">Upload &amp; Parse</span>
        <span x-show="uploading">Uploading...</span>
      </button>
    </div>
  </section>

  <!-- Parse Progress -->
  <section x-show="jobId && !plan" aria-label="Parse progress" class="rounded-lg bg-white border border-stone-200 p-5 space-y-3">
    <h2 class="text-lg font-medium text-stone-800">Parsing Archive</h2>
    <div class="text-sm text-stone-600" data-testid="import-parse-progress">
      <span x-text="job?.phase || 'queued'"></span>
      <span x-show="job?.status === 'failed'" class="text-red-700 font-medium" x-text="'Error: ' + job?.error"></span>
    </div>
    <div x-show="job?.status === 'processing'" class="w-full bg-stone-100 rounded-full h-2">
      <div class="bg-emerald-600 h-2 rounded-full transition-all" :style="'width:' + Math.max(5, (job?.progressPercent || 0)) + '%'"></div>
    </div>
  </section>

  <!-- Error -->
  <div x-show="error" class="rounded-lg bg-red-50 border border-red-200 p-4 text-red-800 text-sm" data-testid="import-error" x-text="error"></div>

  <!-- Review Section (shown after parse completes) -->
  <template x-if="plan">
    <div class="space-y-6">
      <!-- Header Info -->
      <section aria-label="Archive summary" class="rounded-lg bg-white border border-stone-200 p-5 space-y-3">
        <h2 class="text-lg font-medium text-stone-800">Archive Summary</h2>
        <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm" data-testid="import-summary">
          <dt class="text-stone-500">Schema version</dt>
          <dd x-text="plan.schema_version"></dd>
          <dt class="text-stone-500">Source instance</dt>
          <dd x-text="plan.source_instance_id || '(unknown)'"></dd>
          <dt class="text-stone-500">Groups</dt>
          <dd x-text="plan.counts.groups"></dd>
          <dt class="text-stone-500">Resources</dt>
          <dd x-text="plan.counts.resources"></dd>
          <dt class="text-stone-500">Notes</dt>
          <dd x-text="plan.counts.notes"></dd>
          <dt class="text-stone-500">Series</dt>
          <dd x-text="plan.counts.series"></dd>
          <dt class="text-stone-500">Hash collisions (will skip)</dt>
          <dd x-text="plan.conflicts.resource_hash_matches"></dd>
        </dl>
      </section>

      <!-- Missing Hashes Warning -->
      <section x-show="plan.manifest_only_missing_hashes > 0" aria-label="Missing hashes warning"
               class="rounded-lg bg-amber-50 border border-amber-300 p-5 space-y-3" data-testid="import-missing-hashes-warning">
        <h2 class="text-lg font-medium text-amber-800">Missing File Bytes</h2>
        <p class="text-sm text-amber-700">
          This archive was exported without file bytes.
          <strong x-text="plan.manifest_only_missing_hashes"></strong> resources reference hashes
          that do not exist on this instance and cannot be imported.
        </p>
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" x-model="decisions.acknowledge_missing_hashes" class="rounded border-stone-300">
          <span class="text-amber-800">I understand these resources will be skipped</span>
        </label>
      </section>

      <!-- Global Options -->
      <section aria-label="Import options" class="rounded-lg bg-white border border-stone-200 p-5 space-y-4" data-testid="import-options">
        <h2 class="text-lg font-medium text-stone-800">Import Options</h2>
        <div class="grid grid-cols-2 gap-6">
          <div>
            <label class="block text-sm font-medium text-stone-700 mb-1">Parent Group</label>
            <p class="text-xs text-stone-500 mb-2">Imported root groups will be created under this group. Leave empty for top-level.</p>
            <div class="relative">
              <input type="text" x-model="parentGroupQuery" @input.debounce.300ms="searchParentGroups()"
                     placeholder="Search groups..."
                     class="w-full px-3 py-1.5 border border-stone-300 rounded text-sm focus:outline-none focus:border-stone-500">
              <div x-show="parentGroupResults.length > 0" class="absolute z-10 w-full bg-white border border-stone-200 rounded shadow-lg mt-1 max-h-48 overflow-y-auto">
                <button x-show="decisions.parent_group_id" @click="decisions.parent_group_id = null; parentGroupName = ''; parentGroupResults = []"
                        class="w-full text-left px-3 py-2 text-sm text-stone-400 hover:bg-stone-50 border-b">
                  (none - top level)
                </button>
                <template x-for="g in parentGroupResults" :key="g.id">
                  <button @click="decisions.parent_group_id = g.id; parentGroupName = g.name; parentGroupQuery = ''; parentGroupResults = []"
                          class="w-full text-left px-3 py-2 text-sm hover:bg-stone-50"
                          x-text="g.name + ' (#' + g.id + ')'"></button>
                </template>
              </div>
            </div>
            <div x-show="parentGroupName" class="mt-1 text-sm text-stone-600">
              Selected: <span x-text="parentGroupName" class="font-medium"></span>
              <button @click="decisions.parent_group_id = null; parentGroupName = ''" class="text-red-600 text-xs ml-1">(clear)</button>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-stone-700 mb-1" for="collision-policy">Resource Collision Policy</label>
            <p class="text-xs text-stone-500 mb-2">When a resource with the same hash already exists on this instance.</p>
            <select id="collision-policy" x-model="decisions.resource_collision_policy"
                    class="w-full px-3 py-1.5 border border-stone-300 rounded text-sm">
              <option value="skip">Skip (use existing)</option>
              <option value="duplicate">Create duplicate row</option>
            </select>
          </div>
        </div>
      </section>

      <!-- Warnings -->
      <section x-show="plan.warnings?.length > 0" aria-label="Warnings" class="rounded-lg bg-amber-50 border border-amber-200 p-5" data-testid="import-warnings">
        <h2 class="text-lg font-medium text-amber-800 mb-2">Warnings</h2>
        <ul class="list-disc list-inside text-sm text-amber-700 space-y-1">
          <template x-for="w in plan.warnings" :key="w">
            <li x-text="w"></li>
          </template>
        </ul>
      </section>

      <!-- Mapping Panel -->
      <section aria-label="Schema mappings" class="rounded-lg bg-white border border-stone-200 p-5 space-y-4" data-testid="import-mappings">
        <h2 class="text-lg font-medium text-stone-800">Schema Mappings</h2>

        <template x-for="[label, key] in [['Categories', 'categories'], ['Note Types', 'note_types'], ['Resource Categories', 'resource_categories'], ['Tags', 'tags'], ['Group Relation Types', 'group_relation_types']]" :key="key">
          <details x-show="plan.mappings[key]?.length > 0" class="border border-stone-200 rounded">
            <summary class="px-4 py-2 bg-stone-50 cursor-pointer text-sm font-medium text-stone-700" x-text="label + ' (' + (plan.mappings[key]?.length || 0) + ')'"></summary>
            <div class="p-4">
              <table class="w-full text-sm">
                <thead>
                  <tr class="text-left text-stone-500 border-b">
                    <th class="pb-2 w-8"></th>
                    <th class="pb-2">Source</th>
                    <th class="pb-2">Action</th>
                    <th class="pb-2">Destination</th>
                  </tr>
                </thead>
                <tbody>
                  <template x-for="entry in plan.mappings[key]" :key="entry.decision_key">
                    <tr class="border-b border-stone-100" :class="!isMappingIncluded(entry) && 'opacity-40'">
                      <td class="py-2">
                        <input type="checkbox" :checked="isMappingIncluded(entry)"
                               @change="toggleMappingInclude(entry, $event.target.checked)"
                               class="rounded border-stone-300">
                      </td>
                      <td class="py-2">
                        <span x-text="entry.source_key"></span>
                        <span x-show="entry.from_category_name" class="text-xs text-stone-400 block"
                              x-text="'(' + entry.from_category_name + ' \u2192 ' + entry.to_category_name + ')'"></span>
                      </td>
                      <td class="py-2">
                        <span x-show="entry.ambiguous && !getMappingAction(entry)" class="inline-block px-2 py-0.5 bg-amber-100 text-amber-800 text-xs rounded font-medium mb-1">Requires decision</span>
                        <select @change="setMappingAction(entry, $event.target.value)"
                                class="px-2 py-1 border rounded text-xs"
                                :class="entry.ambiguous && !getMappingAction(entry) ? 'border-amber-400 bg-amber-50' : 'border-stone-300'">
                          <option x-show="entry.ambiguous && !getMappingAction(entry)" value="" selected disabled>-- choose --</option>
                          <option value="map" :selected="getMappingAction(entry) === 'map'">Map to existing</option>
                          <option value="create" :selected="getMappingAction(entry) === 'create'">Create new</option>
                        </select>
                      </td>
                      <td class="py-2">
                        <template x-if="getMappingAction(entry) === 'map'">
                          <div>
                            <span x-show="entry.destination_name && !entry.ambiguous && !mappingDestOverride(entry)"
                                  x-text="entry.destination_name + ' (#' + entry.destination_id + ')'"></span>
                            <select x-show="entry.ambiguous && entry.alternatives?.length"
                                    @change="setMappingDest(entry, $event.target.value)"
                                    class="px-2 py-1 border border-stone-300 rounded text-xs">
                              <option value="">-- choose --</option>
                              <template x-for="alt in entry.alternatives" :key="alt.id">
                                <option :value="alt.id" x-text="alt.name + ' (#' + alt.id + ')'"></option>
                              </template>
                            </select>
                            <div x-show="!entry.ambiguous && !entry.destination_id" class="relative">
                              <input type="text" placeholder="Search..."
                                     @input.debounce.300ms="searchMappingDest(entry, $event.target.value)"
                                     class="w-full px-2 py-1 border border-stone-300 rounded text-xs">
                              <div x-show="mappingSearchResults[entry.decision_key]?.length > 0"
                                   class="absolute z-10 w-full bg-white border border-stone-200 rounded shadow-lg mt-1 max-h-32 overflow-y-auto">
                                <template x-for="r in (mappingSearchResults[entry.decision_key] || [])" :key="r.id">
                                  <button @click="setMappingDest(entry, r.id); mappingSearchResults[entry.decision_key] = []"
                                          class="w-full text-left px-2 py-1 text-xs hover:bg-stone-50"
                                          x-text="r.name + ' (#' + r.id + ')'"></button>
                                </template>
                              </div>
                              <div x-show="mappingDestOverride(entry)" class="text-xs text-stone-600 mt-0.5"
                                   x-text="'Mapped to #' + getMappingDestId(entry)"></div>
                            </div>
                          </div>
                        </template>
                        <template x-if="getMappingAction(entry) !== 'map'">
                          <span class="text-stone-400 text-xs">(will create)</span>
                        </template>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </details>
        </template>
      </section>

      <!-- Series Info Panel (read-only, slug-based) -->
      <section x-show="plan.series_info?.length > 0" aria-label="Series reconciliation"
               class="rounded-lg bg-white border border-stone-200 p-5 space-y-3" data-testid="import-series">
        <h2 class="text-lg font-medium text-stone-800">Series</h2>
        <p class="text-sm text-stone-500">Series are matched by slug. No user action needed.</p>
        <table class="w-full text-sm">
          <thead><tr class="text-left text-stone-500 border-b"><th class="pb-2">Name</th><th class="pb-2">Slug</th><th class="pb-2">Action</th></tr></thead>
          <tbody>
            <template x-for="s in plan.series_info" :key="s.export_id">
              <tr class="border-b border-stone-100">
                <td class="py-2" x-text="s.name"></td>
                <td class="py-2 font-mono text-xs" x-text="s.slug || '(no slug)'"></td>
                <td class="py-2">
                  <span x-show="s.action === 'reuse_existing'" class="text-stone-600" x-text="'Reuse: ' + s.dest_name + ' (#' + s.dest_id + ')'"></span>
                  <span x-show="s.action === 'create_new'" class="text-emerald-600">Create new</span>
                  <span x-show="s.action === 'missing'" class="text-amber-600">Missing (F4 was off)</span>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </section>

      <!-- Dangling References (interactive) -->
      <section x-show="plan.dangling_refs?.length > 0" aria-label="Dangling references"
               class="rounded-lg bg-white border border-stone-200 p-5 space-y-3" data-testid="import-dangling">
        <h2 class="text-lg font-medium text-stone-800">Dangling References</h2>
        <p class="text-sm text-stone-500">These references point to entities outside the archive. Choose how to handle each one.</p>
        <table class="w-full text-sm">
          <thead><tr class="text-left text-stone-500 border-b"><th class="pb-2">Kind</th><th class="pb-2">From</th><th class="pb-2">Target</th><th class="pb-2">Action</th></tr></thead>
          <tbody>
            <template x-for="d in plan.dangling_refs" :key="d.id">
              <tr class="border-b border-stone-100">
                <td class="py-2" x-text="d.kind"></td>
                <td class="py-2" x-text="d.from_name || d.from_export_id"></td>
                <td class="py-2" x-text="d.stub_name + ' (source #' + d.stub_source_id + ')'"></td>
                <td class="py-2 space-y-1">
                  <select @change="setDanglingAction(d.id, $event.target.value, null)"
                          class="px-2 py-1 border border-stone-300 rounded text-xs">
                    <option value="drop" selected>Drop relation</option>
                    <option value="map">Map to existing</option>
                  </select>
                  <div x-show="getDanglingAction(d.id) === 'map'" class="relative">
                    <input type="text" placeholder="Search destination..."
                           @input.debounce.300ms="searchDanglingDest(d, $event.target.value)"
                           class="w-full px-2 py-1 border border-stone-300 rounded text-xs">
                    <div x-show="danglingSearchResults[d.id]?.length > 0"
                         class="absolute z-10 w-full bg-white border border-stone-200 rounded shadow-lg mt-1 max-h-32 overflow-y-auto">
                      <template x-for="r in (danglingSearchResults[d.id] || [])" :key="r.id">
                        <button @click="setDanglingDest(d.id, r.id, r.name)"
                                class="w-full text-left px-2 py-1 text-xs hover:bg-stone-50"
                                x-text="r.name + ' (#' + r.id + ')'"></button>
                      </template>
                    </div>
                    <div x-show="getDanglingDest(d.id)" class="text-xs text-stone-600 mt-0.5"
                         x-text="'Mapped to: ' + getDanglingDestName(d.id)"></div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </section>

      <!-- Item Tree (checkboxes for pruning, arbitrary depth) -->
      <section aria-label="Import items" class="rounded-lg bg-white border border-stone-200 p-5 space-y-3" data-testid="import-items">
        <h2 class="text-lg font-medium text-stone-800">Items</h2>
        <p class="text-sm text-stone-500">Uncheck items to exclude them from the import.</p>
        <div class="space-y-0.5">
          <template x-for="fi in flattenedItems" :key="fi.export_id">
            <div class="flex items-center gap-2 text-sm py-1" :style="'padding-left:' + (fi.depth * 1.5) + 'rem'">
              <input type="checkbox" :checked="!isExcluded(fi.export_id)"
                     @change="toggleItem(fi.item, $event.target.checked)"
                     class="rounded border-stone-300">
              <span :class="fi.depth === 0 ? 'font-medium text-stone-800' : 'text-stone-700'" x-text="fi.name"></span>
              <span class="text-xs text-stone-400"
                    x-text="fi.descendant_resource_count + ' resources, ' + fi.descendant_note_count + ' notes'"></span>
            </div>
          </template>
        </div>
      </section>

      <!-- Apply Section -->
      <section aria-label="Apply" class="rounded-lg bg-white border border-stone-200 p-5 space-y-3" data-testid="import-apply">
        <h2 class="text-lg font-medium text-stone-800">Apply Import</h2>
        <p x-show="!applyJobId && !applyResult" class="text-sm text-stone-500 mb-3">Review your decisions above, then apply.</p>

        <!-- Apply button -->
        <div x-show="!applyJobId && !applyResult">
          <button @click="apply()"
                  :disabled="hasIncompleteDecisions() || applying"
                  class="px-4 py-2 bg-emerald-700 text-white rounded hover:bg-emerald-800 disabled:opacity-50 text-sm font-medium"
                  data-testid="import-apply-button">
            <span x-show="!applying">Apply Import</span>
            <span x-show="applying">Submitting...</span>
          </button>
          <p x-show="hasIncompleteDecisions()" class="text-xs text-amber-600 mt-2">
            Resolve all decisions above before applying.
          </p>
        </div>

        <!-- Progress display -->
        <div x-show="applyJobId && applying" class="space-y-2">
          <div class="flex items-center gap-3">
            <div class="text-sm text-stone-600">
              Phase: <span class="font-medium" x-text="applyPhase || 'queued'"></span>
            </div>
            <div x-show="applyJob?.status === 'processing'" class="flex-1 bg-stone-100 rounded-full h-2">
              <div class="bg-emerald-600 h-2 rounded-full transition-all" :style="'width:' + Math.max(5, (applyJob?.progressPercent || 0)) + '%'"></div>
            </div>
          </div>
          <button @click="cancelApply()"
                  class="px-3 py-1.5 bg-stone-200 text-stone-700 rounded hover:bg-stone-300 text-sm">
            Cancel
          </button>
        </div>

        <!-- Success result -->
        <template x-if="applyResult && !error">
          <div class="space-y-3" data-testid="import-apply-result">
            <div class="flex items-center gap-2 text-emerald-700">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
              <span class="font-medium">Import completed</span>
            </div>
            <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
              <dt class="text-stone-500">Groups created</dt>
              <dd x-text="applyResult.created_groups"></dd>
              <dt class="text-stone-500">Resources created</dt>
              <dd x-text="applyResult.created_resources"></dd>
              <dt class="text-stone-500">Notes created</dt>
              <dd x-text="applyResult.created_notes"></dd>
              <dt class="text-stone-500">Skipped (hash match)</dt>
              <dd x-text="applyResult.skipped_by_hash"></dd>
              <dt class="text-stone-500">Skipped (missing bytes)</dt>
              <dd x-text="applyResult.skipped_missing_bytes"></dd>
              <dt class="text-stone-500">Categories created</dt>
              <dd x-text="applyResult.created_categories"></dd>
              <dt class="text-stone-500">Tags created</dt>
              <dd x-text="applyResult.created_tags"></dd>
              <dt class="text-stone-500">Series created</dt>
              <dd x-text="applyResult.created_series"></dd>
              <dt class="text-stone-500">Series reused</dt>
              <dd x-text="applyResult.reused_series"></dd>
              <dt class="text-stone-500">Previews created</dt>
              <dd x-text="applyResult.created_previews"></dd>
              <dt class="text-stone-500">Versions created</dt>
              <dd x-text="applyResult.created_versions"></dd>
            </dl>
            <template x-if="applyResult.warnings?.length > 0">
              <div class="rounded bg-amber-50 border border-amber-200 p-3">
                <p class="text-sm font-medium text-amber-800 mb-1">Warnings</p>
                <ul class="list-disc list-inside text-xs text-amber-700 space-y-0.5">
                  <template x-for="w in applyResult.warnings" :key="w">
                    <li x-text="w"></li>
                  </template>
                </ul>
              </div>
            </template>
            <template x-if="applyResult.created_group_ids?.length > 0">
              <div>
                <p class="text-sm font-medium text-stone-700 mb-1">Created Groups</p>
                <div class="flex flex-wrap gap-1">
                  <template x-for="gid in applyResult.created_group_ids" :key="gid">
                    <a :href="'/group?id=' + gid" class="text-xs text-emerald-700 underline hover:text-emerald-900" x-text="'#' + gid"></a>
                  </template>
                </div>
              </div>
            </template>
          </div>
        </template>

        <!-- Failure with partial results -->
        <template x-if="applyResult && error">
          <div class="space-y-3" data-testid="import-apply-error">
            <div class="flex items-center gap-2 text-red-700">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
              <span class="font-medium">Import failed (partial results below)</span>
            </div>
            <p class="text-sm text-red-600" x-text="error"></p>
            <template x-if="applyResult.created_group_ids?.length > 0">
              <div>
                <p class="text-sm font-medium text-stone-700 mb-1">Created Groups (may need cleanup)</p>
                <div class="flex flex-wrap gap-1">
                  <template x-for="gid in applyResult.created_group_ids" :key="gid">
                    <a :href="'/group?id=' + gid" class="text-xs text-red-700 underline hover:text-red-900" x-text="'#' + gid"></a>
                  </template>
                </div>
              </div>
            </template>
            <template x-if="applyResult.created_resource_ids?.length > 0">
              <div>
                <p class="text-sm font-medium text-stone-700 mb-1">Created Resources (may need cleanup)</p>
                <div class="flex flex-wrap gap-1">
                  <template x-for="rid in applyResult.created_resource_ids" :key="rid">
                    <a :href="'/resource?id=' + rid" class="text-xs text-red-700 underline hover:text-red-900" x-text="'#' + rid"></a>
                  </template>
                </div>
              </div>
            </template>
            <template x-if="applyResult.created_note_ids?.length > 0">
              <div>
                <p class="text-sm font-medium text-stone-700 mb-1">Created Notes (may need cleanup)</p>
                <div class="flex flex-wrap gap-1">
                  <template x-for="nid in applyResult.created_note_ids" :key="nid">
                    <a :href="'/note?id=' + nid" class="text-xs text-red-700 underline hover:text-red-900" x-text="'#' + nid"></a>
                  </template>
                </div>
              </div>
            </template>
          </div>
        </template>
      </section>
    </div>
  </template>
</div>
{% endblock %}

{% extends "/layouts/base.tpl" %}
{% block body %}
<div x-data="adminExport({ preselectedIds: '{{ preselectedGroupIds|default:"" }}' })" class="space-y-6">
  <section aria-label="Group picker" class="space-y-3">
    <h2 class="text-sm font-medium font-mono text-stone-700">Groups</h2>
    <div class="flex flex-wrap gap-2 mb-3" data-testid="export-group-chips">
      <template x-for="g in selectedGroups" :key="g.id">
        <span class="inline-flex items-center gap-1 rounded-full bg-stone-100 px-3 py-1 text-xs">
          <span x-text="g.name"></span>
          <button type="button" @click="removeGroup(g.id)" :aria-label="'Remove ' + g.name">&#xd7;</button>
        </span>
      </template>
    </div>
    <input type="text" x-model="groupQuery" @input.debounce.250ms="searchGroups()"
           placeholder="Search to add groups..." class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded"
           aria-label="Search groups to add" />
    <ul x-show="groupResults.length > 0" class="mt-2 max-h-48 overflow-y-auto border border-stone-200 rounded">
      <template x-for="g in groupResults" :key="g.id">
        <li>
          <button type="button" @click="addGroup(g)" class="w-full text-left px-3 py-1 hover:bg-stone-100">
            <span x-text="g.name"></span>
          </button>
        </li>
      </template>
    </ul>
  </section>

  <section aria-label="Toggles" class="border-t border-stone-200 pt-5 space-y-3">
    <h2 class="text-sm font-medium font-mono text-stone-700">What to include</h2>

    <fieldset class="space-y-2">
      <legend class="text-sm font-medium font-mono text-stone-700">Scope</legend>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="scope.subtree" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include all descendants (S1)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="scope.owned_resources" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include owned resources (S2)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="scope.owned_notes" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include owned notes (S3)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="scope.related_m2m" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include related (m2m) entities (S4)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="scope.group_relations" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include typed group relations (S5)</label>
      <div class="flex items-center gap-2 mt-2" x-show="scope.related_m2m || scope.group_relations">
        <label for="related-depth" class="text-sm text-stone-600">Related depth (0 = off):</label>
        <input type="number" id="related-depth" x-model.number="relatedDepth" min="0" max="10"
               class="w-20 text-sm border-stone-300 rounded focus:ring-1 focus:ring-amber-600 focus:border-amber-600"
               data-testid="export-related-depth">
      </div>
    </fieldset>

    <fieldset class="space-y-2 mt-4">
      <legend class="text-sm font-medium font-mono text-stone-700">Fidelity</legend>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="fidelity.resource_blobs" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include resource file bytes (F1)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="fidelity.resource_versions" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include version history (F2)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="fidelity.resource_previews" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include previews (F3)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="fidelity.resource_series" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Preserve Series membership (F4)</label>
    </fieldset>

    <fieldset class="space-y-2 mt-4">
      <legend class="text-sm font-medium font-mono text-stone-700">Schema definitions</legend>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="schemaDefs.categories_and_types" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include Categories, NoteTypes, ResourceCategories (D1)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="schemaDefs.tags" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include Tag definitions (D2)</label>
      <label class="flex items-center gap-2"><input type="checkbox" x-model="schemaDefs.group_relation_types" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Include GroupRelationType definitions (D3)</label>
    </fieldset>

    <p x-show="!fidelity.resource_blobs" class="mt-3 text-sm text-amber-700">
      Warning: manifest-only exports can only be re-imported into instances that already hold the resource bytes.
    </p>
  </section>

  <section aria-label="Estimate" class="border-t border-stone-200 pt-5 space-y-3">
    <h2 class="text-sm font-medium font-mono text-stone-700">Estimate</h2>
    <button type="button" @click="estimate()" :disabled="selectedGroups.length === 0"
            class="inline-flex justify-center py-2 px-4 border border-stone-300 rounded-md shadow-sm text-sm font-medium font-mono text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50"
            data-testid="export-estimate-button">
      Compute estimate
    </button>
    <div x-show="estimateResult" class="mt-3 text-sm text-stone-700 space-y-1" data-testid="export-estimate-output">
      <div>Groups: <span x-text="estimateResult?.counts?.groups || 0"></span></div>
      <div x-show="(estimateResult?.counts?.shell_groups || 0) > 0">Shell groups: <span x-text="estimateResult?.counts?.shell_groups || 0"></span></div>
      <div>Notes: <span x-text="estimateResult?.counts?.notes || 0"></span></div>
      <div>Resources: <span x-text="estimateResult?.counts?.resources || 0"></span></div>
      <div>Series: <span x-text="estimateResult?.counts?.series || 0"></span></div>
      <div>Unique blobs: <span x-text="estimateResult?.uniqueBlobs || 0"></span></div>
      <div>
        Predicted tar size:
        <span data-testid="export-estimate-size" x-text="humanBytes(estimateResult?.estimatedBytes || 0)"></span>
      </div>

      <div x-show="danglingEntries().length > 0" class="mt-2">
        <div class="font-semibold text-stone-800">Dangling references</div>
        <ul class="list-disc pl-5" data-testid="export-estimate-dangling">
          <template x-for="entry in danglingEntries()" :key="entry.kind">
            <li><span x-text="entry.kind"></span>: <span x-text="entry.count"></span></li>
          </template>
        </ul>
      </div>
      <div x-show="danglingEntries().length === 0" class="mt-2 text-stone-500">
        No dangling references &mdash; every edge stays in scope.
      </div>
    </div>
  </section>

  <section aria-label="Run export" class="border-t border-stone-200 pt-5">
    <button type="button" @click="submit()" :disabled="selectedGroups.length === 0 || jobInProgress"
            class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50"
            data-testid="export-submit-button">
      Start export
    </button>

    <div x-show="job" class="mt-3 space-y-2" data-testid="export-progress-panel">
      <div class="text-sm text-stone-600"><span class="font-semibold">Status:</span> <span x-text="job?.status || ''"></span></div>
      <div class="text-sm text-stone-600"><span class="font-semibold">Phase:</span> <span x-text="job?.phase || 'queued'"></span></div>

      <div class="text-sm text-stone-600" x-show="(job?.phaseTotal || 0) > 0" data-testid="export-phase-counter">
        <span x-text="job?.phaseCount || 0"></span>
        /
        <span x-text="job?.phaseTotal"></span>
        items in current phase
      </div>

      <div class="text-sm text-stone-600" data-testid="export-bytes-counter">
        <span x-text="humanBytes(job?.progress || 0)"></span> written
        <span x-show="(job?.totalSize || 0) > 0">
          / <span x-text="humanBytes(job?.totalSize)"></span> estimated
        </span>
        <span x-show="(job?.progressPercent || -1) >= 0"> (<span x-text="Math.round(job?.progressPercent || 0)"></span>%)</span>
      </div>

      <progress :value="job?.progress || 0" :max="(job?.totalSize || 0) > 0 ? job.totalSize : 100" class="w-full"></progress>

      <div class="flex gap-2">
        <button type="button"
                x-show="canCancel()"
                @click="cancel()"
                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600"
                data-testid="export-cancel-button">
          Cancel
        </button>
        <a x-show="job?.status === 'completed'"
           :href="downloadUrl" download
           class="text-blue-700 underline self-center"
           data-testid="export-download-link">
          Download tar
        </a>
      </div>

      <div x-show="job?.error" class="text-sm text-red-700" data-testid="export-error">
        Error: <span x-text="job?.error"></span>
      </div>
      <div x-show="(job?.warnings || []).length > 0" class="text-sm text-amber-700" data-testid="export-warnings">
        Warnings: <span x-text="job?.warnings?.length || 0"></span>
      </div>
    </div>
  </section>
</div>
{% endblock %}

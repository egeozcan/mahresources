{# Mode switch, change navigation and the counts, shared by both text comparators. #}
<div class="compare-diff-toolbar">
    <div class="compare-segmented-control" role="radiogroup" aria-label="Diff mode"
         @keydown="onRadiogroupKeydown($event, 'mode', ['unified', 'split'])">
        <button @click="mode = 'unified'" role="radio" :aria-checked="mode === 'unified'" aria-label="Unified"
                :tabindex="mode === 'unified' ? 0 : -1"
                class="compare-seg-btn">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="7" y1="8" x2="17" y2="8"/><line x1="7" y1="12" x2="17" y2="12"/><line x1="7" y1="16" x2="13" y2="16"/></svg>
            <span class="compare-seg-label">Unified</span>
        </button>
        <button @click="mode = 'split'" role="radio" :aria-checked="mode === 'split'" aria-label="Side by side"
                :tabindex="mode === 'split' ? 0 : -1"
                class="compare-seg-btn">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="3" width="8" height="18" rx="1"/><rect x="14" y="3" width="8" height="18" rx="1"/></svg>
            <span class="compare-seg-label">Side by side</span>
        </button>
    </div>

    {# Without these, reaching the fourth change in a 4,000-line file means #}
    {# scrolling for it. #}
    <div class="compare-diff-nav" x-show="changeCount > 0" x-cloak>
        <button type="button" class="compare-nav-btn" @click="goToChange(-1)" aria-label="Previous change">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="18 15 12 9 6 15"/></svg>
        </button>
        <span class="compare-nav-count" aria-live="polite"
              x-text="(activeChange >= 0 ? (activeChange + 1) + ' of ' : '') + changeCount + (changeCount === 1 ? ' change' : ' changes')"></span>
        <button type="button" class="compare-nav-btn" @click="goToChange(1)" aria-label="Next change">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="6 9 12 15 18 9"/></svg>
        </button>
    </div>

    <div class="compare-diff-toolbar-end">
        <button type="button" class="compare-nav-btn compare-nav-btn--text" x-show="hasFolds" x-cloak
                @click="expandAll()">Expand all</button>
        <button type="button" class="compare-nav-btn compare-nav-btn--text"
                x-show="!loading && !error && !needsConfirmation && (stats.added || stats.removed)" x-cloak
                @click="copyPatch()" :aria-label="copied ? 'Diff copied' : 'Copy the diff as a patch'">
            <span x-text="copied ? 'Copied' : 'Copy diff'"></span>
        </button>
        <span class="compare-diff-stats" x-show="stats.added || stats.removed" x-cloak>
            <span class="compare-stat-added">+<span x-text="stats.added"></span></span>
            <span class="compare-stat-removed">&minus;<span x-text="stats.removed"></span></span>
            lines
        </span>
    </div>
</div>

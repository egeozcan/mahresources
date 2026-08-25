{# The two diff renderings, shared by the fetched (compareText) and inline #}
{# (compareInlineText) comparators so the two never drift. #}
{# `leftTitle` / `rightTitle` name the columns of the split view. #}

<!-- Size confirmation -->
<div x-show="needsConfirmation" x-cloak class="compare-diff-gate">
    <p>
        These files come to <span class="font-mono" x-text="sizeLabel"></span> together.
        Comparing them loads both in full and may take a while.
    </p>
    <button type="button" class="compare-gate-btn" @click="load()">Compare anyway</button>
</div>

<!-- Loading state -->
<div x-show="loading" class="text-center py-8">
    <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-teal-700 mx-auto"></div>
    <p class="mt-2 text-stone-600">Loading files...</p>
</div>

<!-- Error state -->
<div x-show="error" x-cloak class="bg-red-50 border border-red-200 rounded p-4 text-red-900" x-text="error"></div>

{# Rows are grid divs, not table rows, and `x-if` keeps the hidden view out of #}
{# the DOM entirely. With `x-show` and a table, every line of a large file was #}
{# materialised three times over — once unified, twice split. #}
<template x-if="!loading && !error && !needsConfirmation && mode === 'unified'">
    <div class="compare-diff compare-diff--unified" role="table" aria-label="Unified diff">
        <template x-for="(line, index) in unifiedRows" :key="index">
            <div class="compare-diff-line" role="row"
                 :class="{
                    'is-fold': line.fold,
                    'compare-diff-row--removed': line.type === 'removed',
                    'compare-diff-row--added': line.type === 'added',
                    'compare-diff-row--context': line.type === 'context',
                    'compare-diff-row--active': line.changeIndex !== null && line.changeIndex === activeChange
                 }"
                 :data-change="line.changeIndex"
                 :data-fold="!line.fold ? line.foldId : null">
                {# The gutter carries the row's state: the +/- is decoration and the #}
                {# colour is not read at all, so a diff was announced as line numbers #}
                {# and text with no indication of what had changed. #}
                <span class="compare-diff-num" role="cell" :aria-label="rowLabel(line, null)" x-text="line.leftNum || ''"></span>
                <span class="compare-diff-num compare-diff-num--right" role="cell" x-text="line.rightNum || ''"></span>
                {# One line, deliberately: the container is `white-space: pre`, so a #}
                {# newline between these tags renders as a blank line in the diff. #}
                <span class="compare-diff-content" role="cell"><span class="compare-diff-prefix" :class="{ 'compare-diff-prefix--removed': line.type === 'removed', 'compare-diff-prefix--added': line.type === 'added' }" x-text="line.prefix" aria-hidden="true"></span><template x-for="(seg, si) in line.segments || []" :key="si"><span :class="seg.changed ? (line.type === 'added' ? 'compare-word--added' : 'compare-word--removed') : ''" x-text="seg.text"></span></template><span x-show="!line.segments" x-text="line.content"></span><span class="compare-no-newline" x-show="line.noNewline" x-cloak title="No newline at end of file">\ No newline at end of file</span></span>
                <template x-if="line.fold">
                    <span role="cell" class="compare-fold-cell"><button type="button" class="compare-fold-btn" @click="expandFold(line.foldId, $event)" x-text="'Show ' + line.foldLength + ' unchanged lines'"></button></span>
                </template>
            </div>
        </template>
    </div>
</template>

<template x-if="!loading && !error && !needsConfirmation && mode === 'split'">
    <div class="grid grid-cols-2 gap-0">
        <div class="border-r min-w-0">
            <div class="compare-panel-header--old sticky top-0 z-10">{{ leftTitle }}</div>
            <div class="compare-diff compare-diff--split" role="table" aria-label="{{ leftTitle }}">
                <template x-for="(line, index) in splitLeftRows" :key="index">
                    <div class="compare-diff-line" role="row"
                         :class="{
                            'is-fold': line.fold,
                            'compare-diff-row--removed': line.changed,
                            'compare-diff-row--blank': line.blank,
                            'compare-diff-row--active': line.changeIndex !== null && line.changeIndex === activeChange
                         }"
                         :data-change="line.changeIndex"
                 :data-fold="!line.fold ? line.foldId : null">
                        <span class="compare-diff-num" role="cell" :aria-label="rowLabel(line, 'left')" x-text="line.num || ''"></span>
                        <span class="compare-diff-content" role="cell"><template x-for="(seg, si) in line.segments || []" :key="si"><span :class="seg.changed ? 'compare-word--removed' : ''" x-text="seg.text"></span></template><span x-show="!line.segments" x-text="line.content"></span><span class="compare-no-newline" x-show="line.noNewline" x-cloak title="No newline at end of file">\ No newline at end of file</span></span>
                        <template x-if="line.fold">
                            <span role="cell" class="compare-fold-cell"><button type="button" class="compare-fold-btn" @click="expandFold(line.foldId, $event)" x-text="'Show ' + line.foldLength + ' unchanged lines'"></button></span>
                        </template>
                    </div>
                </template>
            </div>
        </div>
        <div class="min-w-0">
            <div class="compare-panel-header--new sticky top-0 z-10">{{ rightTitle }}</div>
            <div class="compare-diff compare-diff--split" role="table" aria-label="{{ rightTitle }}">
                <template x-for="(line, index) in splitRightRows" :key="index">
                    <div class="compare-diff-line" role="row"
                         :class="{
                            'is-fold': line.fold,
                            'compare-diff-row--added': line.changed,
                            'compare-diff-row--blank': line.blank,
                            'compare-diff-row--active': line.changeIndex !== null && line.changeIndex === activeChange
                         }"
                         :data-change="line.changeIndex"
                 :data-fold="!line.fold ? line.foldId : null">
                        <span class="compare-diff-num" role="cell" :aria-label="rowLabel(line, 'right')" x-text="line.num || ''"></span>
                        <span class="compare-diff-content" role="cell"><template x-for="(seg, si) in line.segments || []" :key="si"><span :class="seg.changed ? 'compare-word--added' : ''" x-text="seg.text"></span></template><span x-show="!line.segments" x-text="line.content"></span><span class="compare-no-newline" x-show="line.noNewline" x-cloak title="No newline at end of file">\ No newline at end of file</span></span>
                        <template x-if="line.fold">
                            <span role="cell" class="compare-fold-cell"><button type="button" class="compare-fold-btn" @click="expandFold(line.foldId, $event)" x-text="'Show ' + line.foldLength + ' unchanged lines'"></button></span>
                        </template>
                    </div>
                </template>
            </div>
        </div>
    </div>
</template>

{# The progress panel of a harness Generation run, shared by the slot hosts and the #}
{# whole-template host. It binds to the host's `run` state, finishRunNow() and      #}
{# cancelRun() (src/components/templateRun.js). Everything the server sent is bound #}
{# with x-text, never x-html: a transcript record is model and entity text.         #}
{# Parameters: name (test id suffix), field_id (unique id prefix).                  #}
{# The live region is always mounted and outside the x-if so its text changes are   #}
{# announced; it says "Turn N started" and the end state, nothing else.             #}
<p class="sr-only" aria-live="polite" aria-atomic="true" data-testid="run-announce-{{ name }}" x-text="run ? run.announcement : ''"></p>
<template x-if="run">
    <section class="mt-2 rounded-md border border-stone-300 bg-white p-2 space-y-2" data-testid="run-panel-{{ name }}" aria-labelledby="{{ field_id }}-runhead">
        <p id="{{ field_id }}-runhead" class="text-xs font-semibold font-mono text-stone-800">
            <span x-text="run.status === 'running' ? 'Generation run in progress' : 'Generation run ended'"></span>
        </p>
        <div class="grid gap-1 text-xs font-mono text-stone-700 sm:grid-cols-2">
            <div>
                <label for="{{ field_id }}-turnmeter" class="block" data-testid="run-turn-label-{{ name }}" x-text="'Turn ' + run.turn + (run.maxTurns ? ' of ' + run.maxTurns : '')"></label>
                <progress id="{{ field_id }}-turnmeter" class="w-full h-2" :max="run.maxTurns || 1" :value="run.turn" data-testid="run-turn-meter-{{ name }}"></progress>
            </div>
            <div>
                <label for="{{ field_id }}-budgetmeter" class="block" data-testid="run-budget-label-{{ name }}" x-text="'Token budget ' + run.budgetUsed + (run.budgetMax ? ' of ' + run.budgetMax : '') + ' used'"></label>
                <progress id="{{ field_id }}-budgetmeter" class="w-full h-2" max="1" :value="run.budgetPercent" data-testid="run-budget-meter-{{ name }}"></progress>
            </div>
        </div>
        <div class="flex flex-wrap gap-2" x-show="run.status === 'running'">
            <button type="button" @click="finishRunNow()" :disabled="run.finishRequested || !run.runId"
                    data-testid="run-finish-{{ name }}"
                    class="px-3 py-1 text-xs font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer whitespace-nowrap">Finish now</button>
            <button type="button" @click="cancelRun()"
                    data-testid="run-cancel-{{ name }}"
                    class="px-3 py-1 text-xs font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer whitespace-nowrap">Cancel</button>
        </div>
        <p class="text-xs font-mono text-stone-600" x-show="run.status === 'running' && run.finishRequested && !run.finishError" data-testid="run-finishing-{{ name }}">The next Turn will be the last.</p>
        <p class="text-xs font-mono text-red-700" x-show="run.finishError" x-text="run.finishError" role="alert"></p>
        <p class="text-xs font-mono text-stone-700" x-show="run.status !== 'running'" data-testid="run-end-{{ name }}"
           x-text="(run.status === 'done' ? 'Finished' : run.status === 'cancelled' ? 'Cancelled' : run.status === 'closed' ? 'The connection closed before the run finished' : 'Failed') + (run.stopReason ? ' (' + run.stopReason + ')' : '') + (run.error ? ': ' + run.error : '') + '.'"></p>
        <details class="rounded border border-stone-200" :open="run.status === 'running'" data-testid="run-turns-{{ name }}">
            <summary class="cursor-pointer px-2 py-1 text-xs font-mono text-stone-800 bg-stone-50" x-text="'Turns (' + run.turns.length + ')'"></summary>
            <ol class="px-3 py-1 space-y-1 text-xs font-mono text-stone-700 list-none">
                <template x-for="t in run.turns" :key="t.turn">
                    <li :data-testid="'run-turn-' + t.turn + '-{{ name }}'">
                        <span class="font-medium" x-text="'Turn ' + t.turn + (t.finalReason ? ' (final: ' + t.finalReason + ')' : '')"></span>
                        <ul class="list-disc pl-5 break-all">
                            <template x-for="(tool, i) in t.tools" :key="i">
                                <li><span x-text="tool.name + (Object.keys(tool.args).length ? ' ' + Object.entries(tool.args).map(([k, v]) => k + '=' + v).join(' ') : '') + ', ' + tool.bytes + ' bytes' + (tool.isError ? ', error' : '')"></span></li>
                            </template>
                            <template x-for="(sub, i) in t.submissions" :key="'s' + i">
                                <li x-text="'submitted ' + sub.field + ': ' + sub.errors + ' error(s), ' + sub.warnings + ' warning(s)'"></li>
                            </template>
                        </ul>
                    </li>
                </template>
            </ol>
        </details>
        <template x-if="run.status !== 'running' && run.records.length">
            <details class="rounded border border-stone-200" data-testid="run-transcript-{{ name }}">
                <summary class="cursor-pointer px-2 py-1 text-xs font-mono text-stone-800 bg-stone-50" x-text="'Transcript (' + run.records.length + ' messages, as sent and received)'"></summary>
                <div class="px-2 py-1 space-y-1">
                    <div x-show="run.legend.length" data-testid="run-reveal-wrap-{{ name }}">
                        <button type="button" @click="toggleRunReveal()" :aria-pressed="run.reveal ? 'true' : 'false'" data-testid="run-reveal-toggle-{{ name }}"
                                class="px-2 py-1 text-xs font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 aria-pressed:bg-sky-100 aria-pressed:border-sky-700 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer">Show real values</button>
                        <span class="ml-1 text-xs font-mono text-stone-600">Real values stay in this browser and are never sent.</span>
                    </div>
                    <template x-for="(rec, i) in run.records" :key="i">
                        <details class="rounded border border-stone-100" :data-testid="'run-record-' + i + '-{{ name }}'">
                            <summary class="cursor-pointer px-2 py-0.5 text-xs font-mono text-stone-800">
                                <span x-text="'Turn ' + rec.turn + ' · ' + rec.label + ' · ' + rec.bytes + ' bytes'"></span>
                                <span x-show="rec.isError" class="text-red-700"> · error</span>
                                <span x-show="rec.notSent" class="text-amber-800" x-text="' · ' + rec.notSentLabel"></span>
                                <span x-show="rec.stubbed" class="text-stone-600" x-text="' · ' + rec.stubbed"></span>
                            </summary>
                            <template x-for="call in rec.calls" :key="call.id">
                                <p class="px-2 text-xs font-mono text-stone-700 break-all"><template x-for="(segment, j) in runSegments('call ' + call.name + ' ' + call.arguments)" :key="j"><span :class="segment.placeholder ? 'rounded-sm bg-sky-100 text-sky-900 outline outline-1 outline-sky-700' : ''" :title="segment.placeholder ? segment.value : null" x-text="segment.text"></span></template></p>
                            </template>
                            <pre tabindex="0" role="region" :aria-label="'Exact text of message ' + (i + 1)" class="mx-2 my-1 max-h-48 overflow-auto rounded border border-stone-200 bg-stone-50 p-2 text-xs font-mono whitespace-pre-wrap break-all focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600" :data-testid="'run-record-text-' + i + '-{{ name }}'"><template x-for="(segment, j) in runSegments(rec.text)" :key="j"><span :class="segment.placeholder ? 'rounded-sm bg-sky-100 text-sky-900 outline outline-1 outline-sky-700' : ''" :title="segment.placeholder ? segment.value : null" x-text="segment.text"></span></template></pre>
                            <template x-if="rec.replayLabel">
                                <details class="mx-2 mb-1">
                                    <summary class="cursor-pointer text-xs font-mono text-stone-700" x-text="rec.replayLabel"></summary>
                                    <pre tabindex="0" role="region" :aria-label="'Provider payload of message ' + (i + 1)" class="max-h-48 overflow-auto rounded border border-stone-200 bg-stone-50 p-2 text-xs font-mono whitespace-pre-wrap break-all focus:outline-hidden focus-visible:ring-2 focus-visible:ring-amber-600"><template x-for="(segment, j) in runSegments(rec.replayText)" :key="j"><span :class="segment.placeholder ? 'rounded-sm bg-sky-100 text-sky-900 outline outline-1 outline-sky-700' : ''" :title="segment.placeholder ? segment.value : null" x-text="segment.text"></span></template></pre>
                                </details>
                            </template>
                        </details>
                    </template>
                </div>
            </details>
        </template>
    </section>
</template>

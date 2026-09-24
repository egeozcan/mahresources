{# The /jobs bulk commands. A Job's commands are its own, so this is one      #}
{# component rather than one declaration per key: it reads each selected      #}
{# Job's advertised commands and offers the ones every selected Job shares.  #}
{# Outcomes are per Job, because a command can still be refused for a Job     #}
{# whose state moved between the offer and the click.                         #}
<div class="px-4" x-data="jobBulkCommands()" data-testid="job-bulk-commands">
    <div class="flex flex-wrap items-center gap-2" role="group" aria-label="Commands for the selected jobs">
        <template x-for="command in commands()" :key="command.key">
            <button type="button" @click="run(command)" :aria-disabled="busy || loading"
                    class="bulk-action-btn inline-flex justify-center py-1.5 px-3 mt-3 border items-center text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 aria-disabled:opacity-50 aria-disabled:cursor-not-allowed"
                    x-text="commandLabel(command)"></button>
        </template>
        <p x-show="loading" role="status" class="mt-3 text-xs text-stone-600">Reading the selected jobs’ commands…</p>
        <p x-show="!loading && !error && commands().length === 0" class="mt-3 text-xs text-stone-600">The selected jobs share no command.</p>
        <p x-show="error" x-text="error" role="alert" class="mt-3 text-xs text-red-800"></p>
    </div>
    <ul x-show="outcomes.length" class="mt-2 space-y-1 text-xs" aria-label="Bulk command outcomes">
        <template x-for="outcome in outcomes" :key="outcome.jobId">
            <li class="flex flex-wrap gap-x-2"><span class="font-mono" x-text="outcome.jobId"></span><span x-text="outcome.message || outcome.code || outcome.status"></span></li>
        </template>
    </ul>
</div>

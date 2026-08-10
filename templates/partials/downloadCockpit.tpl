<div
    x-data="downloadCockpit()"
    x-cloak
    @keydown.escape.window="close()"
    class="download-cockpit"
>
    {# Findings 83 and 102: this was a `fixed bottom-4 right-4 z-40` FAB, and the  #}
    {# bottom-right corner is where real controls live. It won the hit test over   #}
    {# the pagination "Next" link on every paginated page (measured at 1280x900 on #}
    {# /queries: the link spans x 1194-1264 and only x<=1220 reached it), and at   #}
    {# 1280x720 it sat on top of the /logs "After" date picker.                    #}
    {#                                                                            #}
    {# No fixed corner can be safe: the page scrolls underneath a fixed overlay,   #}
    {# so whatever is in that corner at a given scroll offset is covered. The      #}
    {# trigger lives in the header instead — which finding 120's fix makes         #}
    {# genuinely sticky, so it is *more* reachable than the FAB was, not less.     #}
    <button
        @click="toggle($event)"
        data-testid="cockpit-trigger"
        class="cockpit-trigger p-1 flex items-center gap-1 rounded focus:outline-none focus:ring-2 focus:ring-amber-600"
        :class="{ 'cockpit-trigger--busy': hasActiveJobs }"
        title="Jobs (Ctrl+Shift+D / Cmd+Shift+D)"
        :aria-label="isOpen ? 'Close jobs panel' : 'Open jobs panel'"
        :aria-expanded="isOpen.toString()"
        aria-haspopup="dialog"
    >
        <svg aria-hidden="true" class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
        </svg>
        <span x-show="activeCount > 0" x-text="activeCount" class="cockpit-trigger-count"></span>
    </button>

    <!-- Panel overlay and content -->
    {# The trigger above is header chrome; the panel is not. It is teleported into  #}
    {# `.overlays` (position:fixed, z-index:41, root stacking context), which is    #}
    {# where every other overlay in this app lives.                                 #}
    {#                                                                              #}
    {# It used to render in place, carrying a z-index of 60 for a reason that was    #}
    {# true and useless: .header is position:sticky at z-index 40, so it is a        #}
    {# stacking context, and 60 ordered the panel against the settings and account   #}
    {# dropdowns (50) and against nothing else. Raise a dropdown above 60 and the    #}
    {# panel goes back under an aria-modal dialog with no test failing, because the  #}
    {# app's real overlay ordering was in a layer the panel was not part of.         #}
    {# Teleporting it there makes its z-index mean what the others' mean.            #}
    {#                                                                              #}
    {# The numbers above are spelled out rather than written as Tailwind classes on  #}
    {# purpose: index.css declares `@source "./templates/**/*.tpl"`, so a class name #}
    {# in a comment is scanned like any other and emits a rule nothing uses.         #}
    {#                                                                              #}
    {# z-40 is *a* correct value, not the only one: `.overlays` is itself the        #}
    {# stacking context, so any value below the lowest sibling works. The siblings   #}
    {# are lightbox 50, paste-upload 50 and its info toast 50, plugin-action 60,     #}
    {# entity-picker 70 and confirm 50 — so anything <= 49. 40 is chosen because it  #}
    {# reads as "below the modals" and matches the header layer it came from. A      #}
    {# teleport appends last, so a tie would go to the panel; staying under 50       #}
    {# settles it on z-index instead of on DOM order.                                #}
    {#                                                                              #}
    {# The nesting is load-bearing and fails *silently* if reversed. x-if must be    #}
    {# the OUTER template. With x-teleport outside, x-if inserts its clone with      #}
    {# `el.after(clone)` — a sibling of the teleported node, not a descendant — so   #}
    {# Alpine's `_x_teleportBack` hop is never taken, `closestRoot` finds no         #}
    {# [x-data], and x-ref="panel" never registers; focusFirstIn($refs.panel) then   #}
    {# does nothing and no error is raised. They must also never share one           #}
    {# <template>: directiveOrder runs `if` before `teleport` and the teleport       #}
    {# handler is unconditional, so one template produces a second, permanent        #}
    {# overlay. Bare x-teleport, not .prepend/.append — those place the clone        #}
    {# outside `.overlays`, where `.overlays > *` never restores pointer-events.     #}
    <template x-if="isOpen">
        <template x-teleport=".overlays">
        <div class="fixed inset-0 z-40 overflow-hidden" data-testid="cockpit-overlay">
            <!-- Backdrop -->
            <div class="fixed inset-0 bg-black/20" @click="close()"></div>

            <!-- Slide-in panel from right -->
            <div class="fixed right-0 top-0 bottom-0 w-full max-w-md bg-white shadow-xl flex flex-col"
                 x-ref="panel"
                 data-testid="cockpit-panel"
                 role="dialog"
                 aria-modal="true"
                 aria-labelledby="cockpit-panel-heading"
                 tabindex="-1"
                 {# The plan cites this panel as the app's correct example. It is — for   #}
                 {# focus *restore*. Verifying finding 4 measured Tab walking straight    #}
                 {# out of it too (Close button -> body -> "Skip to main content"), so it #}
                 {# was declaring aria-modal without trapping, exactly like the search    #}
                 {# dialog. .noreturn because the component restores focus itself.        #}
                 x-trap.noreturn="isOpen"
                 x-transition:enter="transform transition ease-out duration-300"
                 x-transition:enter-start="translate-x-full"
                 x-transition:enter-end="translate-x-0"
                 x-transition:leave="transform transition ease-in duration-200"
                 x-transition:leave-start="translate-x-0"
                 x-transition:leave-end="translate-x-full">

                <!-- Header -->
                <div class="flex items-center justify-between px-4 py-3 border-b border-stone-200 bg-stone-50">
                    <div class="flex items-center gap-2">
                        <h2 id="cockpit-panel-heading" class="text-lg font-semibold font-mono text-stone-900">Jobs</h2>
                        <span class="w-2 h-2 rounded-full"
                              data-testid="cockpit-connection-status"
                              role="img"
                              :aria-label="'Connection status: ' + connectionStatus"
                              :class="{
                                  'bg-green-500': connectionStatus === 'connected',
                                  'bg-yellow-500 animate-pulse': connectionStatus === 'connecting',
                                  'bg-red-500': connectionStatus === 'disconnected'
                              }"></span>
                    </div>
                    <div class="flex items-center gap-2">
                        {# Finding 40: there was no way to dismiss a finished job. The #}
                        {# panel accumulated 20 permanent rows across sessions.        #}
                        <template x-if="hasFinishedJobs">
                            <button @click="clearCompleted()"
                                    data-testid="cockpit-clear-completed"
                                    class="text-xs text-stone-600 hover:text-stone-900 underline rounded focus:outline-none focus:ring-2 focus:ring-amber-600">
                                Clear completed
                            </button>
                        </template>
                        <button @click="close()" class="p-1 text-stone-500 hover:text-stone-700 rounded focus:outline-none focus:ring-2 focus:ring-amber-600" aria-label="Close jobs panel">
                            <svg aria-hidden="true" class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        </button>
                    </div>
                </div>

                <!-- Job list -->
                <div class="flex-1 overflow-y-auto">
                    <template x-if="displayJobs.length === 0">
                        <div class="flex flex-col items-center justify-center h-full text-stone-500 p-8">
                            <svg aria-hidden="true" class="w-16 h-16 mb-4 text-stone-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                            </svg>
                            <p class="text-center">No jobs in queue</p>
                            <p class="text-sm text-stone-500 mt-1 text-center">Submit remote URLs with "Download in background" to see them here</p>
                        </div>
                    </template>

                    <ul class="divide-y divide-stone-100">
                        <template x-for="job in visibleJobs" :key="job.id">
                            {# data-job-id so a row can be addressed by the job it is: the queue is #}
                            {# process-wide and every test and reader shares it, so "the row that   #}
                            {# says events" is whichever row happens to say that. Review            #}
                            {# remediation finding 5 — three assertions were scoped that way and    #}
                            {# could not fail.                                                      #}
                            <li class="px-4 py-3 hover:bg-stone-50" data-testid="cockpit-job" :data-job-id="job.id">
                                <div class="flex items-start gap-3">
                                    <!-- Status icon -->
                                    <span class="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded-full text-lg"
                                          :class="{
                                              'bg-stone-100': job.status === 'pending',
                                              'bg-amber-100': job.status === 'downloading' || job.status === 'processing' || job.status === 'running',
                                              'bg-green-100': job.status === 'completed',
                                              'bg-red-100': job.status === 'failed' || job.status === 'cancelled',
                                              'bg-yellow-100': job.status === 'paused'
                                          }"
                                          x-text="statusIcons[job.status]"
                                          :aria-label="statusLabels[job.status]"></span>

                                    <!-- Job details -->
                                    <div class="flex-1 min-w-0">
                                        <div class="flex items-center justify-between gap-2">
                                            <p class="text-sm font-medium text-stone-900 truncate"
                                               data-testid="cockpit-job-title"
                                               x-text="getJobTitle(job)"
                                               :title="job._isAction ? job.label : (job.source === 'group-export' ? getJobTitle(job) : job.url)"></p>
                                            <span class="flex-shrink-0 text-xs px-2 py-0.5 rounded-full"
                                                  :class="{
                                                      'bg-stone-100 text-stone-600': job.status === 'pending',
                                                      'bg-amber-100 text-amber-700': job.status === 'downloading' || job.status === 'processing' || job.status === 'running',
                                                      'bg-green-100 text-green-700': job.status === 'completed',
                                                      'bg-red-100 text-red-700': job.status === 'failed' || job.status === 'cancelled',
                                                      'bg-yellow-100 text-yellow-700': job.status === 'paused'
                                                  }"
                                                  x-text="statusLabels[job.status]"></span>
                                        </div>

                                        <!-- URL preview / action subtitle -->
                                        <p class="text-xs text-stone-600 truncate mt-0.5" x-text="getJobSubtitle(job)"></p>

                                        {# Progress readout. Finding 41: this branch was       #}
                                        {# `job.status === 'downloading'` only, so pausing     #}
                                        {# collapsed the row to "Paused <url> Resume" — bytes, #}
                                        {# percent, speed and bar all gone, though the server  #}
                                        {# keeps every one of them.                            #}
                                        {# Finding 113: the bar's name was the bare prefix     #}
                                        {# "Download progress: " and aria-valuenow was pinned  #}
                                        {# at 0 for an unknown total.                          #}
                                        <template x-if="showsProgress(job)">
                                            <div class="mt-2">
                                                <div class="flex justify-between text-xs text-stone-500 mb-1">
                                                    <span x-text="formatProgress(job)"></span>
                                                    {# No speed for a paused job: it is not moving, and the #}
                                                    {# status pill beside the title already says "Paused".   #}
                                                    <span x-text="job.status === 'paused' ? '' : formatSpeed(job)" class="text-amber-700 font-medium"></span>
                                                </div>
                                                <div class="w-full bg-stone-200 rounded-full h-2"
                                                     data-testid="cockpit-progressbar"
                                                     role="progressbar"
                                                     :aria-valuenow="progressValueNow(job)"
                                                     :aria-valuetext="progressValueText(job)"
                                                     aria-valuemin="0"
                                                     aria-valuemax="100"
                                                     :aria-label="progressLabel(job)">
                                                    <div class="h-2 rounded-full transition-all duration-300"
                                                         :class="{ 'animate-pulse': job.totalSize <= 0 && job.status !== 'paused', 'bg-stone-400': job.status === 'paused', 'bg-amber-700': job.status !== 'paused' }"
                                                         :style="'width: ' + (job.totalSize > 0 ? getProgressPercent(job) : 100) + '%'"></div>
                                                </div>
                                            </div>
                                        </template>

                                        <!-- Processing indicator with progress bar -->
                                        <template x-if="job.status === 'processing'">
                                            <div class="mt-2">
                                                <div class="flex justify-between text-xs text-stone-500 mb-1">
                                                    <span>Creating resource...</span>
                                                </div>
                                                <div class="w-full bg-stone-200 rounded-full h-2"
                                                     data-testid="cockpit-progressbar"
                                                     role="progressbar"
                                                     aria-valuenow="100"
                                                     aria-valuemin="0"
                                                     aria-valuemax="100"
                                                     aria-label="Processing: creating resource">
                                                    <div class="bg-amber-700 h-2 rounded-full w-full animate-pulse"></div>
                                                </div>
                                            </div>
                                        </template>

                                        <!-- Plugin action progress -->
                                        <template x-if="job._isAction && job.status === 'running' && job.progress > 0">
                                            <div class="mt-2">
                                                <div class="flex justify-between text-xs text-stone-500 mb-1">
                                                    <span x-text="job.progress + '%'"></span>
                                                </div>
                                                <div class="w-full bg-stone-200 rounded-full h-2"
                                                     data-testid="cockpit-progressbar"
                                                     role="progressbar"
                                                     :aria-valuenow="Math.round(job.progress)"
                                                     aria-valuemin="0"
                                                     aria-valuemax="100"
                                                     :aria-label="'Action progress: ' + job.progress + '%'">
                                                    <div class="bg-purple-600 h-2 rounded-full transition-all duration-300"
                                                         :style="'width: ' + job.progress + '%'"></div>
                                                </div>
                                            </div>
                                        </template>

                                        <!-- Error message -->
                                        <template x-if="job.error">
                                            <p class="mt-1 text-xs text-red-700 truncate" x-text="job.error" :title="job.error"></p>
                                        </template>

                                        <!-- Plugin job message -->
                                        <template x-if="job._isAction && job.message">
                                            <p class="mt-1 text-xs truncate"
                                               :class="job.status === 'failed' ? 'text-red-700' : 'text-stone-500'"
                                               x-text="job.message"></p>
                                        </template>

                                        {# Keyed on the resource id alone, not on `completed`. A cancel that   #}
                                        {# lands after the file was saved reports `cancelled` and keeps the id #}
                                        {# (see DownloadJob.finish), so gating this on the status would hide a #}
                                        {# file that exists — which is the orphaned-file objection the status  #}
                                        {# decision had to answer, not create. If the row names a resource,   #}
                                        {# the reader can reach it.                                           #}
                                        <template x-if="job.resourceId">
                                            <a :href="'/resource?id=' + job.resourceId"
                                               class="mt-1 inline-block text-xs text-amber-700 hover:text-amber-900 hover:underline">
                                                <span x-text="job.status === 'cancelled' ? 'View the file that was saved' : 'View resource'"></span> &rarr;
                                            </a>
                                        </template>

                                        <!-- BH-026: Group export download link on completion -->
                                        <template x-if="job.status === 'completed' && job.source === 'group-export' && job.resultPath">
                                            <a :href="'/v1/exports/' + (job.id) + '/download'"
                                               data-testid="cockpit-job-download"
                                               class="mt-1 inline-block text-xs text-amber-700 hover:text-amber-900 hover:underline">
                                                Download export &darr;
                                            </a>
                                        </template>

                                        <!-- BH-036: retention expiry timestamp for completed group exports -->
                                        <template x-if="job.status === 'completed' && job.source === 'group-export' && job.completedAt && exportRetentionMs > 0">
                                            <p class="mt-0.5 text-xs text-stone-500"
                                               data-testid="cockpit-job-expiry"
                                               :title="'Completed ' + new Date(job.completedAt).toLocaleString() + '; expires ' + new Date(new Date(job.completedAt).getTime() + exportRetentionMs).toLocaleString()">
                                                Expires <span x-text="formatRelativeTime(new Date(job.completedAt).getTime() + exportRetentionMs)"></span>
                                            </p>
                                        </template>

                                        <!-- Plugin action result link -->
                                        <template x-if="job._isAction && job.status === 'completed' && job.result?.redirect">
                                            <a :href="job.result.redirect"
                                               class="mt-1 inline-block text-xs text-purple-600 hover:text-purple-800 hover:underline">
                                                View result &rarr;
                                            </a>
                                        </template>

                                        <!-- Action buttons (download jobs only) -->
                                        <template x-if="!job._isAction">
                                        <div class="mt-2 flex gap-3">
                                            <!-- Pause button (for pending/downloading) -->
                                            <template x-if="canPause(job)">
                                                <button @click="pauseJob(job.id)"
                                                        class="text-xs text-yellow-600 hover:text-yellow-800 focus:outline-none focus:underline">
                                                    Pause
                                                </button>
                                            </template>

                                            <!-- Resume button (for paused) -->
                                            <template x-if="canResume(job)">
                                                <button @click="resumeJob(job.id)"
                                                        class="text-xs text-amber-700 hover:text-amber-800 focus:outline-none focus:underline">
                                                    Resume
                                                </button>
                                            </template>

                                            <!-- Retry button (for failed/cancelled) -->
                                            <template x-if="canRetry(job)">
                                                <button @click="retryJob(job.id)"
                                                        class="text-xs text-amber-700 hover:text-amber-800 focus:outline-none focus:underline">
                                                    Retry
                                                </button>
                                            </template>

                                            {# Cancel. Finding 2: gated on isActive(job), which  #}
                                            {# excludes `paused`, so a paused 1 GB download      #}
                                            {# offered only Resume and the API answered a cancel #}
                                            {# with 404 "already finished".                      #}
                                            <template x-if="canCancel(job)">
                                                <button @click="cancelJob(job.id)"
                                                        class="text-xs text-red-700 hover:text-red-800 focus:outline-none focus:underline">
                                                    Cancel
                                                </button>
                                            </template>
                                        </div>
                                        </template>
                                    </div>
                                </div>
                            </li>
                        </template>
                    </ul>
                </div>

                {# The panel shows the newest N rows; everything else — including every  #}
                {# download that has already been swept out of the in-memory queue — lives #}
                {# on /downloads, which can filter, retry and delete in bulk. The link is  #}
                {# always rendered, not only when rows are hidden: it is the only way to   #}
                {# reach a download from last week, and a control that appears only on a   #}
                {# busy queue is one nobody finds.                                         #}
                <div class="px-4 py-2 border-t border-stone-200 text-xs text-stone-600 flex items-center justify-between gap-2">
                    <span data-testid="cockpit-shown-count"
                          x-text="hiddenJobCount > 0 ? ('Showing ' + visibleJobs.length + ' of ' + displayJobs.length) : ''"></span>
                    <a href="/downloads"
                       data-testid="cockpit-all-downloads"
                       class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700 rounded focus:outline-none focus:ring-2 focus:ring-amber-600">
                        All downloads
                    </a>
                </div>

                <!-- Footer with keyboard hint -->
                <div class="px-4 py-2 bg-stone-50 border-t border-stone-200 text-xs text-stone-600 text-center">
                    Press <kbd class="px-1.5 py-0.5 bg-white rounded border border-stone-300 font-mono">Esc</kbd> to close
                    or <kbd class="px-1.5 py-0.5 bg-white rounded border border-stone-300 font-mono">
                        <span x-text="navigator.platform.indexOf('Mac') > -1 ? '\u2318' : 'Ctrl'"></span>+Shift+D
                    </kbd> to toggle
                </div>
            </div>
        </div>
        </template>
    </template>
</div>

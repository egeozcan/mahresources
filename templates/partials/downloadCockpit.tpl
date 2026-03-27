<div
    x-data="downloadCockpit()"
    x-cloak
    @keydown.escape.window="close()"
    class="download-cockpit"
>
    <!-- Floating trigger button (always visible in corner) -->
    <button
        @click="toggle()"
        class="fixed bottom-4 right-4 z-40 flex items-center gap-2 px-3 py-2 bg-amber-700 text-white rounded-full shadow-lg hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-amber-600 focus:ring-offset-2 transition-all"
        :class="{ 'animate-pulse': hasActiveJobs }"
        title="Jobs (Ctrl+Shift+D / Cmd+Shift+D)"
        aria-label="Open jobs panel"
    >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
        </svg>
        <span x-show="activeCount > 0" x-text="activeCount" class="px-1.5 py-0.5 text-xs bg-white text-amber-700 rounded-full font-bold"></span>
    </button>

    <!-- Panel overlay and content -->
    <template x-if="isOpen">
        <div class="fixed inset-0 z-50 overflow-hidden">
            <!-- Backdrop -->
            <div class="fixed inset-0 bg-black/20" @click="close()"></div>

            <!-- Slide-in panel from right -->
            <div class="fixed right-0 top-0 bottom-0 w-full max-w-md bg-white shadow-xl flex flex-col"
                 x-transition:enter="transform transition ease-out duration-300"
                 x-transition:enter-start="translate-x-full"
                 x-transition:enter-end="translate-x-0"
                 x-transition:leave="transform transition ease-in duration-200"
                 x-transition:leave-start="translate-x-0"
                 x-transition:leave-end="translate-x-full">

                <!-- Header -->
                <div class="flex items-center justify-between px-4 py-3 border-b border-stone-200 bg-stone-50">
                    <div class="flex items-center gap-2">
                        <h2 class="text-lg font-semibold font-mono text-stone-900">Jobs</h2>
                        <span class="w-2 h-2 rounded-full"
                              :class="{
                                  'bg-green-500': connectionStatus === 'connected',
                                  'bg-yellow-500 animate-pulse': connectionStatus === 'connecting',
                                  'bg-red-500': connectionStatus === 'disconnected'
                              }"
                              :title="connectionStatus"></span>
                    </div>
                    <button @click="close()" class="p-1 text-stone-400 hover:text-stone-600 rounded focus:outline-none focus:ring-2 focus:ring-amber-600" aria-label="Close jobs panel">
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                    </button>
                </div>

                <!-- Job list -->
                <div class="flex-1 overflow-y-auto">
                    <template x-if="displayJobs.length === 0">
                        <div class="flex flex-col items-center justify-center h-full text-stone-500 p-8">
                            <svg class="w-16 h-16 mb-4 text-stone-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                            </svg>
                            <p class="text-center">No jobs in queue</p>
                            <p class="text-sm text-stone-400 mt-1 text-center">Submit remote URLs with "Download in background" to see them here</p>
                        </div>
                    </template>

                    <ul class="divide-y divide-stone-100">
                        <template x-for="job in displayJobs" :key="job.id">
                            <li class="px-4 py-3 hover:bg-stone-50">
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
                                               x-text="getJobTitle(job)"
                                               :title="job._isAction ? job.label : job.url"></p>
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
                                        <p class="text-xs text-stone-400 truncate mt-0.5" x-text="getJobSubtitle(job)"></p>

                                        <!-- Progress bar (for downloading) -->
                                        <template x-if="job.status === 'downloading'">
                                            <div class="mt-2">
                                                <div class="flex justify-between text-xs text-stone-500 mb-1">
                                                    <span x-text="formatProgress(job)"></span>
                                                    <span x-text="formatSpeed(job)" class="text-amber-700 font-medium"></span>
                                                </div>
                                                <div class="w-full bg-stone-200 rounded-full h-2">
                                                    <div class="bg-amber-700 h-2 rounded-full transition-all duration-300"
                                                         :class="{ 'animate-pulse': job.totalSize <= 0 }"
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
                                                <div class="w-full bg-stone-200 rounded-full h-2">
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
                                                <div class="w-full bg-stone-200 rounded-full h-2">
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

                                        <!-- Resource link on completion -->
                                        <template x-if="job.status === 'completed' && job.resourceId">
                                            <a :href="'/resource?id=' + job.resourceId"
                                               class="mt-1 inline-block text-xs text-amber-700 hover:text-amber-900 hover:underline">
                                                View resource &rarr;
                                            </a>
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

                                            <!-- Cancel button (for active jobs, not paused) -->
                                            <template x-if="isActive(job)">
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

                <!-- Footer with keyboard hint -->
                <div class="px-4 py-2 bg-stone-50 border-t border-stone-200 text-xs text-stone-500 text-center">
                    Press <kbd class="px-1.5 py-0.5 bg-white rounded border border-stone-300 font-mono">Esc</kbd> to close
                    or <kbd class="px-1.5 py-0.5 bg-white rounded border border-stone-300 font-mono">
                        <span x-text="navigator.platform.indexOf('Mac') > -1 ? '\u2318' : 'Ctrl'"></span>+Shift+D
                    </kbd> to toggle
                </div>
            </div>
        </div>
    </template>
</div>

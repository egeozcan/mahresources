<div
    x-data="downloadCockpit()"
    x-cloak
    @keydown.escape.window="close()"
    class="download-cockpit"
>
    <!-- Floating trigger button (always visible in corner) -->
    <button
        @click="toggle()"
        class="fixed bottom-4 right-4 z-40 flex items-center gap-2 px-3 py-2 bg-indigo-600 text-white rounded-full shadow-lg hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 transition-all"
        :class="{ 'animate-pulse': hasActiveJobs }"
        title="Download Cockpit (Ctrl+Shift+D / Cmd+Shift+D)"
        aria-label="Open download cockpit"
    >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
        </svg>
        <span x-show="activeCount > 0" x-text="activeCount" class="px-1.5 py-0.5 text-xs bg-white text-indigo-600 rounded-full font-bold"></span>
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
                <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200 bg-gray-50">
                    <div class="flex items-center gap-2">
                        <h2 class="text-lg font-semibold text-gray-900">Downloads</h2>
                        <span class="w-2 h-2 rounded-full"
                              :class="{
                                  'bg-green-500': connectionStatus === 'connected',
                                  'bg-yellow-500 animate-pulse': connectionStatus === 'connecting',
                                  'bg-red-500': connectionStatus === 'disconnected'
                              }"
                              :title="connectionStatus"></span>
                    </div>
                    <button @click="close()" class="p-1 text-gray-400 hover:text-gray-600 rounded focus:outline-none focus:ring-2 focus:ring-indigo-500">
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                    </button>
                </div>

                <!-- Job list -->
                <div class="flex-1 overflow-y-auto">
                    <template x-if="jobs.length === 0">
                        <div class="flex flex-col items-center justify-center h-full text-gray-500 p-8">
                            <svg class="w-16 h-16 mb-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                            </svg>
                            <p class="text-center">No downloads in queue</p>
                            <p class="text-sm text-gray-400 mt-1 text-center">Submit remote URLs with "Download in background" to see them here</p>
                        </div>
                    </template>

                    <ul class="divide-y divide-gray-100">
                        <template x-for="job in jobs" :key="job.id">
                            <li class="px-4 py-3 hover:bg-gray-50">
                                <div class="flex items-start gap-3">
                                    <!-- Status icon -->
                                    <span class="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded-full text-lg"
                                          :class="{
                                              'bg-gray-100': job.status === 'pending',
                                              'bg-blue-100': job.status === 'downloading' || job.status === 'processing',
                                              'bg-green-100': job.status === 'completed',
                                              'bg-red-100': job.status === 'failed' || job.status === 'cancelled'
                                          }"
                                          x-text="statusIcons[job.status]"
                                          :aria-label="statusLabels[job.status]"></span>

                                    <!-- Job details -->
                                    <div class="flex-1 min-w-0">
                                        <div class="flex items-center justify-between gap-2">
                                            <p class="text-sm font-medium text-gray-900 truncate"
                                               x-text="getFilename(job.url)"
                                               :title="job.url"></p>
                                            <span class="flex-shrink-0 text-xs px-2 py-0.5 rounded-full"
                                                  :class="{
                                                      'bg-gray-100 text-gray-600': job.status === 'pending',
                                                      'bg-blue-100 text-blue-700': job.status === 'downloading' || job.status === 'processing',
                                                      'bg-green-100 text-green-700': job.status === 'completed',
                                                      'bg-red-100 text-red-700': job.status === 'failed' || job.status === 'cancelled'
                                                  }"
                                                  x-text="statusLabels[job.status]"></span>
                                        </div>

                                        <!-- URL preview -->
                                        <p class="text-xs text-gray-400 truncate mt-0.5" x-text="truncateUrl(job.url, 50)"></p>

                                        <!-- Progress bar (for active downloads) -->
                                        <template x-if="job.status === 'downloading' && job.totalSize > 0">
                                            <div class="mt-2">
                                                <div class="flex justify-between text-xs text-gray-500 mb-1">
                                                    <span x-text="formatProgress(job)"></span>
                                                </div>
                                                <div class="w-full bg-gray-200 rounded-full h-1.5">
                                                    <div class="bg-blue-600 h-1.5 rounded-full transition-all duration-300"
                                                         :style="'width: ' + getProgressPercent(job) + '%'"></div>
                                                </div>
                                            </div>
                                        </template>

                                        <!-- Indeterminate progress for unknown size -->
                                        <template x-if="job.status === 'downloading' && job.totalSize <= 0 && job.progress > 0">
                                            <p class="mt-1 text-xs text-gray-500" x-text="formatProgress(job)"></p>
                                        </template>

                                        <!-- Processing indicator -->
                                        <template x-if="job.status === 'processing'">
                                            <p class="mt-1 text-xs text-blue-600">Creating resource...</p>
                                        </template>

                                        <!-- Error message -->
                                        <template x-if="job.error">
                                            <p class="mt-1 text-xs text-red-600 truncate" x-text="job.error" :title="job.error"></p>
                                        </template>

                                        <!-- Resource link on completion -->
                                        <template x-if="job.status === 'completed' && job.resourceId">
                                            <a :href="'/resource?id=' + job.resourceId"
                                               class="mt-1 inline-block text-xs text-indigo-600 hover:text-indigo-800 hover:underline">
                                                View resource &rarr;
                                            </a>
                                        </template>

                                        <!-- Cancel button -->
                                        <template x-if="isActive(job)">
                                            <button @click="cancelJob(job.id)"
                                                    class="mt-2 text-xs text-red-600 hover:text-red-800 focus:outline-none focus:underline">
                                                Cancel
                                            </button>
                                        </template>
                                    </div>
                                </div>
                            </li>
                        </template>
                    </ul>
                </div>

                <!-- Footer with keyboard hint -->
                <div class="px-4 py-2 bg-gray-50 border-t border-gray-200 text-xs text-gray-500 text-center">
                    Press <kbd class="px-1.5 py-0.5 bg-white rounded border border-gray-300 font-mono">Esc</kbd> to close
                    or <kbd class="px-1.5 py-0.5 bg-white rounded border border-gray-300 font-mono">
                        <span x-text="navigator.platform.indexOf('Mac') > -1 ? '\u2318' : 'Ctrl'"></span>+Shift+D
                    </kbd> to toggle
                </div>
            </div>
        </div>
    </template>
</div>

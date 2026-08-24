{# Client-side bulk upload progress. Rendered inside the create-resource form,  #}
{# driven by the resourceUpload component that owns the form's x-data scope.    #}
{# Only in-flight and failed files get a row: a batch can be hundreds of files, #}
{# and completed ones collapse to a count.                                      #}
<div x-show="phase !== 'idle'" x-cloak
     data-testid="bulk-upload-panel"
     class="mt-6 border border-stone-300 rounded-md p-4 bg-stone-50">

    <h2 class="text-sm font-medium font-mono text-stone-700">Uploading</h2>

    {# Phase transitions are announced through the shared live region in the    #}
    {# component (window.mahAnnounce), so this text is visual only.             #}
    <div class="mt-2 flex justify-between text-xs text-stone-600">
        {# tabindex="-1" so cancel() can park focus here: the Cancel button is  #}
        {# hidden by the phase change that follows it, and focus would otherwise #}
        {# fall to <body>. Not reachable by Tab.                                 #}
        <span data-testid="bulk-upload-summary" tabindex="-1" x-text="formatProgress()"></span>
        <span x-show="phase === 'uploading'" x-text="inFlight.length + ' in flight'"></span>
    </div>

    <div class="w-full bg-stone-200 rounded-full h-2 mt-1"
         data-testid="bulk-upload-progressbar"
         role="progressbar"
         :aria-valuenow="progressValueNow()"
         :aria-valuetext="progressValueText()"
         aria-valuemin="0"
         aria-valuemax="100"
         :aria-label="progressLabel()">
        <div class="h-2 rounded-full transition-all duration-300"
             :class="phase === 'partial' ? 'bg-stone-400' : 'bg-amber-700'"
             :style="'width: ' + progress.percent + '%'"></div>
    </div>

    {# The files actually moving right now — at most `concurrency` rows. #}
    <ul x-show="inFlight.length > 0" class="mt-3 space-y-2" data-testid="bulk-upload-inflight">
        <template x-for="entry in inFlight" :key="entry.name + ':' + entry.size">
            <li>
                <p class="text-xs text-stone-600 truncate" x-text="entry.name"></p>
                <div class="w-full bg-stone-200 rounded-full h-1.5 mt-0.5"
                     role="progressbar"
                     :aria-valuenow="fileValueNow(entry)"
                     aria-valuemin="0"
                     aria-valuemax="100"
                     :aria-label="fileLabel(entry)">
                    <div class="bg-amber-600 h-1.5 rounded-full"
                         :style="'width: ' + (fileValueNow(entry) || 0) + '%'"></div>
                </div>
            </li>
        </template>
    </ul>

    {# Failures persist until the batch is retried or abandoned. A duplicate    #}
    {# carries the id of the resource it collided with, which is the only       #}
    {# useful action left for it.                                               #}
    <div x-show="failed.length > 0" class="mt-4" role="alert" data-testid="bulk-upload-failures">
        <h3 class="text-sm font-medium text-red-800"
            x-text="failed.length + ' file' + (failed.length === 1 ? '' : 's') + ' could not be saved'"></h3>
        <ul class="mt-1 space-y-1">
            <template x-for="entry in failed" :key="'failed:' + entry.name + ':' + entry.size">
                <li class="text-sm text-red-800">
                    <span class="font-mono" x-text="entry.name"></span>
                    <span x-text="' — ' + entry.error"></span>
                    <template x-if="entry.errorResourceId">
                        <a :href="'/resource?id=' + entry.errorResourceId"
                           class="underline hover:text-red-900">Open the existing resource</a>
                    </template>
                </li>
            </template>
        </ul>
    </div>

    {# Aborting an XHR stops the browser reading the response, not the server  #}
    {# processing the request. A file that was mid-flight may well have been    #}
    {# saved, and saying otherwise would be a guess presented as a fact.        #}
    <div x-show="cancelledInFlight.length > 0" class="mt-3" role="status" data-testid="bulk-upload-cancelled">
        <p class="text-sm text-stone-700"
           x-text="cancelledInFlight.length + ' upload' + (cancelledInFlight.length === 1 ? ' was' : 's were') + ' still in progress when you cancelled. The server may have saved ' + (cancelledInFlight.length === 1 ? 'it' : 'them') + ' anyway — check the resource list before uploading again.'"></p>
    </div>

    <p x-show="phase === 'partial'" class="mt-3 text-xs text-stone-600">
        Files that uploaded successfully have already been saved and are not re-sent.
        Choose files again to start a new batch.
    </p>

    <div class="mt-4 flex gap-2">
        <button type="button"
                x-show="phase === 'uploading'"
                @click="cancel()"
                data-testid="bulk-upload-cancel"
                class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            Cancel
        </button>
        <button type="button"
                x-show="phase === 'partial' && retryableIndices.length > 0"
                @click="retryFailed()"
                data-testid="bulk-upload-retry"
                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">
            Retry failed
        </button>
    </div>
</div>

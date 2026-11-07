{# The template generation disclosure: what a generation request will send, shown #}
{# before it is sent. Driven by the `generationDisclosure` store                    #}
{# (src/components/generationDisclosure.js). It follows confirmDialog.tpl (overlay  #}
{# z-index, inert page, store-owned focus return, Escape cancels) with one layout  #}
{# difference and one focus difference: no x-trap. The page is inert and          #}
{# src/utils/containFocus.js keeps Tab inside, also after a mouse blur.            #}
{# The layout: the panel is bounded to the viewport and only the section list      #}
{# scrolls, so the actions stay reachable however long a sample block is. User data #}
{# is bound with x-text only.                                                       #}
<div x-data x-cloak>
    <template x-if="$store.generationDisclosure.isOpen">
        <div class="confirm-dialog-overlay fixed inset-0 flex items-center justify-center bg-black/50 p-2 sm:p-4"
             @keydown.escape.window="$store.generationDisclosure.dismiss()">
            <div x-init="$store.generationDisclosure._mount($el)"
                 class="relative bg-white rounded-xl shadow-2xl w-full max-w-3xl max-h-[92dvh] flex flex-col overflow-hidden ring-1 ring-black/5"
                 role="dialog"
                 aria-modal="true"
                 aria-labelledby="generation-disclosure-title"
                 aria-describedby="generation-disclosure-summary"
                 data-testid="generation-disclosure">
                <div class="px-4 sm:px-5 pt-4 pb-3 border-b border-stone-100 shrink-0">
                    <h2 id="generation-disclosure-title" class="text-lg font-medium font-mono text-stone-900">Review what will be sent</h2>
                    <div id="generation-disclosure-summary" class="mt-1 text-sm text-stone-700 space-y-1">
                        <p>These messages go to <strong class="font-mono" x-text="$store.generationDisclosure.host"></strong>. Nothing is sent until you press Send.</p>
                        <ul class="text-xs font-mono text-stone-600 list-disc pl-5" data-testid="generation-disclosure-profiles">
                            <template x-for="(line, i) in $store.generationDisclosure.profiles" :key="i">
                                <li x-text="line"></li>
                            </template>
                        </ul>
                        <ul class="text-xs font-mono text-stone-600 list-none break-all" aria-label="How it is sent" data-testid="generation-disclosure-transport">
                            <template x-for="(line, i) in $store.generationDisclosure.transport" :key="i">
                                <li x-text="line"></li>
                            </template>
                        </ul>
                        <p class="text-xs text-stone-600" x-show="$store.generationDisclosure.continuationRule" data-testid="generation-disclosure-rule">
                            <span class="font-medium">Later calls:</span> <span x-text="$store.generationDisclosure.continuationRule"></span>
                        </p>
                    </div>
                </div>
                {# The only scrolling part. tabindex makes it reachable and scrollable by keyboard. #}
                <div class="min-h-0 flex-1 overflow-y-auto px-4 sm:px-5 py-3 space-y-2 focus:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-amber-600"
                     tabindex="0"
                     role="region"
                     aria-label="Messages to be sent"
                     data-testid="generation-disclosure-sections">
                    {# Harness runs only: the opt-ins, the Envelope, the Tools and the caps. They live in #}
                    {# this scrolling region, not the fixed summary, so they stay reachable at small   #}
                    {# viewports and high zoom. All of it is bound with x-text.                         #}
                    <template x-if="$store.generationDisclosure.harness">
                        <div class="space-y-2" data-testid="generation-disclosure-harness" :aria-busy="$store.generationDisclosure.redisclosing ? 'true' : 'false'">
                            <p class="text-sm text-stone-800" data-testid="generation-disclosure-envelope" x-text="$store.generationDisclosure.envelope"></p>
                            <fieldset class="rounded border border-stone-200 px-3 py-2 space-y-2">
                                <legend class="px-1 text-sm font-mono font-medium text-stone-800">Options for this run (off by default)</legend>
                                <div class="flex items-start gap-2">
                                    <input type="checkbox" id="generation-optin-mrql" data-testid="generation-optin-mrql"
                                           class="mt-1 h-4 w-4 rounded border-stone-300 text-amber-700 focus:ring-amber-600"
                                           :checked="$store.generationDisclosure.previewMRQL"
                                           @change="$store.generationDisclosure.setOptIn('previewMRQL', $event.target.checked)">
                                    <label for="generation-optin-mrql" class="text-sm text-stone-800">Let previews run <span class="font-mono">[mrql]</span> queries against my data, within the preview limits. Off: each renders an inert placeholder.</label>
                                </div>
                                <div class="flex items-start gap-2">
                                    <input type="checkbox" id="generation-optin-plugins" data-testid="generation-optin-plugins"
                                           class="mt-1 h-4 w-4 rounded border-stone-300 text-amber-700 focus:ring-amber-600"
                                           :checked="$store.generationDisclosure.previewPlugins"
                                           @change="$store.generationDisclosure.setOptIn('previewPlugins', $event.target.checked)">
                                    <label for="generation-optin-plugins" class="text-sm text-stone-800">Let previews run plugin shortcodes, read-only. Plugin shortcodes run the installed plugin's code with its own resource use; the preview limits do not apply inside it. Off: each renders a placeholder.</label>
                                </div>
                                <p class="text-xs text-stone-600" role="status" data-testid="generation-disclosure-redisclosing" x-show="$store.generationDisclosure.redisclosing">Updating what will be sent…</p>
                                <p class="text-xs text-red-700" role="alert" data-testid="generation-disclosure-redisclose-error" x-show="$store.generationDisclosure.redisclosureError" x-text="'Send is disabled: ' + $store.generationDisclosure.redisclosureError"></p>
                            </fieldset>
                            <details class="rounded border border-stone-200" data-testid="generation-disclosure-caps">
                                <summary class="cursor-pointer px-2 py-1 text-sm font-mono text-stone-800 bg-stone-50">Limits of the run</summary>
                                <ul class="px-3 py-1 text-xs font-mono text-stone-700 list-disc pl-6 break-all">
                                    <template x-for="(line, i) in $store.generationDisclosure.capLines" :key="i">
                                        <li x-text="line"></li>
                                    </template>
                                </ul>
                            </details>
                            <details class="rounded border border-stone-200" data-testid="generation-disclosure-tools">
                                <summary class="cursor-pointer px-2 py-1 text-sm font-mono text-stone-800 bg-stone-50">Tools the model can call (<span x-text="$store.generationDisclosure.toolRows.length"></span>)</summary>
                                <div class="px-2 py-1 space-y-1">
                                    <template x-for="tool in $store.generationDisclosure.toolRows" :key="tool.name">
                                        <details class="rounded border border-stone-100">
                                            <summary class="cursor-pointer px-2 py-0.5 text-xs font-mono text-stone-800" x-text="tool.name"></summary>
                                            <p class="px-2 text-xs text-stone-700" x-text="tool.description"></p>
                                            <pre class="px-2 py-1 text-xs font-mono whitespace-pre-wrap break-all text-stone-700" x-text="tool.parameters"></pre>
                                        </details>
                                    </template>
                                </div>
                            </details>
                        </div>
                    </template>
                    <template x-for="(section, i) in $store.generationDisclosure.sections" :key="i">
                        <details class="rounded border border-stone-200" :open="!section.constant" :data-section="section.key">
                            <summary class="cursor-pointer px-2 py-1 text-sm font-mono text-stone-800 bg-stone-50">
                                <span x-text="section.label"></span>
                                <span class="text-xs text-stone-500" x-text="'(' + section.role + ', ' + section.bytes + ' bytes' + (section.constant ? ', same for every request' : '') + ')'"></span>
                            </summary>
                            <pre class="px-2 py-1 text-xs font-mono whitespace-pre-wrap break-all text-stone-800" x-text="section.text"></pre>
                        </details>
                    </template>
                </div>
                <div class="flex flex-wrap justify-end gap-2 px-4 sm:px-5 py-3 bg-stone-50 border-t border-stone-100 shrink-0">
                    <button type="button"
                            autofocus
                            @click="$store.generationDisclosure.dismiss()"
                            data-testid="generation-disclosure-cancel"
                            class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-medium font-mono rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-stone-500">Cancel</button>
                    <button type="button"
                            @click="$store.generationDisclosure.send()"
                            :disabled="!$store.generationDisclosure.canSend"
                            data-testid="generation-disclosure-send"
                            class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed"
                            x-text="'Send to ' + $store.generationDisclosure.host"></button>
                </div>
            </div>
        </div>
    </template>
</div>

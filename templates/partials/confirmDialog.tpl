{# The app's one destructive-confirmation dialog. Driven entirely by the          #}
{# `confirmDialog` store (src/components/confirmDialog.js), which every           #}
{# destructive site reaches through `$store.confirmDialog.ask()` or the           #}
{# `askToConfirm()` helper — there is no per-site markup.                         #}
{#                                                                                 #}
{# role="alertdialog", not "dialog": a confirm interrupts to demand a decision     #}
{# about a consequence, and alertdialog is what tells a screen reader to announce  #}
{# the description immediately rather than waiting to be explored.                 #}
<div x-data x-cloak>
    <template x-if="$store.confirmDialog.isOpen">
        {# No @click.self on the backdrop, deliberately. A stray click dismissing a    #}
        {# destructive confirm is precisely the accident window.confirm cannot have,   #}
        {# and re-earning that property is the point of replacing it. Escape cancels,  #}
        {# and cancelling means the action does not happen.                            #}
        {# `confirm-dialog-overlay` carries the z-index, not a `z-50` utility. Being    #}
        {# the last sibling in `.overlays` orders this dialog only against siblings at  #}
        {# the *same* z-index, and two of them are higher: the plugin action / mass     #}
        {# edit overlay (60) and the raised entity picker (70). A confirm raised from   #}
        {# inside one of those painted underneath it — invisible, while `_applyInert`   #}
        {# froze the modal that covered it, so the reader saw a dead page. It is not    #}
        {# even reported as occluded by a hit test, because an inert element is skipped #}
        {# in hit testing, which is why the e2e that clicks this dialog kept passing.   #}
        <div class="confirm-dialog-overlay fixed inset-0 flex items-center justify-center bg-black/50 p-4"
             @keydown.escape.window="$store.confirmDialog.cancel()">
            {# x-init rather than a directive on the template: the subtree only exists  #}
            {# while open, so this is the moment the dialog has a root to make the rest #}
            {# of the page inert around. x-trap.inert would not do it — it sets         #}
            {# aria-hidden on siblings, which hides the page from assistive tech but    #}
            {# leaves it clickable and focusable.                                       #}
            {#                                                                          #}
            {# .noreturn because the store owns the focus return; see the comment in    #}
            {# src/components/confirmDialog.js and in partials/pluginActionModal.tpl.   #}
            <div x-init="$store.confirmDialog._applyInert($el)"
                 class="relative bg-white rounded-xl shadow-2xl w-full max-w-md overflow-hidden ring-1 ring-black/5"
                 role="alertdialog"
                 aria-modal="true"
                 aria-labelledby="confirm-dialog-title"
                 aria-describedby="confirm-dialog-message"
                 x-trap.noscroll.noreturn="$store.confirmDialog.isOpen">
                <div class="px-5 pt-5 pb-3">
                    <h2 id="confirm-dialog-title" class="text-lg font-medium font-mono text-stone-900"
                        x-text="$store.confirmDialog.title"></h2>
                    <p id="confirm-dialog-message" class="mt-2 text-sm text-stone-700"
                       x-text="$store.confirmDialog.message"></p>
                </div>
                {# Cancel first in the DOM and carrying autofocus, so the destructive    #}
                {# button is never the default target: a reader who confirms by reflex   #}
                {# on Enter must not destroy anything.                                   #}
                <div class="flex justify-end gap-2 px-5 py-4 bg-stone-50 border-t border-stone-100">
                    <button type="button"
                            autofocus
                            @click="$store.confirmDialog.cancel()"
                            class="inline-flex justify-center py-2 px-4 border border-stone-300 shadow-sm text-sm font-medium font-mono rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-stone-500"
                            x-text="$store.confirmDialog.cancelLabel"></button>
                    <button type="button"
                            @click="$store.confirmDialog.accept()"
                            class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600"
                            x-text="$store.confirmDialog.confirmLabel"></button>
                </div>
            </div>
        </div>
    </template>
</div>

{# WS4 finding 90: expanded, this covers the viewport, but it declared no role, #}
{# no aria-modal, no label, no trap and no Escape handler — Tab walked straight  #}
{# onto controls the overlay was hiding.                                          #}
{#                                                                                #}
{# Every attribute below is BOUND, not literal. This element is never created or  #}
{# destroyed — only the `expanded` class changes — and json.tpl is included by    #}
{# displayResource / displayNote / displayNoteText / displayGroup / displayTag, so #}
{# a hardcoded role="dialog" would put a permanent, never-open dialog landmark on #}
{# all of them. Alpine drops an attribute whose bound value is false.             #}
<div
        class="tableContainer flex gap-3 flex-col"
        x-cloak
        :class="expanded && 'expanded'"
        x-data="
            () => ({
                jsonData: {{ jsonData|json }},
                keys: '{{ keys }}' ,
                expanded: false,
                _expandTrigger: null,
                setExpanded(next, event) {
                    if (next && event) this._expandTrigger = event.currentTarget;
                    this.expanded = next;
                    if (next) return;
                    const trigger = this._expandTrigger;
                    this._expandTrigger = null;
                    if (trigger) requestAnimationFrame(() => requestAnimationFrame(() => trigger.focus()));
                },
            })
        "
        :role="expanded && 'dialog'"
        :aria-modal="expanded && 'true'"
        :aria-label="expanded && '{{ metaTitle|default:"Meta Data" }}'"
        {# Not .noscroll — x-effect below already owns the body scroll lock.      #}
        {# .noreturn because the trap's own returnFocus records whatever had      #}
        {# focus ~15ms after activation, which measured as an unrelated sidebar   #}
        {# autocompleter. setExpanded() restores to the control that opened it,   #}
        {# two frames later so the trap has finished releasing.                   #}
        x-trap.noreturn="expanded"
        @keydown.escape.stop="expanded && setExpanded(false)"
        x-effect="document.body.classList.toggle('overflow-hidden', expanded)"
        @click="(e) => {if(!e.shiftKey) return; setExpanded(!expanded, e); e.preventDefault();}"
>
    <div class="metaHeader">
        <h2 class="sidebar-group-title">{{ metaTitle|default:"Meta Data" }}</h2>
        <button
                x-show="jsonData && (Array.isArray(jsonData) ? jsonData.length : Object.keys(jsonData).length)"
                class="metaExpandBtn"
                @click.prevent="setExpanded(!expanded, $event)"
                :aria-label="expanded ? 'Minimize metadata view' : 'Expand metadata to fullscreen'"
                :aria-expanded="expanded.toString()"
        >
            <template x-if="!expanded">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M2 6V2h4M14 6V2h-4M2 10v4h4M14 10v4h-4"/></svg>
            </template>
            <template x-if="expanded">
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M6 2H2v4M10 2h4v4M6 14H2v-4M10 14h4v-4"/></svg>
            </template>
            <span x-text="expanded ? 'Minimize' : 'Expand'"></span>
        </button>
    </div>
    <div class="metaTableInner" x-init="$el.appendChild(renderJsonTable(keys ? pick(jsonData, ...keys.split(',')) : jsonData))"></div>
</div>

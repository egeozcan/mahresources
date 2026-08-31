{# Mass Edit: one modal over several ops in one transaction.                  #}
{# Included once from layouts/base.tpl for the three list pages (the context  #}
{# providers publish massEditEntity beside totalCount). The opener buttons    #}
{# live in the bulk editor partials and dispatch `mass-edit-open`.            #}
{#                                                                            #}
{# Focus is owned by the component through focusedElement()/restoreFocus, the #}
{# pattern pluginActionModal.tpl settled on: x-trap's own restore points at   #}
{# whatever had focus when it armed, which for a control the opener has since #}
{# hidden is nothing.                                                         #}
{% if massEditEntity %}
<div x-data="massEditModal()" x-cloak>
    <template x-if="isOpen">
        <div class="plugin-action-overlay" @click.self="close()" @keydown.escape.window="isOpen && close()">
            <div class="plugin-action-modal" role="dialog" aria-modal="true" aria-labelledby="mass-edit-modal-title" x-trap.noreturn.noscroll="isOpen">
                <header class="plugin-action-modal-header">
                    <h3 id="mass-edit-modal-title" class="plugin-action-modal-title">Mass edit <span x-text="noun()"></span></h3>
                    <button @click="close()" class="plugin-action-modal-close" aria-label="Close">&times;</button>
                </header>

                <form @submit.prevent="submit()" x-ref="form">
                    <template x-if="error">
                        <div class="plugin-action-modal-error" role="alert" x-text="error"></div>
                    </template>

                    <div class="mass-edit-body">
                    <fieldset class="mass-edit-section mass-edit-section--first">
                        <legend class="mass-edit-legend">Which <span x-text="noun()"></span>?</legend>
                        <label class="flex items-center gap-2 text-sm">
                            <input type="radio" name="massEditTarget" value="ids" x-model="target">
                            <span>Selected items (<span x-text="selectedIds.length"></span>)</span>
                        </label>
                        <label class="flex items-center gap-2 text-sm mt-1">
                            <input type="radio" name="massEditTarget" value="filter" x-model="target">
                            <span>Every <span x-text="noun()"></span> matching the current filter (<span x-text="totalCount"></span> on this list)</span>
                        </label>
                    </fieldset>

                    {% if massEditEntity == 'resource' or massEditEntity == 'note' or massEditEntity == 'group' %}
                    <fieldset class="mass-edit-section">
                        <div class="mass-edit-row">
                            <span class="mass-edit-legend">Tags</span>
                            <select aria-label="Tags operation" name="TagsOp" class="mass-edit-select">
                                <option value="">— leave tags unchanged —</option>
                                <option value="add">Add</option>
                                <option value="remove">Remove</option>
                                <option value="replace">Replace with</option>
                            </select>
                        </div>
                        {% include "/partials/form/autocompleter.tpl" with profile='tag' usage=massEditEntity elName='TagIds' title='Tags to apply' onChange='onTagsChange' id=getNextId("massedit_tags") %}
                    </fieldset>
                    {% endif %}

                    {% if massEditEntity == 'resource' or massEditEntity == 'note' %}
                    <fieldset class="mass-edit-section">
                        <div class="mass-edit-row">
                            <span class="mass-edit-legend">Related groups</span>
                            <select aria-label="Related groups operation" name="GroupsOp" class="mass-edit-select">
                                <option value="">— leave groups unchanged —</option>
                                <option value="add">Add</option>
                                <option value="remove">Remove</option>
                                <option value="replace">Replace with</option>
                            </select>
                        </div>
                        {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='group' categoryDecoration=true elName='GroupIds' title='Groups to apply' onChange='onGroupsChange' id=getNextId("massedit_groups") %}
                    </fieldset>
                    {% endif %}

                    {% if massEditEntity == 'resource' or massEditEntity == 'group' %}
                    <fieldset class="mass-edit-section">
                        <div class="mass-edit-row">
                            <span class="mass-edit-legend">Related notes</span>
                            <select aria-label="Related notes operation" name="NotesOp" class="mass-edit-select">
                                <option value="">— leave notes unchanged —</option>
                                <option value="add">Add</option>
                                <option value="remove">Remove</option>
                                <option value="replace">Replace with</option>
                            </select>
                        </div>
                        {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='note' elName='NoteIds' title='Notes to apply' onChange='onNotesChange' id=getNextId("massedit_notes") %}
                    </fieldset>
                    {% endif %}

                    {% if massEditEntity == 'note' or massEditEntity == 'group' %}
                    <fieldset class="mass-edit-section">
                        <div class="mass-edit-row">
                            <span class="mass-edit-legend">Related resources</span>
                            <select aria-label="Related resources operation" name="ResourcesOp" class="mass-edit-select">
                                <option value="">— leave resources unchanged —</option>
                                <option value="add">Add</option>
                                <option value="remove">Remove</option>
                                <option value="replace">Replace with</option>
                            </select>
                        </div>
                        {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='resource' elName='ResourceIds' title='Resources to apply' onChange='onResourcesChange' id=getNextId("massedit_resources") %}
                    </fieldset>
                    {% endif %}

                    {% if massEditEntity == 'group' %}
                    <fieldset class="mass-edit-section">
                        <div class="mass-edit-row">
                            <span class="mass-edit-legend">Related groups</span>
                            <select aria-label="Related groups operation" name="RelatedGroupsOp" class="mass-edit-select">
                                <option value="">— leave related groups unchanged —</option>
                                <option value="add">Add</option>
                                <option value="remove">Remove</option>
                                <option value="replace">Replace with</option>
                            </select>
                        </div>
                        {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='group' categoryDecoration=true elName='RelatedGroupIds' title='Groups to apply' onChange='onRelatedGroupsChange' id=getNextId("massedit_related_groups") %}
                    </fieldset>
                    {% endif %}

                    <fieldset class="mass-edit-section">
                        <legend class="mass-edit-legend">Owner{% if massEditEntity == 'group' %} (the parent group){% endif %}</legend>
                        <label class="flex items-center gap-2 text-sm">
                            <input type="radio" name="massEditOwner" value="set" x-model="ownerMode">
                            <span>Set owner</span>
                        </label>
                        <div x-show="ownerMode === 'set'" class="mt-1">
                            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='group' max=1 elName='OwnerId' title='Owner' id=getNextId("massedit_owner") %}
                        </div>
                        <label class="flex items-center gap-2 text-sm mt-1">
                            <input type="radio" name="massEditOwner" value="clear" x-model="ownerMode">
                            <span>Clear owner</span>
                        </label>
                    </fieldset>

                    <fieldset class="mass-edit-section">
                        <div class="mass-edit-row">
                            <span class="mass-edit-legend">Meta</span>
                            <select aria-label="Meta operation" name="MetaOp" class="mass-edit-select">
                                <option value="">— leave meta unchanged —</option>
                                <option value="merge">Merge</option>
                                <option value="replace">Replace with</option>
                                <option value="removeKeys">Remove keys</option>
                            </select>
                        </div>
                        <div x-show="fdMetaVisible()" class="mt-1">
                            {% include "/partials/form/freeFields.tpl" with name="Meta" url=massEditMetaKeysUrl jsonOutput="true" id=getNextId("massedit_meta") %}
                        </div>
                        <div x-show="metaKeysVisible()" class="mt-1">
                            <datalist id="mass-edit-meta-keys-list"></datalist>
                            <template x-for="(row, index) in metaKeyRows" :key="index">
                                <input type="text" x-model="metaKeyRows[index]" list="mass-edit-meta-keys-list"
                                       :aria-label="'Meta key ' + (index + 1) + ' to remove'"
                                       placeholder="Key to remove"
                                       class="w-full focus:ring-1 focus:ring-amber-600 focus:border-amber-600 text-sm border-stone-300 rounded mt-1">
                            </template>
                            <button type="button" class="text-sm text-teal-700 mt-1" @click="metaKeyRows.push('')">Add another key</button>
                        </div>
                    </fieldset>

                    </div>

                    <div class="plugin-action-modal-actions">
                        <button type="button" @click="close()" class="btn btn-secondary">Cancel</button>
                        <button type="submit" :disabled="submitting" class="btn btn-primary">
                            <span x-show="!submitting">Apply</span>
                            <span x-show="submitting">Applying...</span>
                        </button>
                    </div>
                </form>
            </div>
        </div>
    </template>
</div>
{% endif %}

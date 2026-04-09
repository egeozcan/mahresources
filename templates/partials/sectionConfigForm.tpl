<fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-4"
         x-data="sectionConfigForm('{{ sectionConfigValue.String|escapejs }}', '{{ sectionConfigType }}')">
    <legend class="text-base font-semibold font-mono text-stone-800 px-2">Section Visibility</legend>

    <input type="hidden" name="SectionConfig" :value="JSON.stringify(config)">

    <p class="text-sm text-stone-600">
        Control which sections are visible on detail pages for
        <template x-if="type === 'group'"><span>groups</span></template>
        <template x-if="type === 'resource'"><span>resources</span></template>
        <template x-if="type === 'note'"><span>notes</span></template>
        in this category.
    </p>

    {# ── Main Content ── #}
    <div class="space-y-2">
        <h3 class="text-sm font-semibold font-mono text-stone-700">Main Content</h3>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
            <template x-if="type !== 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.description"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Description
                </label>
            </template>
            <template x-if="type === 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.content"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Content (description &amp; blocks)
                </label>
            </template>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.metaSchemaDisplay"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Schema Display
            </label>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.timestamps"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Timestamps
            </label>
            <template x-if="type !== 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.breadcrumb"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Breadcrumb
                </label>
            </template>
        </div>
    </div>

    {# ── Group: Collapsible Sections ── #}
    <template x-if="type === 'group'">
        <div class="space-y-4">
            {# Own Entities #}
            <div class="space-y-2 rounded-md border border-stone-200 bg-white p-3">
                <div class="flex items-center gap-3">
                    <h3 class="text-sm font-semibold font-mono text-stone-700">Own Entities</h3>
                    <select x-model="config.ownEntities.state" aria-label="Own Entities visibility"
                            class="text-sm rounded border-stone-300 bg-stone-50 text-stone-700 focus:ring-amber-600 focus:border-amber-600">
                        <option value="default">Default</option>
                        <option value="open">Open</option>
                        <option value="collapsed">Collapsed</option>
                        <option value="off">Off</option>
                    </select>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-2"
                     :class="config.ownEntities.state === 'off' && 'opacity-50 pointer-events-none'">
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.ownEntities.ownNotes"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Own Notes
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.ownEntities.ownGroups"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Own Groups
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.ownEntities.ownResources"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Own Resources
                    </label>
                </div>
            </div>

            {# Related Entities #}
            <div class="space-y-2 rounded-md border border-stone-200 bg-white p-3">
                <div class="flex items-center gap-3">
                    <h3 class="text-sm font-semibold font-mono text-stone-700">Related Entities</h3>
                    <select x-model="config.relatedEntities.state" aria-label="Related Entities visibility"
                            class="text-sm rounded border-stone-300 bg-stone-50 text-stone-700 focus:ring-amber-600 focus:border-amber-600">
                        <option value="default">Default</option>
                        <option value="open">Open</option>
                        <option value="collapsed">Collapsed</option>
                        <option value="off">Off</option>
                    </select>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-2"
                     :class="config.relatedEntities.state === 'off' && 'opacity-50 pointer-events-none'">
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.relatedEntities.relatedGroups"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Related Groups
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.relatedEntities.relatedResources"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Related Resources
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.relatedEntities.relatedNotes"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Related Notes
                    </label>
                </div>
            </div>

            {# Relations #}
            <div class="space-y-2 rounded-md border border-stone-200 bg-white p-3">
                <div class="flex items-center gap-3">
                    <h3 class="text-sm font-semibold font-mono text-stone-700">Relations</h3>
                    <select x-model="config.relations.state" aria-label="Relations visibility"
                            class="text-sm rounded border-stone-300 bg-stone-50 text-stone-700 focus:ring-amber-600 focus:border-amber-600">
                        <option value="default">Default</option>
                        <option value="open">Open</option>
                        <option value="collapsed">Collapsed</option>
                        <option value="off">Off</option>
                    </select>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-2"
                     :class="config.relations.state === 'off' && 'opacity-50 pointer-events-none'">
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.relations.forwardRelations"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Forward Relations
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.relations.reverseRelations"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Reverse Relations
                    </label>
                </div>
            </div>
        </div>
    </template>

    {# ── Resource: Collapsible Sections ── #}
    <template x-if="type === 'resource'">
        <div class="space-y-4">
            {# Technical Details #}
            <div class="space-y-2 rounded-md border border-stone-200 bg-white p-3">
                <div class="flex items-center gap-3">
                    <h3 class="text-sm font-semibold font-mono text-stone-700">Technical Details</h3>
                    <select x-model="config.technicalDetails.state" aria-label="Technical Details visibility"
                            class="text-sm rounded border-stone-300 bg-stone-50 text-stone-700 focus:ring-amber-600 focus:border-amber-600">
                        <option value="default">Default</option>
                        <option value="open">Open</option>
                        <option value="collapsed">Collapsed</option>
                        <option value="off">Off</option>
                    </select>
                </div>
            </div>

            {# Associations #}
            <div class="space-y-2">
                <h3 class="text-sm font-semibold font-mono text-stone-700">Associations</h3>
                <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.notes"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Notes
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.groups"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Groups
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.series"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Series
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.similarResources"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Similar Resources
                    </label>
                    <label class="flex items-center gap-2 text-sm text-stone-700">
                        <input type="checkbox" x-model="config.versions"
                               class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        Versions
                    </label>
                </div>
            </div>
        </div>
    </template>

    {# ── Note: Associations ── #}
    <template x-if="type === 'note'">
        <div class="space-y-2">
            <h3 class="text-sm font-semibold font-mono text-stone-700">Associations</h3>
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.groups"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Groups
                </label>
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.resources"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Resources
                </label>
            </div>
        </div>
    </template>

    {# ── Sidebar ── #}
    <div class="space-y-2">
        <h3 class="text-sm font-semibold font-mono text-stone-700">Sidebar</h3>
        <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.tags"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Tags
            </label>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.metaJson"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Meta JSON
            </label>
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.owner"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Owner
            </label>

            {# Group-specific sidebar items #}
            <template x-if="type === 'group'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.merge"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Merge
                </label>
            </template>
            <template x-if="type === 'group'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.clone"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Clone
                </label>
            </template>
            <template x-if="type === 'group'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.treeLink"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Tree Link
                </label>
            </template>

            {# Resource-specific sidebar items #}
            <template x-if="type === 'resource'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.metadataGrid"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Metadata Grid
                </label>
            </template>
            <template x-if="type === 'resource'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.previewImage"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Preview Image
                </label>
            </template>
            <template x-if="type === 'resource'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.imageOperations"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Image Operations
                </label>
            </template>
            <template x-if="type === 'resource'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.categoryLink"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Category Link
                </label>
            </template>
            <template x-if="type === 'resource'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.fileSize"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    File Size
                </label>
            </template>

            {# Note-specific sidebar items #}
            <template x-if="type === 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.noteTypeLink"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Note Type Link
                </label>
            </template>
            <template x-if="type === 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.share"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Share &amp; Actions
                </label>
            </template>
        </div>
    </div>
</fieldset>

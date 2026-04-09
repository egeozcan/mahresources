export function sectionConfigForm(initialJson, type) {
    const groupDefaults = {
        ownEntities: { state: 'default', ownNotes: true, ownGroups: true, ownResources: true },
        relatedEntities: { state: 'default', relatedGroups: true, relatedResources: true, relatedNotes: true },
        relations: { state: 'default', forwardRelations: true, reverseRelations: true },
        tags: true, timestamps: true, metaJson: true, merge: true, clone: true, treeLink: true,
        owner: true, breadcrumb: true, description: true, metaSchemaDisplay: true,
    };
    const resourceDefaults = {
        technicalDetails: { state: 'default' },
        metadataGrid: true, timestamps: true, notes: true, groups: true, series: true,
        similarResources: true, versions: true, tags: true, metaJson: true,
        previewImage: true, imageOperations: true, categoryLink: true,
        fileSize: true, owner: true, breadcrumb: true, description: true, metaSchemaDisplay: true,
    };
    const noteDefaults = {
        content: true, groups: true, resources: true, timestamps: true,
        tags: true, metaJson: true, metaSchemaDisplay: true,
        owner: true, noteTypeLink: true, share: true,
    };
    const defaults = type === 'group' ? groupDefaults : type === 'note' ? noteDefaults : resourceDefaults;
    let parsed = {};
    try { parsed = initialJson ? JSON.parse(initialJson) || {} : {}; } catch { parsed = {}; }
    // Deep merge: defaults first, then parsed overrides
    const config = JSON.parse(JSON.stringify(defaults));
    for (const [k, v] of Object.entries(parsed)) {
        if (typeof v === 'object' && v !== null && typeof config[k] === 'object') {
            Object.assign(config[k], v);
        } else {
            config[k] = v;
        }
    }
    return { config, type };
}

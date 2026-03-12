/**
 * renderMentions - Replace @[type:id:name] markers in text with HTML links/cards.
 *
 * Mirrors the server-side Go render_mentions filter:
 * - Standalone resource mentions (only content on their line) -> card with thumbnail
 * - Inline resource mentions -> small inline thumbnail + name link
 * - Other types -> badge link
 *
 * @param {string} text - Text containing @[type:id:name] markers
 * @returns {string} Text with markers replaced by HTML
 */
export function renderMentions(text) {
    if (!text) return text;

    const mentionPattern = /@\[([a-zA-Z]+):(\d+):([^\]]+)\]/g;

    // Entity type to URL path mapping
    const entityPaths = {
        resource: '/resource',
        note: '/note',
        group: '/group',
        tag: '/tag',
    };

    return text.replace(mentionPattern, (match, type, id, name) => {
        const lowerType = type.toLowerCase();
        const numId = parseInt(id, 10);
        if (!numId) return match;

        const escapedName = escapeHTML(name);
        const path = entityPaths[lowerType] || ('/' + lowerType);

        if (lowerType === 'resource') {
            // Check if this mention is the only content on its line
            if (isMentionOnlyOnLine(text, match)) {
                return `<a href="${path}?id=${numId}" class="mention-card">` +
                    `<img src="/v1/resource/preview?id=${numId}" alt="${escapedName}" class="mention-card-thumb">` +
                    `<span class="mention-card-name">${escapedName}</span></a>`;
            } else {
                return `<a href="${path}?id=${numId}" class="mention-inline">` +
                    `<img src="/v1/resource/preview?id=${numId}" alt="" class="mention-inline-thumb">` +
                    `${escapedName}</a>`;
            }
        } else {
            return `<a href="${path}?id=${numId}" class="mention-badge mention-${lowerType}">${escapedName}</a>`;
        }
    });
}

/**
 * Check if a mention marker is the only non-whitespace content on its line.
 * @param {string} fullText - The full text
 * @param {string} marker - The exact marker string to check
 * @returns {boolean}
 */
function isMentionOnlyOnLine(fullText, marker) {
    const lines = fullText.split('\n');
    for (const line of lines) {
        if (line.includes(marker)) {
            if (line.trim() === marker) {
                return true;
            }
        }
    }
    return false;
}

/**
 * Escape HTML special characters.
 * @param {string} str
 * @returns {string}
 */
function escapeHTML(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

/**
 * renderMentions - Replace @[type:id:name] markers in text with HTML links/cards.
 *
 * Mirrors the server-side Go render_mentions filter:
 * - Standalone resource mentions (only content on their line) -> card with thumbnail
 * - Inline resource mentions -> small inline thumbnail + name link
 * - Other types -> badge link
 *
 * Each occurrence is checked individually for standalone-vs-inline context.
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

    // Use replacer callback — regex .replace processes left-to-right, one at a time,
    // and the offset parameter gives us the position of this specific occurrence.
    return text.replace(mentionPattern, (match, type, id, name, offset) => {
        const lowerType = type.toLowerCase();
        const numId = parseInt(id, 10);
        if (!numId) return match;

        const escapedName = escapeHTML(name);
        const path = entityPaths[lowerType] || ('/' + lowerType);

        if (lowerType === 'resource') {
            if (isMentionStandaloneAt(text, offset, match)) {
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
 * Check if a mention at a specific position is the only non-whitespace,
 * non-HTML content on its line.
 * @param {string} fullText - The full text
 * @param {number} pos - The character offset of the marker in fullText
 * @param {string} marker - The exact marker string
 * @returns {boolean}
 */
function isMentionStandaloneAt(fullText, pos, marker) {
    // Find the line containing this position
    let lineStart = fullText.lastIndexOf('\n', pos - 1);
    lineStart = lineStart === -1 ? 0 : lineStart + 1;

    let lineEnd = fullText.indexOf('\n', pos + marker.length);
    if (lineEnd === -1) lineEnd = fullText.length;

    const line = fullText.substring(lineStart, lineEnd);
    const trimmed = line.trim();
    if (trimmed === marker) return true;

    // After markdown, the line may be wrapped in HTML tags like <p>...</p>
    const stripped = trimmed.replace(/<[^>]*>/g, '').trim();
    return stripped === marker;
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

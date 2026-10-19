{# Every text block in `blocks`, rendered the way description.tpl renders a Description (markdown, mentions, shortcodes against `note`), one <template> per block for blockEditor.js to read. #}
{# Used on the note page and by GET /v1/note/renderedTextBlocks, so both render through the same tag. #}
{% for block in blocks %}{% if block.Type == "text" %}<template data-rendered-text-block="{{ block.ID }}">{% autoescape off %}{% process_shortcodes block.TextContent()|markdown2|render_mentions note %}{% endautoescape %}</template>{% endif %}{% endfor %}

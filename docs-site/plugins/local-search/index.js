const fs = require('fs');
const path = require('path');

const PLUGIN_NAME = 'local-search';
const MAX_CONTENT_LENGTH = 14000;

function resolveSourcePath(siteDir, source) {
  if (source.startsWith('@site/')) {
    return path.join(siteDir, source.slice('@site/'.length));
  }

  return source;
}

function stripFrontMatter(markdown) {
  if (!markdown.startsWith('---')) {
    return markdown;
  }

  const end = markdown.indexOf('\n---', 3);
  if (end === -1) {
    return markdown;
  }

  return markdown.slice(end + 4);
}

function cleanInlineMarkdown(value) {
  return value
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/_([^_]+)_/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')
    .replace(/<[^>]+>/g, ' ')
    .replace(/[{}()[\]#>*_|~-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function extractHeadings(markdown) {
  return markdown
    .split('\n')
    .map((line) => line.match(/^#{1,6}\s+(.+)$/)?.[1])
    .filter(Boolean)
    .map(cleanInlineMarkdown);
}

function markdownToSearchText(markdown) {
  const withoutFrontMatter = stripFrontMatter(markdown);
  const withoutFences = withoutFrontMatter.replace(/```[\s\S]*?```/g, (block) =>
    block.replace(/```[a-zA-Z0-9_-]*\n?/g, '').replace(/```/g, ''),
  );

  return withoutFences
    .split('\n')
    .filter((line) => !line.trim().startsWith(':::'))
    .filter((line) => !/^import\s/.test(line.trim()))
    .filter((line) => !/^export\s/.test(line.trim()))
    .map(cleanInlineMarkdown)
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function sectionFromDocId(id) {
  const [firstSegment] = id.split('/');

  return firstSegment
    .split('-')
    .map((segment) => segment.charAt(0).toUpperCase() + segment.slice(1))
    .join(' ');
}

function toSearchEntry(siteDir, doc) {
  const sourcePath = resolveSourcePath(siteDir, doc.source);
  const markdown = fs.readFileSync(sourcePath, 'utf8');
  const content = markdownToSearchText(markdown);

  return {
    id: doc.id,
    title: doc.title,
    description: doc.description ?? '',
    permalink: doc.permalink,
    section: sectionFromDocId(doc.id),
    path: doc.id.replaceAll('/', ' / '),
    headings: extractHeadings(stripFrontMatter(markdown)),
    content: content.slice(0, MAX_CONTENT_LENGTH),
  };
}

module.exports = function localSearchPlugin(context) {
  const {siteDir} = context;

  return {
    name: PLUGIN_NAME,

    getPathsToWatch() {
      return [path.join(siteDir, 'docs/**/*.{md,mdx}')];
    },

    async allContentLoaded({allContent, actions}) {
      const docsContent =
        allContent['docusaurus-plugin-content-docs']?.default ??
        allContent['docusaurus-plugin-content-docs']?.['default'];
      const loadedVersions = docsContent?.loadedVersions ?? [];
      const docs = loadedVersions.flatMap((version) => version.docs ?? []);

      const searchIndex = docs
        .filter((doc) => !doc.draft && !doc.unlisted)
        .map((doc) => toSearchEntry(siteDir, doc))
        .sort((a, b) => a.title.localeCompare(b.title));

      actions.setGlobalData({
        version: 1,
        docs: searchIndex,
      });
    },
  };
};

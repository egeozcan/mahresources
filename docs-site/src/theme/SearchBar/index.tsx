import React, {
  type KeyboardEvent,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
import Link from '@docusaurus/Link';
import {useHistory} from '@docusaurus/router';
import {usePluginData} from '@docusaurus/useGlobalData';

type SearchDoc = {
  id: string;
  title: string;
  description: string;
  permalink: string;
  section: string;
  path: string;
  headings: string[];
  content: string;
};

type SearchData = {
  docs?: SearchDoc[];
};

type IndexedSearchDoc = SearchDoc & {
  normalized: {
    title: string;
    description: string;
    section: string;
    path: string;
    headings: string;
    content: string;
    combined: string;
  };
};

type SearchResult = {
  doc: IndexedSearchDoc;
  score: number;
  snippet: string;
};

const MIN_QUERY_LENGTH = 2;
const MAX_RESULTS = 8;

function normalize(value: string): string {
  return value
    .toLocaleLowerCase()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '');
}

function tokenize(query: string): string[] {
  return normalize(query)
    .split(/\s+/)
    .map((token) => token.trim())
    .filter(Boolean);
}

function createSnippet(doc: IndexedSearchDoc, tokens: string[]): string {
  const source =
    doc.content || doc.description || doc.headings.join(' ') || doc.title;
  const normalizedSource = normalize(source);
  const matchPositions = tokens
    .map((token) => normalizedSource.indexOf(token))
    .filter((position) => position >= 0);
  const firstMatch = matchPositions.length > 0 ? Math.min(...matchPositions) : 0;
  const start = Math.max(0, firstMatch - 70);
  const end = Math.min(source.length, start + 180);
  const prefix = start > 0 ? '...' : '';
  const suffix = end < source.length ? '...' : '';

  return `${prefix}${source.slice(start, end).trim()}${suffix}`;
}

function scoreDoc(
  doc: IndexedSearchDoc,
  normalizedQuery: string,
  tokens: string[],
): number {
  if (!tokens.every((token) => doc.normalized.combined.includes(token))) {
    return 0;
  }

  let score = 0;

  if (doc.normalized.title === normalizedQuery) {
    score += 1000;
  }
  if (doc.normalized.title.startsWith(normalizedQuery)) {
    score += 360;
  }
  if (doc.normalized.title.includes(normalizedQuery)) {
    score += 260;
  }
  if (doc.normalized.path.includes(normalizedQuery)) {
    score += 160;
  }
  if (doc.normalized.headings.includes(normalizedQuery)) {
    score += 140;
  }
  if (doc.normalized.description.includes(normalizedQuery)) {
    score += 100;
  }
  if (doc.normalized.content.includes(normalizedQuery)) {
    score += 50;
  }

  for (const token of tokens) {
    if (doc.normalized.title.includes(token)) {
      score += 90;
    }
    if (doc.normalized.path.includes(token)) {
      score += 45;
    }
    if (doc.normalized.headings.includes(token)) {
      score += 45;
    }
    if (doc.normalized.description.includes(token)) {
      score += 32;
    }
    if (doc.normalized.content.includes(token)) {
      score += 15;
    }
  }

  return score;
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  return (
    target.isContentEditable ||
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  );
}

export default function SearchBar(): React.ReactNode {
  const history = useHistory();
  const panelId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const searchData = usePluginData('local-search') as SearchData | undefined;
  const docs = searchData?.docs ?? [];
  const [query, setQuery] = useState('');
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);

  const indexedDocs = useMemo<IndexedSearchDoc[]>(
    () =>
      docs.map((doc) => {
        const normalized = {
          title: normalize(doc.title),
          description: normalize(doc.description),
          section: normalize(doc.section),
          path: normalize(doc.path),
          headings: normalize(doc.headings.join(' ')),
          content: normalize(doc.content),
          combined: '',
        };

        normalized.combined = [
          normalized.title,
          normalized.description,
          normalized.section,
          normalized.path,
          normalized.headings,
          normalized.content,
        ].join(' ');

        return {...doc, normalized};
      }),
    [docs],
  );

  const results = useMemo<SearchResult[]>(() => {
    const trimmedQuery = query.trim();
    if (trimmedQuery.length < MIN_QUERY_LENGTH) {
      return [];
    }

    const normalizedQuery = normalize(trimmedQuery);
    const tokens = tokenize(trimmedQuery);

    return indexedDocs
      .map((doc) => ({
        doc,
        score: scoreDoc(doc, normalizedQuery, tokens),
        snippet: createSnippet(doc, tokens),
      }))
      .filter((result) => result.score > 0)
      .sort((a, b) => b.score - a.score || a.doc.title.localeCompare(b.doc.title))
      .slice(0, MAX_RESULTS);
  }, [indexedDocs, query]);

  const shouldShowPanel = isOpen && query.trim().length >= MIN_QUERY_LENGTH;
  const activeResult = results[activeIndex];

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  useEffect(() => {
    if (!shouldShowPanel) {
      return undefined;
    }

    function closeOnOutsideClick(event: MouseEvent) {
      if (
        rootRef.current &&
        event.target instanceof Node &&
        !rootRef.current.contains(event.target)
      ) {
        setIsOpen(false);
      }
    }

    document.addEventListener('mousedown', closeOnOutsideClick);
    return () => document.removeEventListener('mousedown', closeOnOutsideClick);
  }, [shouldShowPanel]);

  useEffect(() => {
    function focusSearch(event: globalThis.KeyboardEvent) {
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLocaleLowerCase() === 'k' &&
        !isEditableTarget(event.target)
      ) {
        event.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
        setIsOpen(true);
      }
    }

    window.addEventListener('keydown', focusSearch);
    return () => window.removeEventListener('keydown', focusSearch);
  }, []);

  function navigateToResult(result: SearchResult | undefined) {
    if (!result) {
      return;
    }

    setIsOpen(false);
    setQuery('');
    history.push(result.doc.permalink);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setIsOpen(true);
      setActiveIndex((index) =>
        results.length === 0 ? 0 : Math.min(index + 1, results.length - 1),
      );
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActiveIndex((index) => Math.max(index - 1, 0));
    } else if (event.key === 'Enter') {
      if (shouldShowPanel && activeResult) {
        event.preventDefault();
        navigateToResult(activeResult);
      }
    } else if (event.key === 'Escape') {
      setIsOpen(false);
      inputRef.current?.blur();
    }
  }

  return (
    <div className="local-search" ref={rootRef}>
      <input
        ref={inputRef}
        className="local-search__input"
        type="search"
        placeholder="Search docs"
        value={query}
        role="combobox"
        aria-autocomplete="list"
        aria-controls={shouldShowPanel ? panelId : undefined}
        aria-expanded={shouldShowPanel}
        aria-label="Search documentation"
        onChange={(event) => {
          setQuery(event.target.value);
          setIsOpen(true);
        }}
        onFocus={() => setIsOpen(true)}
        onKeyDown={handleKeyDown}
      />
      {shouldShowPanel && (
        <div className="local-search__panel" id={panelId} role="listbox">
          {results.length > 0 ? (
            results.map((result, index) => (
              <Link
                className="local-search__result"
                data-active={index === activeIndex}
                key={result.doc.id}
                role="option"
                aria-selected={index === activeIndex}
                to={result.doc.permalink}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => {
                  setIsOpen(false);
                  setQuery('');
                }}>
                <span className="local-search__result-title">
                  {result.doc.title}
                </span>
                <span className="local-search__result-section">
                  {result.doc.section}
                </span>
                <span className="local-search__result-snippet">
                  {result.snippet}
                </span>
              </Link>
            ))
          ) : (
            <div className="local-search__empty">No results</div>
          )}
        </div>
      )}
    </div>
  );
}

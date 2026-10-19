// markdownEditor: a CodeMirror markdown editor over a Description textarea
// (docs/plans/2026-10-19-markdown-description-editor-design.md).
//
// The textarea is the field the form submits. It stays usable while
// CodeMirror loads, the editor mounts with whatever it holds once the imports
// land (so nothing typed meanwhile is lost), and every edit is written back
// to it. If loading fails the textarea simply stays.
import * as commands from './markdownCommands.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

// About six lines of the editor's text, then up to 60% of the viewport.
const MIN_HEIGHT = '9rem';
const MAX_HEIGHT = '60vh';

const DEFAULT_MENTION_TYPES = 'resource,note,group,tag';

// The entity types whose Descriptions run shortcodes on their detail page:
// the ones the server can build a shortcode context for
// (BuildMetaContextForEntity in shortcode_tag.go). Forms pass the same choice
// from the template; inline editing derives it from the entity type.
// CodeMirror default bindings the editor leaves out; see the keymap below.
const DROPPED_DEFAULT_KEYS = new Set(['Ctrl-k', 'Mod-Enter']);

export const SHORTCODE_ENTITY_TYPES = new Set(['group', 'note', 'resource', 'category', 'resourceCategory', 'noteType']);

// The stylesheets base.tpl ships, so the preview frame looks like the page.
const PREVIEW_STYLESHEETS = ['/public/index.css', '/public/tailwind.css', '/public/jsonTable.css'];

export function markdownEditor({
  mentionTypes = DEFAULT_MENTION_TYPES,
  shortcodes = false,
  previewUrl = '',
  previewId = '',
  autofocus = false,
} = {}) {
  return {
    view: null,
    ready: false,
    failed: false,
    previewing: false,
    previewLoading: false,
    previewError: '',
    _previewGeneration: 0,
    _frame: null,
    _destroyed: false,
    _source: null,
    _container: null,
    _labelId: '',
    _liveRegion: null,

    init() {
      // Captured here: inside a method reached from a toolbar @click, $el is
      // the button and $refs may not resolve (docs/lessons.md).
      this._source = this.$refs.source;
      this._container = this.$refs.mdContainer;
      this._frame = this.$refs.previewFrame || null;
      this._labelId = this._source.dataset.labelId || '';
      // Mention result counts, as mentionTextarea.js announced them: CodeMirror
      // wires the listbox but says nothing about how many results came back.
      this._liveRegion = createLiveRegion();
      // Opened on purpose (double-click to edit): focus the textarea now, so
      // typing during the load lands in it and the mount carries focus over.
      if (autofocus) this._source.focus();
      this._mount().catch((err) => {
        console.error('markdown editor failed to load', err);
        this.failed = true;
      });
    },

    async _mount() {
      const [
        { EditorView, keymap, drawSelection, placeholder },
        { EditorState },
        { defaultKeymap, history, historyKeymap },
        { syntaxHighlighting, defaultHighlightStyle },
        { autocompletion, completionKeymap },
        { markdown },
        { mentionCompletionSource },
        shortcodeMod,
      ] = await Promise.all([
        import('@codemirror/view'),
        import('@codemirror/state'),
        import('@codemirror/commands'),
        import('@codemirror/language'),
        import('@codemirror/autocomplete'),
        import('@codemirror/lang-markdown'),
        import('./markdownMentions.js'),
        shortcodes ? import('./shortcodeCompletion.js') : Promise.resolve(null),
      ]);
      // Closed (an inline editor cancelled) before the imports landed.
      if (this._destroyed) return;

      const source = this._source;
      const sources = [mentionCompletionSource(mentionTypes, (msg) => this._liveRegion?.announce(msg))];
      const shortcodeExtensions = [];
      if (shortcodeMod) {
        sources.push(shortcodeMod.shortcodeCompletionSource(() => ''));
        shortcodeExtensions.push(shortcodeMod.shortcodeAutoTrigger(), shortcodeMod.shortcodeHoverTooltip());
      }

      const contentAttributes = { 'aria-multiline': 'true' };
      if (this._labelId) contentAttributes['aria-labelledby'] = this._labelId;
      else if (source.getAttribute('aria-label')) contentAttributes['aria-label'] = source.getAttribute('aria-label');
      if (source.getAttribute('aria-describedby')) contentAttributes['aria-describedby'] = source.getAttribute('aria-describedby');

      // Read now, not before the imports: text typed into the textarea while
      // CodeMirror loaded is part of the document.
      const doc = source.value;
      const hadFocus = document.activeElement === source;
      const selStart = source.selectionStart;
      const selEnd = source.selectionEnd;
      const backward = source.selectionDirection === 'backward';

      const extensions = [
        history(),
        drawSelection(),
        markdown(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        EditorView.lineWrapping,
        autocompletion({ override: sources, icons: false }),
        ...shortcodeExtensions,
        // No indentWithTab: Tab moves focus out of the field, as in a textarea.
        // Without macOS's Emacs-style Ctrl-k (delete to line end): Ctrl/Cmd+K
        // belongs to global search, and it would also delete the line's text.
        keymap.of([
          ...commands.markdownKeymap,
          ...completionKeymap,
          // Nor Mod-Enter (insert blank line): Ctrl/Cmd+Enter saves an inline
          // edit (descriptionEditor), and a form has no use for it.
          ...defaultKeymap.filter((binding) => !DROPPED_DEFAULT_KEYS.has(binding.key) && binding.mac !== 'Ctrl-k'),
          ...historyKeymap,
        ]),
        EditorView.contentAttributes.of(contentAttributes),
        EditorView.updateListener.of((update) => {
          if (!update.docChanged) return;
          source.value = update.state.doc.toString();
          source.dispatchEvent(new Event('input', { bubbles: true }));
        }),
        EditorView.theme({
          '&': { minHeight: MIN_HEIGHT, maxHeight: MAX_HEIGHT, fontSize: '0.875rem' },
          // The same metrics as the textarea underneath (createFormMarkdownInput.tpl).
          '.cm-scroller': { overflow: 'auto', fontFamily: 'inherit', lineHeight: '1.5', minHeight: MIN_HEIGHT },
          '.cm-content': { padding: '0.5rem 0.75rem' },
          // Focus: the global .cm-editor.cm-focused ring (index.css), whose
          // transparent outline still shows in forced-colors mode.
        }),
      ];
      if (source.placeholder) extensions.push(placeholder(source.placeholder));

      this.view = new EditorView({
        state: EditorState.create({ doc, extensions }),
        parent: this._container,
      });
      // axe scrollable-region-focusable, as in codeEditor.js.
      this.view.scrollDOM.tabIndex = 0;
      this._container._cmView = this.view;

      // Shown and the textarea hidden in the same task, before focusing: a
      // focus() on a still-hidden editor would drop focus to <body> and lose
      // the keystrokes of someone typing as it mounts.
      this._container.hidden = false;
      source.hidden = true;
      this.ready = true;
      if (hadFocus) {
        const clamp = (n) => Math.min(n ?? doc.length, doc.length);
        const [from, to] = [clamp(selStart), clamp(selEnd)];
        this.view.dispatch({ selection: backward ? { anchor: to, head: from } : { anchor: from, head: to } });
        this.view.focus();
      }
    },

    // What the preview pane says about shortcodes: they run only against a
    // saved entity, and widgets need scripts the sandboxed frame does not run.
    get shortcodeHint() {
      if (!shortcodes) return '';
      return previewId ? 'Interactive widgets do not work in preview.' : 'Shortcodes render after saving.';
    },

    get canPreview() {
      return Boolean(previewUrl) && this.ready;
    },

    // Preview: the server renders the text exactly as the detail page would
    // (POST /v1/<entity>/previewDescription) into a sandboxed frame. The frame
    // has no allow-same-origin and no scripts, so raw HTML in a Description
    // cannot reach this page or join its form.
    async showPreview() {
      if (!this.canPreview) return;
      const generation = ++this._previewGeneration;
      this.previewing = true;
      this._container.hidden = true;
      this.previewLoading = true;
      this.previewError = '';
      // Blank until this request succeeds: a failed refresh must not leave the
      // previous text on show as if it were the preview of the current text.
      if (this._frame) this._frame.srcdoc = '';
      const url = previewId ? `${previewUrl}?id=${encodeURIComponent(previewId)}` : previewUrl;
      const body = new FormData();
      body.append('text', this._source.value);
      try {
        const response = await fetch(url, { method: 'POST', body, headers: { Accept: 'application/json' } });
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data.error || `Preview failed (${response.status})`);
        if (generation !== this._previewGeneration) return;
        this._renderFrame(data.html || '');
      } catch (err) {
        if (generation !== this._previewGeneration) return;
        this.previewError = err.message || 'Could not render the preview';
      } finally {
        if (generation === this._previewGeneration) this.previewLoading = false;
      }
    },

    showWrite() {
      this._previewGeneration++;
      this.previewing = false;
      this.previewLoading = false;
      this.previewError = '';
      if (!this.view) return;
      this._container.hidden = false;
      this.view.focus();
    },

    _renderFrame(html) {
      if (!this._frame) return;
      const links = PREVIEW_STYLESHEETS.map((href) => `<link rel="stylesheet" href="${href}">`).join('\n');
      // <base target="_blank">: a link in the preview opens a tab rather than
      // navigating the frame.
      this._frame.srcdoc = `<!doctype html><html><head>
<meta charset="utf-8">
<base target="_blank">
${links}
<style>body{margin:0;padding:1rem;background:#fff;color:#1c1917;}</style>
</head><body><div class="prose lg:prose-xl max-w-none font-sans">${html}</div></body></html>`;
    },

    focusEditor(event) {
      if (!this.view) return;
      event?.preventDefault();
      this.view.focus();
    },

    _run(command) {
      if (this.view && !this.previewing) command(this.view);
    },
    bold() { this._run(commands.bold); },
    italic() { this._run(commands.italic); },
    link() { this._run(commands.insertLink); },
    heading() { this._run(commands.heading); },
    bulletList() { this._run(commands.bulletList); },
    numberedList() { this._run(commands.numberedList); },
    inlineCode() { this._run(commands.inlineCode); },

    destroy() {
      this._destroyed = true;
      this._liveRegion?.destroy();
      if (this.view) {
        this.view.destroy();
        this.view = null;
      }
    },
  };
}

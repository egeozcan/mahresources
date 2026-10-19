// markdownEditor: a CodeMirror markdown editor over a Description textarea
// (docs/plans/2026-10-19-markdown-description-editor-design.md).
//
// The textarea is the field the form submits. It stays usable while
// CodeMirror loads, the editor mounts with whatever it holds once the imports
// land (so nothing typed meanwhile is lost), and every edit is written back
// to it. If loading fails the textarea simply stays.
import * as commands from './markdownCommands.js';

// About six lines of the editor's text, then up to 60% of the viewport.
const MIN_HEIGHT = '9rem';
const MAX_HEIGHT = '60vh';

const DEFAULT_MENTION_TYPES = 'resource,note,group,tag';

export function markdownEditor({ mentionTypes = DEFAULT_MENTION_TYPES, shortcodes = false } = {}) {
  return {
    view: null,
    ready: false,
    failed: false,
    _destroyed: false,
    _source: null,
    _container: null,
    _labelId: '',

    init() {
      // Captured here: inside a method reached from a toolbar @click, $el is
      // the button and $refs may not resolve (docs/lessons.md).
      this._source = this.$refs.source;
      this._container = this.$refs.mdContainer;
      this._labelId = this._source.dataset.labelId || '';
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
      const sources = [mentionCompletionSource(mentionTypes)];
      const shortcodeExtensions = [];
      if (shortcodeMod) {
        sources.push(shortcodeMod.shortcodeCompletionSource(() => ''));
        shortcodeExtensions.push(shortcodeMod.shortcodeAutoTrigger(), shortcodeMod.shortcodeHoverTooltip());
      }

      const contentAttributes = { 'aria-multiline': 'true' };
      if (this._labelId) contentAttributes['aria-labelledby'] = this._labelId;

      // Read now, not before the imports: text typed into the textarea while
      // CodeMirror loaded is part of the document.
      const doc = source.value;
      const hadFocus = document.activeElement === source;
      const caret = source.selectionStart;

      const extensions = [
        history(),
        drawSelection(),
        markdown(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        EditorView.lineWrapping,
        autocompletion({ override: sources, icons: false }),
        ...shortcodeExtensions,
        // No indentWithTab: Tab moves focus out of the field, as in a textarea.
        keymap.of([...commands.markdownKeymap, ...completionKeymap, ...defaultKeymap, ...historyKeymap]),
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
          '&.cm-focused': { outline: 'none' },
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

      source.hidden = true;
      this.ready = true;
      if (hadFocus) {
        const pos = Math.min(caret ?? doc.length, doc.length);
        this.view.dispatch({ selection: { anchor: pos } });
        this.view.focus();
      }
    },

    focusEditor(event) {
      if (!this.view) return;
      event?.preventDefault();
      this.view.focus();
    },

    _run(command) {
      if (this.view) command(this.view);
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
      if (this.view) {
        this.view.destroy();
        this.view = null;
      }
    },
  };
}

/**
 * The inline description editor used by `templates/partials/description.tpl`,
 * which twelve templates include on both detail pages and list cards.
 *
 * It used to live entirely in an `@click.away` attribute, which made a mouse
 * click elsewhere on the page the *only* way to commit an edit: there was no
 * Save control and no keyboard path at all, so a keyboard-only user could type
 * a description and had no way to store it. Tab left the editor open holding
 * text that was never sent, and Escape threw it away with no confirmation
 * (UI bug hunt 2026-07-29, findings 15 / 53 / 87).
 *
 * Commit paths, and why they differ:
 *   - Save, Ctrl/Cmd+Enter and the original click-away reload the page
 *     afterwards. The stored text is markdown with shortcodes and only the
 *     server renders it; these are all explicit commits, and reloading is what
 *     click-away already did.
 *   - Leaving the editor region with the keyboard commits but does *not*
 *     reload. It is a safety net against silent loss, and a user tabbing on to
 *     the next control must not have the page pulled out from under them
 *     (WCAG 3.2.2). The saved text is shown as plain text until the next load.
 */
export function descriptionEditor({ url = '' } = {}) {
    return {
        editing: false,
        saving: false,
        error: '',
        descriptionEditUrl: url,

        /**
         * The last value known to be on the server, or null while that is still
         * whatever the server rendered into the textarea. Needed because a
         * commit that does not reload leaves the template's `{{ description }}`
         * stale, so re-opening the editor must not resurrect it.
         */
        serverValue: null,

        /** Set by onKeyDown so onFocusOut can tell Tab from a pointer blur. */
        _keyboardExit: false,

        /** The component root, captured while attached. See the comment below. */
        rootEl: null,

        init() {
            this.rootEl = this.$el;
        },

        startEditing() {
            if (!this.descriptionEditUrl) return;
            this.error = '';
            this.editing = true;
        },

        cancel() {
            this.error = '';
            this.editing = false;
        },

        // Neither $el nor $root works here. Alpine binds $el to the element whose
        // directive is evaluating, so inside a method reached from the textarea's
        // @keydown $el is the textarea, not the component root. $root fixes that
        // but resolves by walking up from that same element — so a callback that
        // runs after `editing` flips false, when the x-if subtree it came from is
        // already detached, gets undefined back. Hence a root captured at init.
        _editorEl() {
            return this.rootEl.querySelector('[data-description-editor]');
        },

        _textareaEl() {
            return this.rootEl.querySelector('textarea[name="description"]');
        },

        /**
         * Commit whatever is in the textarea.
         * @param {{ reload?: boolean, event?: Event }} [options]
         */
        async save({ reload = true, event = null } = {}) {
            if (this.saving) return;
            const textarea = this._textareaEl();
            if (!textarea) return;

            const value = textarea.value;
            const baseline = this.serverValue === null ? textarea.defaultValue : this.serverValue;
            if (value === baseline) {
                this.editing = false;
                return;
            }

            // A click that lands on a link is a navigation the user asked for:
            // save, but let the browser follow it instead of reloading here.
            const clickedLink = !!(event && event.target && event.target.closest && event.target.closest('a[href]'));
            const container = this.rootEl;

            const formData = new FormData();
            formData.append('description', value);

            this.saving = true;
            try {
                const response = await fetch(this.descriptionEditUrl, { method: 'POST', body: formData });
                if (!response.ok) {
                    throw new Error(await this._errorMessage(response));
                }

                this.serverValue = value;
                this.error = '';

                if (reload && !clickedLink) {
                    location.reload();
                    return;
                }

                this.editing = false;
                this.$nextTick(() => this._showPlainText(value));
            } catch (e) {
                console.error('Failed to save description:', e);
                // Keep editing=true so the user's unsaved text is retained for retry.
                this.error = (e && e.message) || 'Could not save description';
                window.mahAnnounce && window.mahAnnounce(this.error, { assertive: true });
                if (container) {
                    container.style.transition = 'background-color 0.3s';
                    container.style.backgroundColor = '#fee2e2';
                    setTimeout(() => { container.style.backgroundColor = ''; }, 2000);
                }
            } finally {
                this.saving = false;
            }
        },

        /**
         * Tab is the only focus move this component commits on. A pointer-driven
         * blur is part of the same gesture as the click that follows it, and
         * `@click.away` already owns that path — reloading so shortcodes render.
         * Claiming it here too would have both fire and the reload would lose.
         * Same `_keyboardExit` idiom as src/webcomponents/inlineedit.js.
         */
        onKeyDown(event) {
            if (event.key === 'Tab') this._keyboardExit = true;
        },

        /** Commit only once focus has left the editor entirely, not on a hop to Save. */
        onFocusOut(event) {
            const byKeyboard = this._keyboardExit;
            this._keyboardExit = false;
            if (!byKeyboard) return;

            const editor = this._editorEl();
            if (!editor) return;
            if (event.relatedTarget && editor.contains(event.relatedTarget)) return;
            // relatedTarget is null when focus moves to the document body or
            // another window, so confirm against activeElement a tick later.
            queueMicrotask(() => {
                if (!this.editing || this.saving) return;
                if (editor.contains(document.activeElement)) return;
                this.save({ reload: false });
            });
        },

        async _errorMessage(response) {
            const fallback = `Could not save description (HTTP ${response.status})`;
            try {
                const body = await response.json();
                return (body && body.error) || fallback;
            } catch {
                return fallback;
            }
        },

        /**
         * Show the value just stored, after a save that did not reload. Markdown
         * and shortcodes stay unrendered until the next page load, which is the
         * trade for not moving the user's focus.
         */
        _showPlainText(value) {
            const display = this.rootEl.querySelector('[data-description-display]');
            if (!display) return;
            const paragraph = document.createElement('p');
            if (value) {
                paragraph.textContent = value;
            } else {
                paragraph.textContent = 'No description';
                paragraph.className = 'text-stone-500 italic';
            }
            display.replaceChildren(paragraph);
        },
    };
}

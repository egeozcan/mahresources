/** Prefer the API's own explanation over a generic client-side one. */
async function readServerError(response, fallback) {
    try {
        const body = await response.json();
        if (body && typeof body.error === 'string' && body.error.trim()) {
            return body.error;
        }
    } catch {
        // Not JSON — fall through.
    }
    return `${fallback} (HTTP ${response.status})`;
}

/**
 * Broadcast a committed inline edit to every other element on the page that
 * displays the same field. The resource detail page renders the name twice —
 * once in the <h1> as this component, once server-side in the METADATA card —
 * and only the first one used to update, so a rename left two different names
 * on screen until a manual reload (UI bug hunt 2026-07-29, finding 151).
 *
 * `data-entity-field` marks a display of the *page's main entity*. Detail pages
 * show exactly one, which is why a document-wide broadcast is safe; anything
 * rendered per card must not carry the attribute.
 */
document.addEventListener('inline-edit:saved', (event) => {
    const { field, value } = event.detail || {};
    if (!field) return;
    document.querySelectorAll(`[data-entity-field="${CSS.escape(field)}"]`).forEach((el) => {
        // The editor keeps its own text; rewriting it here would fight it.
        if (el.closest('inline-edit')) return;
        el.textContent = value;
    });
});

class InlineEdit extends HTMLElement {
    static get observedAttributes() {
        return ['multiline', 'post', 'name', 'label', 'value-is-placeholder'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.isEditing = false;

        // Add styles for focus indicator
        const style = document.createElement('style');
        style.textContent = `
            .edit-button:focus {
                outline: 2px solid #4f46e5;
                outline-offset: 2px;
                border-radius: 2px;
            }
            .edit-button:focus:not(:focus-visible) {
                outline: none;
            }
            .edit-button:focus-visible {
                outline: 2px solid #4f46e5;
                outline-offset: 2px;
                border-radius: 2px;
            }
        `;
        this.shadowRoot.appendChild(style);

        // Create container for display mode
        this.displayContainer = document.createElement('span');
        Object.assign(this.displayContainer.style, {
            display: 'inline-flex',
            alignItems: 'center',
        });

        // Create text span
        this.displayText = document.createElement('span');
        this.displayContainer.appendChild(this.displayText);

        // Create edit button wrapper for keyboard accessibility
        this.editButton = document.createElement('button');
        this.editButton.type = 'button';
        this.editButton.className = 'edit-button';
        this.editButton.setAttribute('aria-label', 'Edit');
        Object.assign(this.editButton.style, {
            background: 'none',
            border: 'none',
            padding: '2px',
            cursor: 'pointer',
            display: 'inline-flex',
            alignItems: 'center',
            marginLeft: '4px',
        });

        // Create SVG pencil icon
        const svgNS = 'http://www.w3.org/2000/svg';
        this.pencilIcon = document.createElementNS(svgNS, 'svg');
        this.pencilIcon.setAttribute('width', '20');
        this.pencilIcon.setAttribute('height', '20');
        this.pencilIcon.setAttribute('viewBox', '0 0 24 24');
        this.pencilIcon.setAttribute('fill', 'none');
        this.pencilIcon.setAttribute('stroke', 'currentColor');
        this.pencilIcon.setAttribute('stroke-width', '2');
        this.pencilIcon.setAttribute('stroke-linecap', 'round');
        this.pencilIcon.setAttribute('stroke-linejoin', 'round');
        this.pencilIcon.setAttribute('aria-hidden', 'true');

        const path1 = document.createElementNS(svgNS, 'path');
        path1.setAttribute('d', 'M12 20h9');
        const path2 = document.createElementNS(svgNS, 'path');
        path2.setAttribute('d', 'M16.5 3.5l4 4L7 21H3v-4L16.5 3.5z');

        this.pencilIcon.appendChild(path1);
        this.pencilIcon.appendChild(path2);

        this.editButton.appendChild(this.pencilIcon);
        this.displayContainer.appendChild(this.editButton);

        this.editButton.addEventListener('click', (e) => {
            e.preventDefault();
            this.enterEditMode();
        });

        this.shadowRoot.appendChild(this.displayContainer);

        // Visible failure message. Deliberately not a live region: the assertive
        // window.mahAnnounce call already speaks the failure, and a role="alert"
        // here would make screen readers say it twice. The input points at it
        // with aria-describedby instead.
        this.errorElement = document.createElement('div');
        this.errorElement.id = 'inline-edit-error';
        this.errorElement.setAttribute('data-inline-edit-error', '');
        // Not appended until there is something to say: the <h1> that hosts this
        // component must not grow a permanently reserved row.
        Object.assign(this.errorElement.style, {
            marginTop: '4px',
            color: '#b91c1c',
            fontSize: '0.875rem',
            fontWeight: '400',
            lineHeight: '1.25rem',
            whiteSpace: 'normal',
        });

        // Hidden slot consumes light DOM content so it is not rendered alongside
        // the shadow DOM. Without this, both the light DOM text and the shadow
        // DOM span are exposed to assistive technology, causing double announcement.
        const hiddenSlot = document.createElement('slot');
        hiddenSlot.style.display = 'none';
        this.shadowRoot.appendChild(hiddenSlot);

        this.updateProperties();
    }

    connectedCallback() {
        // `value-is-placeholder` means the slot text is a *fallback label*, not the
        // stored value — the server rendered it so the heading is not blank with
        // JS disabled, and the editor must still open empty. See _renderValue.
        const slotText = this.textContent.trim();
        if (this.hasAttribute('value-is-placeholder')) {
            this._value = '';
            this._placeholder = slotText;
        } else {
            this._value = slotText;
            this._placeholder = '';
        }
        this._renderValue();
    }

    /**
     * Paint the current value, falling back to the `placeholder` attribute when
     * there is none.
     *
     * UI bug hunt 2026-07-29, finding 64/59: a relation created without a Name —
     * which the create form permits — rendered an <h1> containing only an empty
     * <inline-edit>, so the page had a blank top-level heading and did not
     * identify itself, while <title> already computed "Relation from X to Y".
     *
     * The placeholder is display only. `_value` stays empty, so opening the
     * editor does not prefill the fallback as if it were the stored name, and
     * the change check below still sees "" -> "something" as a change.
     *
     * It is signalled by an explicit `value-is-placeholder` attribute rather than
     * inferred from "slot text equals some fallback": for almost every entity the
     * page title *is* the name, so inferring it would make a real name open an
     * empty editor.
     */
    _renderValue() {
        const placeholder = this._placeholder || '';
        if (this._value) {
            this.displayText.textContent = this._value;
            this.displayText.removeAttribute('data-placeholder');
            this.displayText.style.opacity = '';
        } else if (placeholder) {
            this.displayText.textContent = placeholder;
            this.displayText.setAttribute('data-placeholder', 'true');
            // Dimmed, but only to 0.75: this is the page's <h1>, and the text has
            // to keep 4.5:1 against white (WCAG 1.4.3).
            this.displayText.style.opacity = '0.75';
        } else {
            this.displayText.textContent = '';
            this.displayText.removeAttribute('data-placeholder');
            this.displayText.style.opacity = '';
        }
    }

    disconnectedCallback() {
        // If currently editing, cancel without saving
        if (this.isEditing) {
            this.isEditing = false;
        }
    }

    attributeChangedCallback() {
        this.updateProperties();
    }

    updateProperties() {
        this.multiline = this.hasAttribute('multiline');
        this.postUrl = this.getAttribute('post');
        this.name = this.getAttribute('name') || 'value';
        this.label = this.getAttribute('label') || this.name || 'value';

        // Update edit button aria-label
        if (this.editButton) {
            this.editButton.setAttribute('aria-label', `Edit ${this.label}`);
        }

        if (
            !this.inputElement ||
            (this.multiline !== (this.inputElement.tagName.toLowerCase() === 'textarea'))
        ) {
            // Create input element based on 'multiline' attribute
            this.inputElement = this.multiline
                ? document.createElement('textarea')
                : document.createElement('input');

            // Set accessibility attributes
            this.inputElement.setAttribute('aria-label', this.label);

            Object.assign(this.inputElement.style, {
                border: '1px solid #ccc',
                borderRadius: '4px',
                padding: '4px',
                fontSize: 'inherit',
                fontFamily: 'inherit',
                color: 'inherit',
                boxSizing: 'border-box',
                width: '100%',
                resize: this.multiline ? 'vertical' : 'none',
            });

            this.inputElement.addEventListener('click', (e) => {
                e.stopPropagation();
                e.preventDefault();
            });
            this.inputElement.addEventListener('blur', () => this.exitEditMode());
            this.inputElement.addEventListener('keydown', (e) => {
                if (e.key === 'Escape') {
                    e.preventDefault();
                    this._cancelled = true;
                    this._keyboardExit = true;
                    this.exitEditMode();
                } else if (!this.multiline && e.key === 'Enter') {
                    e.preventDefault();
                    this._keyboardExit = true;
                    this.exitEditMode();
                }
            });
        } else {
            // Update aria-label on existing input
            this.inputElement.setAttribute('aria-label', this.label);
        }
    }

    enterEditMode() {
        if (this.isEditing) return;
        this.clearError();
        this.isEditing = true;
        this._borrowHostLayout();
        this._originalValue = this._value ?? '';
        this.inputElement.value = this._originalValue;
        this.shadowRoot.replaceChild(this.inputElement, this.displayContainer);
        this.inputElement.focus();
        this.inputElement.select();
    }

    /**
     * Show a rejection where the user can actually see it, and put them back in
     * the field with the text they typed. Before this, the only signals were a
     * 1x1 clipped live region and a one-second red flash, and the typed value
     * was discarded (findings 21 / 88).
     */
    showError(message, keepValue) {
        this.errorElement.textContent = message;
        if (!this.errorElement.isConnected) {
            this.shadowRoot.appendChild(this.errorElement);
        }

        this.inputElement.setAttribute('aria-invalid', 'true');
        this.inputElement.setAttribute('aria-describedby', this.errorElement.id);
        this.inputElement.style.borderColor = '#b91c1c';

        this.isEditing = true;
        // exitEditMode() has already restored the host's inline layout by the time a
        // rejection comes back, and this puts the reader into the input again — so the
        // borrowed block layout has to come back with it, or a retry after a failed
        // save is typed into a 166px box (finding 141).
        this._borrowHostLayout();
        this.inputElement.value = keepValue;
        if (this.displayContainer.parentNode === this.shadowRoot) {
            this.shadowRoot.replaceChild(this.inputElement, this.displayContainer);
        }
        this.inputElement.focus();
        this.inputElement.select();
    }

    clearError() {
        if (this.errorElement.isConnected) {
            this.errorElement.remove();
        }
        this.errorElement.textContent = '';
        this.inputElement.removeAttribute('aria-invalid');
        this.inputElement.removeAttribute('aria-describedby');
        this.inputElement.style.borderColor = '#ccc';
    }

    /**
     * Make the host block-level for the duration of an edit.
     *
     * The host is an inline custom element, so it is only as wide as the text it was
     * displaying, and the shadow input is width:100% of the host — which made the
     * rename field 166px for a 166-character name, scrollWidth 2215, about 9% of the
     * value visible at a time with no wrapping (UI bug hunt 2026-07-29, finding 141).
     * The previous inline styles are remembered rather than assumed empty, so a host
     * that was styled by its own page is put back the way it was.
     */
    _borrowHostLayout() {
        if (this._hostLayoutBorrowed) return;
        this._hostLayoutBorrowed = true;
        this._previousHostDisplay = this.style.display;
        this._previousHostWidth = this.style.width;
        this.style.display = 'block';
        this.style.width = '100%';
    }

    /** Undo the block-level layout _borrowHostLayout() took. */
    _restoreHostLayout() {
        if (!this._hostLayoutBorrowed) return;
        this._hostLayoutBorrowed = false;
        this.style.display = this._previousHostDisplay || '';
        this.style.width = this._previousHostWidth || '';
    }

    exitEditMode() {
        if (!this.isEditing) return;
        this.isEditing = false;
        this._restoreHostLayout();

        // Return focus to the edit button only when the edit was dismissed via
        // keyboard (Enter/Escape) — not on blur — so clicking or tabbing away is
        // not yanked back. WCAG 2.4.3.
        const keyboardExit = this._keyboardExit;
        this._keyboardExit = false;

        if (this._cancelled) {
            this._cancelled = false;
            this.clearError();
            this._value = this._originalValue;
            this._renderValue();
            this.shadowRoot.replaceChild(this.displayContainer, this.inputElement);
            if (keyboardExit) this.editButton.focus();
            return;
        }

        const newValue = this.inputElement.value.trim();
        this._value = newValue;
        if (newValue) {
            // A real name now exists; the fallback heading must not come back if a
            // later edit is cancelled or rejected.
            this._placeholder = '';
            this.removeAttribute('value-is-placeholder');
        }
        this._renderValue();
        this.textContent = newValue;
        this.shadowRoot.replaceChild(this.displayContainer, this.inputElement);
        if (keyboardExit) this.editButton.focus();

        // Only post if the value actually changed
        if (this.postUrl && newValue !== this._originalValue) {
            const formData = new FormData();
            formData.append(this.name, newValue);

            fetch(this.postUrl, {
                method: 'POST',
                body: formData,
            }).then(async (response) => {
                if (!response.ok) {
                    // The server explains itself — "a tag named "design" already
                    // exists", "name must not be empty" — and this used to throw
                    // that away in favour of "Could not save name" (finding 152).
                    throw new Error(await readServerError(response, `Could not save ${this.label}`));
                }
                // Retire any message a previous rejection left behind, or a
                // corrected name lands under a red error that no longer applies.
                this.clearError();
                // Update document title when the entity name changes
                if (this.name === 'name') {
                    const suffix = ' - mahresources';
                    const currentTitle = document.title;
                    // Replace the old name portion before the suffix
                    if (currentTitle.endsWith(suffix)) {
                        const prefix = currentTitle.slice(0, currentTitle.length - suffix.length);
                        // The title format varies: "Entity Name - mahresources" or
                        // "Type Entity Name - mahresources". Try to replace just the
                        // entity name portion (after the last known type prefix).
                        const idx = prefix.lastIndexOf(this._originalValue);
                        if (idx !== -1) {
                            document.title = prefix.slice(0, idx) + newValue + prefix.slice(idx + this._originalValue.length) + suffix;
                        }
                    }
                }
                // Flash success indicator
                this.displayText.style.transition = 'background-color 0.3s';
                this.displayText.style.backgroundColor = '#d1fae5';
                setTimeout(() => { this.displayText.style.backgroundColor = ''; }, 1000);
                // Announce to assistive tech (the green flash alone is color-only). WCAG 4.1.3.
                window.mahAnnounce?.(`${this.label} saved`);
                // Let every other display of this field catch up (finding 151).
                this.dispatchEvent(new CustomEvent('inline-edit:saved', {
                    bubbles: true,
                    composed: true,
                    detail: { field: this.name, value: newValue, previous: this._originalValue, post: this.postUrl },
                }));
            }).catch((error) => {
                console.error('Error posting data:', error);
                const message = (error && error.message) || `Could not save ${this.label}`;
                // The stored value is unchanged, so every *display* of it reverts…
                this._value = this._originalValue;
                this._renderValue();
                this.textContent = this._originalValue;
                this.displayText.style.transition = 'background-color 0.3s';
                this.displayText.style.backgroundColor = '#fee2e2';
                setTimeout(() => { this.displayText.style.backgroundColor = ''; }, 1000);
                // …but the user's attempt is not thrown away: the editor reopens
                // holding it, next to a visible reason.
                this.showError(message, newValue);
                // Announce the failure assertively (the red flash alone is color-only). WCAG 4.1.3.
                window.mahAnnounce?.(message, { assertive: true });
            });
        }
    }
}

customElements.define('inline-edit', InlineEdit);

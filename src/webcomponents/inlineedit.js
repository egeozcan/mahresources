class InlineEdit extends HTMLElement {
    static get observedAttributes() {
        return ['multiline', 'post', 'name', 'label'];
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
        this.updateProperties();
    }

    connectedCallback() {
        this.displayText.textContent = this.textContent.trim();
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
        this.label = this.getAttribute('label') || 'Edit value';

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

            this.inputElement.addEventListener('blur', () => this.exitEditMode());
            this.inputElement.addEventListener('keydown', (e) => {
                if (e.key === 'Escape') {
                    e.preventDefault();
                    this._cancelled = true;
                    this.exitEditMode();
                } else if (!this.multiline && e.key === 'Enter') {
                    e.preventDefault();
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
        this.isEditing = true;
        this._originalValue = this.displayText.textContent;
        this.inputElement.value = this._originalValue;
        this.shadowRoot.replaceChild(this.inputElement, this.displayContainer);
        this.inputElement.focus();
        this.inputElement.select();
    }

    exitEditMode() {
        if (!this.isEditing) return;
        this.isEditing = false;

        if (this._cancelled) {
            this._cancelled = false;
            this.displayText.textContent = this._originalValue;
            this.shadowRoot.replaceChild(this.displayContainer, this.inputElement);
            return;
        }

        const newValue = this.inputElement.value.trim();
        this.displayText.textContent = newValue;
        this.shadowRoot.replaceChild(this.displayContainer, this.inputElement);

        // Only post if the value actually changed
        if (this.postUrl && newValue !== this._originalValue) {
            const formData = new FormData();
            formData.append(this.name, newValue);

            fetch(this.postUrl, {
                method: 'POST',
                body: formData,
            }).then(() => {
                // Flash success indicator
                this.displayText.style.transition = 'background-color 0.3s';
                this.displayText.style.backgroundColor = '#d1fae5';
                setTimeout(() => { this.displayText.style.backgroundColor = ''; }, 1000);
            }).catch((error) => {
                console.error('Error posting data:', error);
                // Revert on error
                this.displayText.textContent = this._originalValue;
                this.displayText.style.transition = 'background-color 0.3s';
                this.displayText.style.backgroundColor = '#fee2e2';
                setTimeout(() => { this.displayText.style.backgroundColor = ''; }, 1000);
            });
        }
    }
}

customElements.define('inline-edit', InlineEdit);

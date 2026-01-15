class InlineEdit extends HTMLElement {
    static get observedAttributes() {
        return ['multiline', 'post', 'name'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.isEditing = false;

        // Create container for display mode
        this.displayContainer = document.createElement('span');
        Object.assign(this.displayContainer.style, {
            cursor: 'pointer',
            display: 'inline-flex',
            alignItems: 'center',
        });

        // Create text span
        this.displayText = document.createElement('span');
        this.displayContainer.appendChild(this.displayText);

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
        this.pencilIcon.style.marginLeft = '4px';

        const path1 = document.createElementNS(svgNS, 'path');
        path1.setAttribute('d', 'M12 20h9');
        const path2 = document.createElementNS(svgNS, 'path');
        path2.setAttribute('d', 'M16.5 3.5l4 4L7 21H3v-4L16.5 3.5z');

        this.pencilIcon.appendChild(path1);
        this.pencilIcon.appendChild(path2);

        this.displayContainer.appendChild(this.pencilIcon);

        this.pencilIcon.addEventListener('click', (e) => {
            e.preventDefault();
            this.enterEditMode();
        });
        this.shadowRoot.appendChild(this.displayContainer);
        this.updateProperties();
    }

    connectedCallback() {
        this.displayText.textContent = this.textContent.trim();
    }

    attributeChangedCallback() {
        this.updateProperties();
    }

    updateProperties() {
        this.multiline = this.hasAttribute('multiline');
        this.postUrl = this.getAttribute('post');
        this.name = this.getAttribute('name') || 'value';

        if (
            !this.inputElement ||
            (this.multiline !== (this.inputElement.tagName.toLowerCase() === 'textarea'))
        ) {
            // Create input element based on 'multiline' attribute
            this.inputElement = this.multiline
                ? document.createElement('textarea')
                : document.createElement('input');
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
                if ((!this.multiline && e.key === 'Enter') || e.key === 'Escape') {
                    e.preventDefault();
                    this.exitEditMode();
                }
            });
        }
    }

    enterEditMode() {
        if (this.isEditing) return;
        this.isEditing = true;
        this.inputElement.value = this.displayText.textContent;
        this.shadowRoot.replaceChild(this.inputElement, this.displayContainer);
        this.inputElement.focus();
        this.inputElement.select();
    }

    exitEditMode() {
        if (!this.isEditing) return;
        this.isEditing = false;

        const newValue = this.inputElement.value.trim();
        this.displayText.textContent = newValue;
        this.shadowRoot.replaceChild(this.displayContainer, this.inputElement);

        // Post data to server
        if (this.postUrl) {
            const formData = new FormData();
            formData.append(this.name, newValue);

            fetch(this.postUrl, {
                method: 'POST',
                body: formData,
            }).catch((error) => {
                console.error('Error posting data:', error);
            });
        }
    }
}

customElements.define('inline-edit', InlineEdit);

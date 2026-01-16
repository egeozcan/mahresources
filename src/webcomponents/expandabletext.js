import { updateClipboard } from '../index.js';

class ExpandableText extends HTMLElement {
    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.uniqueId = `expandable-${Math.random().toString(36).substring(2, 11)}`;

        // Append styles immediately
        const style = document.createElement('style');
        style.textContent = `
          .container {
            font-family: Arial, sans-serif;
          }
          button {
            cursor: pointer;
            display: inline;
            margin-left: 1rem;
          }
          button + button {
            margin-left: 0.5rem;
          }
          button:focus {
            outline: 2px solid #4f46e5;
            outline-offset: 2px;
            border-radius: 2px;
          }
          button:focus:not(:focus-visible) {
            outline: none;
          }
          button:focus-visible {
            outline: 2px solid #4f46e5;
            outline-offset: 2px;
            border-radius: 2px;
          }
        `;
        this.shadowRoot.appendChild(style);
    }

    connectedCallback() {
        const container = document.createElement('span');
        container.setAttribute('class', 'container');

        // Get the full text from the slot
        const fullText = this.innerHTML.trim();

        // Show only the first 30 characters initially
        const previewText = fullText.length > 30 ? fullText.substring(0, 30) : fullText;

        const previewSpan = document.createElement('span');
        previewSpan.textContent = previewText;

        const fullTextSpan = document.createElement('span');
        fullTextSpan.id = this.uniqueId;
        fullTextSpan.textContent = fullText;
        fullTextSpan.style.display = 'none'; // Initially hidden
        fullTextSpan.setAttribute('aria-hidden', 'true');

        // Create the toggle button (inline) only if the text is longer than 30 characters
        let button = null;
        let copyButton = null;

        if (fullText.length > 30) {
            button = document.createElement('button');
            button.type = 'button';
            button.textContent = 'Read more';
            button.setAttribute('aria-expanded', 'false');
            button.setAttribute('aria-controls', this.uniqueId);

            // Handle the expand/collapse logic
            button.addEventListener('click', () => {
                const isExpanded = fullTextSpan.style.display !== 'none';
                if (!isExpanded) {
                    fullTextSpan.style.display = 'inline';
                    fullTextSpan.setAttribute('aria-hidden', 'false');
                    previewSpan.style.display = 'none';
                    button.textContent = 'Read less';
                    button.setAttribute('aria-expanded', 'true');
                } else {
                    fullTextSpan.style.display = 'none';
                    fullTextSpan.setAttribute('aria-hidden', 'true');
                    previewSpan.style.display = 'inline';
                    button.textContent = 'Read more';
                    button.setAttribute('aria-expanded', 'false');
                }
            });

            copyButton = document.createElement('button');
            copyButton.type = 'button';
            copyButton.textContent = 'Copy';
            copyButton.setAttribute('aria-label', 'Copy text to clipboard');
            copyButton.addEventListener('click', () => {
                updateClipboard(fullText);
            });
        }

        // Append elements to the shadow DOM
        container.appendChild(previewSpan);
        container.appendChild(fullTextSpan);

        if (button && copyButton) {
            container.appendChild(button);
            container.appendChild(copyButton);
        }

        this.shadowRoot.appendChild(container);
    }
}

customElements.define('expandable-text', ExpandableText);

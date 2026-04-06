import { updateClipboard } from '../index.js';

class ExpandableText extends HTMLElement {
    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.uniqueId = `expandable-${Math.random().toString(36).substring(2, 11)}`;

        const style = document.createElement('style');
        style.textContent = `
          :host {
            display: inline;
          }
          .container {
            font-family: 'IBM Plex Mono', monospace;
            color: #292524;
          }
          .ellipsis {
            color: #a8a29e;
          }
          .toggle {
            font-family: 'IBM Plex Mono', monospace;
            font-size: 0.6875rem;
            color: #b45309;
            background: none;
            border: none;
            cursor: pointer;
            margin-left: 0.375rem;
            padding: 0;
            font-weight: 500;
          }
          .toggle:hover {
            text-decoration: underline;
          }
          .toggle:focus-visible {
            outline: 2px solid #b45309;
            outline-offset: 2px;
            border-radius: 2px;
          }
          .toggle:focus:not(:focus-visible) {
            outline: none;
          }
          .copy-btn {
            font-size: 0;
            color: #a8a29e;
            background: none;
            border: none;
            cursor: pointer;
            margin-left: 0.375rem;
            padding: 0.125rem;
            opacity: 0;
            transition: opacity 120ms;
            vertical-align: middle;
          }
          :host(:hover) .copy-btn,
          .copy-btn:focus-visible {
            opacity: 1;
          }
          .copy-btn:hover {
            color: #57534e;
          }
          .copy-btn:focus-visible {
            outline: 2px solid #b45309;
            outline-offset: 2px;
            border-radius: 2px;
            opacity: 1;
          }
          .copy-btn:focus:not(:focus-visible) {
            outline: none;
          }
        `;
        this.shadowRoot.appendChild(style);

        // Hidden slot suppresses light DOM from the accessibility tree
        const hiddenSlot = document.createElement('slot');
        hiddenSlot.style.display = 'none';
        hiddenSlot.setAttribute('aria-hidden', 'true');
        this.shadowRoot.appendChild(hiddenSlot);
    }

    disconnectedCallback() {
        if (this._container) {
            this._container.remove();
            this._container = null;
        }
    }

    connectedCallback() {
        if (this._container) return;

        const container = document.createElement('span');
        container.setAttribute('class', 'container');
        this._container = container;

        const fullText = this.textContent.trim();

        const previewSpan = document.createElement('span');
        const fullTextSpan = document.createElement('span');
        fullTextSpan.id = this.uniqueId;
        fullTextSpan.textContent = fullText;
        fullTextSpan.style.display = 'none';
        fullTextSpan.setAttribute('aria-hidden', 'true');

        if (fullText.length > 30) {
            previewSpan.textContent = fullText.substring(0, 30);

            const ellipsis = document.createElement('span');
            ellipsis.className = 'ellipsis';
            ellipsis.textContent = '...';
            previewSpan.appendChild(ellipsis);
        } else {
            previewSpan.textContent = fullText;
        }

        container.appendChild(previewSpan);
        container.appendChild(fullTextSpan);

        if (fullText.length > 30) {
            const toggleBtn = document.createElement('button');
            toggleBtn.type = 'button';
            toggleBtn.className = 'toggle';
            toggleBtn.textContent = 'show more';
            toggleBtn.setAttribute('aria-expanded', 'false');
            toggleBtn.setAttribute('aria-controls', this.uniqueId);

            toggleBtn.addEventListener('click', () => {
                const isExpanded = fullTextSpan.style.display !== 'none';
                if (!isExpanded) {
                    fullTextSpan.style.display = 'inline';
                    fullTextSpan.setAttribute('aria-hidden', 'false');
                    previewSpan.style.display = 'none';
                    toggleBtn.textContent = 'show less';
                    toggleBtn.setAttribute('aria-expanded', 'true');
                } else {
                    fullTextSpan.style.display = 'none';
                    fullTextSpan.setAttribute('aria-hidden', 'true');
                    previewSpan.style.display = 'inline';
                    toggleBtn.textContent = 'show more';
                    toggleBtn.setAttribute('aria-expanded', 'false');
                }
            });

            const copyBtn = document.createElement('button');
            copyBtn.type = 'button';
            copyBtn.className = 'copy-btn';
            copyBtn.setAttribute('aria-label', 'Copy text to clipboard');
            copyBtn.innerHTML = '<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="5" y="5" width="9" height="9" rx="1.5"/><path d="M5 11H3.5A1.5 1.5 0 012 9.5V3.5A1.5 1.5 0 013.5 2h6A1.5 1.5 0 0111 3.5V5"/></svg>';
            copyBtn.addEventListener('click', () => {
                updateClipboard(fullText);
            });

            container.appendChild(toggleBtn);
            container.appendChild(copyBtn);
        }

        this.shadowRoot.appendChild(container);
    }
}

customElements.define('expandable-text', ExpandableText);

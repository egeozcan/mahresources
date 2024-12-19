class ExpandableText extends HTMLElement {
    constructor() {
        super();
        this.attachShadow({ mode: 'open' });

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
        fullTextSpan.textContent = fullText;
        fullTextSpan.style.display = 'none'; // Initially hidden

        // Create the toggle button (inline) only if the text is longer than 30 characters
        let button = null;
        const copyButton = document.createElement('button');

        if (fullText.length > 30) {
            button = document.createElement('button');
            button.textContent = 'Read more';

            // Handle the expand/collapse logic
            button.addEventListener('click', () => {
                if (fullTextSpan.style.display === 'none') {
                    fullTextSpan.style.display = 'inline';
                    previewSpan.style.display = 'none';
                    button.textContent = 'Read less';
                } else {
                    fullTextSpan.style.display = 'none';
                    previewSpan.style.display = 'inline';
                    button.textContent = 'Read more';
                }
            });

            copyButton.textContent = 'Copy';
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
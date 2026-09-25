/**
 * Create a visually hidden ARIA live region for screen reader announcements.
 * @param {HTMLElement} [parent=document.body] - The parent element to append the region to.
 * @param {{ assertive?: boolean }} [options] - assertive=true uses role="alert"/aria-live="assertive" (for errors); default is polite.
 * @returns {{ element: HTMLElement, announce: (message: string) => void, cancel: () => (string|null), destroy: () => void }}
 */
export function createLiveRegion(parent = document.body, { assertive = false } = {}) {
    const element = document.createElement('div');
    element.setAttribute('role', assertive ? 'alert' : 'status');
    element.setAttribute('aria-live', assertive ? 'assertive' : 'polite');
    element.setAttribute('aria-atomic', 'true');
    Object.assign(element.style, {
        position: 'absolute',
        width: '1px',
        height: '1px',
        padding: '0',
        margin: '-1px',
        overflow: 'hidden',
        clip: 'rect(0, 0, 0, 0)',
        whiteSpace: 'nowrap',
        border: '0'
    });
    parent.appendChild(element);

    let announceTimeout = null;
    let pendingMessage = null;

    function announce(message) {
        if (announceTimeout) {
            clearTimeout(announceTimeout);
        }
        element.textContent = '';
        pendingMessage = message;
        announceTimeout = setTimeout(() => {
            announceTimeout = null;
            pendingMessage = null;
            element.textContent = message;
        }, 50);
    }

    // Withdraws a message that has not landed yet and returns it, or null. For a
    // caller that announces through a second region of its own (a modal dialog's):
    // a newer message there must replace this one, not be followed by it.
    function cancel() {
        if (announceTimeout) {
            clearTimeout(announceTimeout);
            announceTimeout = null;
        }
        const message = pendingMessage;
        pendingMessage = null;
        return message;
    }

    function destroy() {
        if (announceTimeout) {
            clearTimeout(announceTimeout);
            announceTimeout = null;
        }
        if (element.parentNode) {
            element.parentNode.removeChild(element);
        }
    }

    return { element, announce, cancel, destroy };
}

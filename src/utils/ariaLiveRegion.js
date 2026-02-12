/**
 * Create a visually hidden ARIA live region for screen reader announcements.
 * @param {HTMLElement} [parent=document.body] - The parent element to append the region to.
 * @returns {{ element: HTMLElement, announce: (message: string) => void, destroy: () => void }}
 */
export function createLiveRegion(parent = document.body) {
    const element = document.createElement('div');
    element.setAttribute('role', 'status');
    element.setAttribute('aria-live', 'polite');
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

    function announce(message) {
        if (announceTimeout) {
            clearTimeout(announceTimeout);
        }
        element.textContent = '';
        announceTimeout = setTimeout(() => {
            element.textContent = message;
        }, 50);
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

    return { element, announce, destroy };
}

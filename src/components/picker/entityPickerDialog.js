import { createFocusTrap } from 'focus-trap';

/**
 * One trap for the whole session, not one per retained filter step.
 * Alpine's x-trap activates on an uncancelled timer; closing before it fires
 * can trap a hidden dialog (and leave its background hidden to assistive tech).
 * Own the render generation so a cancelled open cannot acquire anything later.
 */
export function createEntityPickerDialog(root, {
    afterRender = queueMicrotask,
    trapFactory = createFocusTrap,
} = {}) {
    const trap = trapFactory(root, {
        escapeDeactivates: false,
        allowOutsideClick: true,
        initialFocus: false,
        fallbackFocus: root,
        delayInitialFocus: false,
    });
    let open = false;
    let destroyed = false;
    let generation = 0;
    let release = () => {};

    function setOpen(value) {
        if (destroyed || open === value) return;
        open = value;
        const current = ++generation;
        if (!open) {
            trap.deactivate({ returnFocus: false });
            release();
            release = () => {};
            return;
        }
        afterRender(() => {
            if (!open || destroyed || current !== generation || !root.isConnected) return;
            const previous = [];
            for (let branch = root; branch.parentElement; branch = branch.parentElement) {
                for (const sibling of branch.parentElement.children) {
                    if (sibling !== branch && 'inert' in sibling) {
                        previous.push([sibling, sibling.inert]);
                        sibling.inert = true;
                    }
                }
                if (branch.parentElement === root.ownerDocument.body) break;
            }
            const html = root.ownerDocument.documentElement;
            const win = root.ownerDocument.defaultView;
            const overflow = html.style.overflow;
            const padding = html.style.paddingRight;
            const gap = Math.max(0, win.innerWidth - html.clientWidth);
            if (gap) {
                html.style.paddingRight = `${(parseFloat(win.getComputedStyle(html).paddingRight) || 0) + gap}px`;
            }
            html.style.overflow = 'hidden';
            release = () => {
                for (const [node, inert] of previous) node.inert = inert;
                html.style.overflow = overflow;
                html.style.paddingRight = padding;
            };
            trap.activate();
        });
    }

    return {
        setOpen,
        destroy() {
            setOpen(false);
            destroyed = true;
        },
    };
}

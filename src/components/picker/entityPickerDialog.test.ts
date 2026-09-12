// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest';
import { createEntityPickerDialog } from './entityPickerDialog.js';

afterEach(() => {document.body.innerHTML = '';document.documentElement.style.overflow = '';document.documentElement.style.paddingRight = '';});
function setup() {
 document.body.innerHTML = '<main aria-hidden="false"></main><aside inert></aside><div role="dialog" tabindex="-1"></div>';
 const root = document.body.lastElementChild as HTMLElement;
 const queued: Array<() => void> = [];
 const trap = { activate: vi.fn(), deactivate: vi.fn() };
 const controller = createEntityPickerDialog(root, { afterRender: (fn: () => void) => queued.push(fn), trapFactory: () => trap });
 return { controller, queued, trap, root, background: document.body.firstElementChild as HTMLElement };
}
it('never activates a dialog closed before its render callback runs', () => {
 const { controller, queued, trap, background } = setup();
 controller.setOpen(true);controller.setOpen(false);queued.shift()!();
 expect(trap.activate).not.toHaveBeenCalled();expect(background.inert).toBe(false);
 expect(document.documentElement.style.overflow).toBe('');
});
it('owns background inertness and scrolling only while open, preserving previous values', () => {
 const { controller, queued, trap, background } = setup();
 document.documentElement.style.overflow = 'auto';
 controller.setOpen(true);queued.shift()!();
 expect(trap.activate).toHaveBeenCalledOnce();expect(background.inert).toBe(true);expect(document.documentElement.style.overflow).toBe('hidden');
 controller.setOpen(true);expect(queued).toHaveLength(0); // A nested step is not another trap.
 controller.setOpen(false);
 expect(trap.deactivate).toHaveBeenCalledWith({ returnFocus: false });expect(background.inert).toBe(false);
 expect(background.getAttribute('aria-hidden')).toBe('false');expect((document.querySelector('aside') as HTMLElement).inert).toBe(true);
 expect(document.documentElement.style.overflow).toBe('auto');
});
it('ignores stale opens across replacement and destruction', () => {
 const { controller, queued, trap } = setup();
 controller.setOpen(true);controller.setOpen(false);controller.setOpen(true);
 queued.shift()!();expect(trap.activate).not.toHaveBeenCalled();queued.shift()!();expect(trap.activate).toHaveBeenCalledOnce();
 controller.destroy();controller.setOpen(true);expect(queued).toHaveLength(0);
});

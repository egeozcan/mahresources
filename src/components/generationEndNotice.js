// The signal that a template generation has ended. A run can take minutes, and
// its result lands in a panel the reader may have scrolled away from or a tab
// they left. When the result is on screen nothing extra is shown: the panel
// and its status line are the signal. Otherwise a notice in the corner says how
// it ended and takes the reader to the result, and a background tab's title
// carries the outcome until the tab is looked at again.
//
// The notice is not a live region: each generation control already announces
// its end in its own status region, and a second announcement would repeat it.

const TITLE_MARK = {
  ready: '(Draft ready) ',
  review: '(Draft needs review) ',
  failed: '(Generation failed) ',
};

const BORDER = {
  ready: 'border-green-700',
  review: 'border-amber-600',
  failed: 'border-red-700',
};

const AUTO_DISMISS_MS = 15000;

let currentMark = '';
let toast = null;
let timer = 0;
// Bumped by every dismissal: a notice still waiting for its frames is dropped.
let generation = 0;
// What holds the notice open: the pointer on it, focus in it. Either one is enough.
let hovered = false;
let focused = false;
// Where focus was when the notice appeared, for a keyboard dismissal to return to.
let returnFocus = null;

// A dialog open over the page: the page behind it is inert, so the notice
// must not lead there, and Escape belongs to the dialog.
function modalOpen() {
  return [...document.querySelectorAll('[aria-modal="true"]')].some((el) => el.getClientRects().length > 0);
}

function onDocumentKey(event) {
  if (event.key === 'Escape' && toast && (toast.contains(event.target) || !modalOpen())) dismissGenerationNotice();
}

function stripMark(title) {
  return currentMark && title.startsWith(currentMark) ? title.slice(currentMark.length) : title;
}

function clearTitleMark() {
  if (!currentMark) return;
  document.title = stripMark(document.title);
  currentMark = '';
}

function onVisibility() {
  if (document.visibilityState === 'visible') {
    clearTitleMark();
    document.removeEventListener('visibilitychange', onVisibility);
  }
}

function markTitle(kind) {
  const mark = TITLE_MARK[kind];
  if (!mark) return;
  const base = stripMark(document.title);
  currentMark = mark;
  document.title = mark + base;
  document.addEventListener('visibilitychange', onVisibility);
}

function onScreen(el) {
  if (!el || !el.isConnected) return false;
  // A closed <details> can leave its hidden content a nonzero box.
  if (el.closest('details:not([open])')) return false;
  if (typeof el.checkVisibility === 'function' && !el.checkVisibility()) return false;
  const rect = el.getBoundingClientRect();
  if (!rect.width && !rect.height) return false;
  return rect.bottom > 0 && rect.top < (window.innerHeight || document.documentElement.clientHeight);
}

// Bring el into view: a closed <details> it sits in is opened first, or there
// is nothing to scroll to.
function reveal(el) {
  for (let details = el.closest('details:not([open])'); details; details = details.parentElement?.closest('details:not([open])')) {
    details.open = true;
  }
  // A panel taller than the window is aligned by its top, where its heading and summary are.
  const tall = el.getBoundingClientRect().height > (window.innerHeight || document.documentElement.clientHeight) * 0.8;
  el.scrollIntoView({ block: tall ? 'start' : 'center', behavior: 'smooth' });
  el.focus({ preventScroll: true });
}

function armTimer() {
  clearTimeout(timer);
  if (hovered || focused) return;
  timer = setTimeout(dismissGenerationNotice, AUTO_DISMISS_MS);
}

function button(label, testId, primary) {
  const el = document.createElement('button');
  el.type = 'button';
  el.textContent = label;
  el.dataset.testid = testId;
  el.className = primary
    ? 'px-3 py-1 text-xs font-mono font-medium text-white bg-amber-700 border border-transparent rounded-md hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer whitespace-nowrap'
    : 'px-3 py-1 text-xs font-mono text-stone-700 bg-white border border-stone-300 rounded-md hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600 cursor-pointer whitespace-nowrap';
  return el;
}

function showToast({ kind, heading, detail, locate }) {
  dismissGenerationNotice();
  returnFocus = document.activeElement && document.activeElement !== document.body ? document.activeElement : null;
  const box = document.createElement('div');
  box.dataset.testid = 'generation-end-notice';
  box.dataset.kind = kind;
  box.setAttribute('aria-label', heading);
  box.setAttribute('role', 'group');
  box.className = `fixed bottom-4 left-4 right-4 sm:left-auto sm:w-96 z-30 rounded-md border border-stone-300 border-l-4 ${BORDER[kind] || 'border-stone-500'} bg-white p-3 shadow-lg`;

  const title = document.createElement('p');
  title.className = 'text-sm font-semibold font-mono text-stone-900';
  title.textContent = heading;
  box.appendChild(title);
  if (detail) {
    const text = document.createElement('p');
    text.className = 'mt-1 text-xs text-stone-700';
    text.textContent = detail;
    box.appendChild(text);
  }
  const actions = document.createElement('div');
  actions.className = 'mt-2 flex flex-wrap gap-2';
  const show = button(kind === 'failed' ? 'Show' : 'Review', 'generation-end-notice-show', true);
  show.addEventListener('click', () => {
    // Behind an open dialog the result is inert and cannot take focus: wait.
    if (modalOpen()) return;
    const el = locate?.();
    returnFocus = null;
    dismissGenerationNotice();
    if (el && el.isConnected) reveal(el);
  });
  const close = button('Dismiss', 'generation-end-notice-dismiss', false);
  close.addEventListener('click', dismissGenerationNotice);
  actions.append(show, close);
  box.appendChild(actions);

  // Held open while the reader is on it.
  box.addEventListener('mouseenter', () => { hovered = true; clearTimeout(timer); });
  box.addEventListener('mouseleave', () => { hovered = false; armTimer(); });
  box.addEventListener('focusin', () => { focused = true; clearTimeout(timer); });
  box.addEventListener('focusout', (event) => {
    if (box.contains(event.relatedTarget)) return;
    focused = false;
    armTimer();
  });

  document.body.appendChild(box);
  toast = box;
  // Escape works from wherever focus is, since the notice leaves focus where it was.
  document.addEventListener('keydown', onDocumentKey);
  armTimer();
}

/**
 * Signal the end of a generation the reader sent.
 * @param {{ kind: 'ready'|'review'|'failed', heading: string, detail?: string,
 *   locate: () => (HTMLElement|null), immediate?: boolean }} options
 *   locate finds the element holding the result (the draft panel, or the status
 *   line for a failure); it is measured after the next frame so a panel Alpine
 *   is still rendering counts. immediate measures now (tests).
 */
export function noticeGenerationEnd({ kind, heading, detail = '', locate, immediate = false }) {
  const hidden = document.visibilityState === 'hidden' || document.hidden;
  if (hidden) markTitle(kind);
  const ticket = generation;
  // Visibility is read again when deciding: frames are held while the tab is in
  // the background, so a result on screen when the reader returns needs no notice.
  const decide = () => {
    if (ticket !== generation) return;
    if (document.visibilityState === 'hidden' || !onScreen(locate?.())) showToast({ kind, heading, detail, locate });
  };
  if (immediate || typeof requestAnimationFrame !== 'function') decide();
  else requestAnimationFrame(() => requestAnimationFrame(decide));
}

/** Remove the notice, if one is showing. A new generation, Apply and Discard call this. */
export function dismissGenerationNotice() {
  generation++;
  clearTimeout(timer);
  timer = 0;
  hovered = false;
  focused = false;
  document.removeEventListener('keydown', onDocumentKey);
  if (toast) {
    // Removing the focused control would drop focus on <body>: it goes back
    // to where the reader was when the notice appeared.
    const hadFocus = toast.contains(document.activeElement);
    toast.remove();
    toast = null;
    if (hadFocus && returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
  }
  returnFocus = null;
}

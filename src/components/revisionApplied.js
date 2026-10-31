// One way for any form control to say where content it just wrote came from.
// The templateBundle component on the carrier form listens and posts the
// entries as revisionApplied with the save (see recordApplied there).
export const REVISION_APPLIED_EVENT = 'revision-applied';

export function announceApplied(entry) {
  document.dispatchEvent(new CustomEvent(REVISION_APPLIED_EVENT, { detail: { entry } }));
}

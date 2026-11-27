// A refresh can replace a slot and move its tab sets. Only carry selection and
// IDs across a one-to-one match of the server's source-and-entity identity.
function hostsByKey(container) {
  const hosts = new Map();
  const nodes = Array.from(container.querySelectorAll('tabs-shortcode[data-tabs-key]'));
  if (container.matches?.('tabs-shortcode[data-tabs-key]')) nodes.unshift(container);
  const own = nodes.filter((host) => {
    // A containing [lazy]/[details] comes back unrevealed, not with its old body.
    const deferred = host.parentElement?.closest('lazy-shortcode, details-shortcode');
    return !deferred || !container.contains(deferred);
  });
  for (const host of own) {
    const key = host.getAttribute('data-tabs-key');
    hosts.set(key, hosts.has(key) ? null : host);
  }
  return { hosts, nodes: own };
}

export function captureTabSelections(container) {
  const saved = new Map();
  for (const [key, host] of hostsByKey(container).hosts) {
    if (host?._init) saved.set(key, { uid: host._uid, selected: host._selected });
  }
  return saved;
}

export function restoreTabSelections(container, saved) {
  const { hosts, nodes } = hostsByKey(container);
  for (const host of nodes) {
    const key = host.getAttribute('data-tabs-key');
    host._restoreSelection?.(hosts.get(key) === host ? saved.get(key) : undefined);
  }
}

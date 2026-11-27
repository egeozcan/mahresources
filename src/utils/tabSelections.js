// A refresh can replace a slot and move its tab sets. Carry selection, IDs and
// focus only across a one-to-one match of the server's source-and-entity identity.
function hostsByKey(container) {
  const hosts = new Map();
  const nodes = Array.from(container.querySelectorAll('tabs-shortcode[data-tabs-key]'));
  if (container.matches?.('tabs-shortcode[data-tabs-key]')) nodes.unshift(container);
  const own = nodes.filter((host) => {
    // A containing [lazy]/[details] comes back unrevealed, not with its old body.
    const deferred = host.parentElement?.closest('lazy-shortcode, details-shortcode');
    return !deferred || deferred === container || !container.contains(deferred);
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

// A focus target must match the same unique set on both sides, just like
// selection. A lookup that finds nothing means park focus, not a positional
// match in a different set. null from a capture means focus was not in a tab.
function matchTabHost(container, host) {
  const key = host.getAttribute('data-tabs-key');
  const unique = hostsByKey(container).hosts.get(key) === host;
  return () => unique ? hostsByKey(container).hosts.get(key) : null;
}

export function captureTabFocus(container, active) {
  const host = active?.closest('tabs-shortcode[data-tabs-key]');
  if (!host?._init || !container.contains(host)) return null;
  const index = host._tabs().indexOf(active);
  if (index < 0) return null;
  const findHost = matchTabHost(container, host);
  return () => findHost()?._tabs()[index];
}

export function captureTabReloadButton(container, button, reloadButtons) {
  const host = button.closest('tabs-shortcode[data-tabs-key]');
  if (!host?._init || !container.contains(host)) return null;
  const panel = button.closest('.tabs-panel');
  const panelIndex = host._panels().indexOf(panel);
  if (panelIndex < 0) return null;
  const buttonsIn = (panel) => reloadButtons(container).filter((btn) => btn.closest('.tabs-panel') === panel);
  const buttonIndex = buttonsIn(panel).indexOf(button);
  if (buttonIndex < 0) return null;
  const findHost = matchTabHost(container, host);
  return () => {
    const nextPanel = findHost()?._panels()[panelIndex];
    return nextPanel ? buttonsIn(nextPanel)[buttonIndex] : null;
  };
}

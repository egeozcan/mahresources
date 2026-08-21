/**
 * Node identity for plugin-rendered HTML.
 *
 * Lit renders a DOM node by identity. Both plugin display renderers used to
 * build a fresh wrapper element, set innerHTML on it and hand the detached node
 * to Lit on *every* render — so the node was a different one each time, Lit
 * swapped the live one out, and anything the plugin's markup was holding went
 * with it: an Alpine component's state, a custom element's internals, focus,
 * scroll position, a running animation. The host re-renders for reasons that
 * have nothing to do with the plugin's field, so that happened often and
 * silently.
 *
 * Holding the built node and returning the same one while the HTML is unchanged
 * is the whole fix. The cache is keyed by HTML rather than merely by field,
 * because a genuinely new render *must* produce a new node.
 */
export interface CachedPluginNode {
  html: string;
  node: HTMLElement;
}

export class PluginNodeCache {
  private nodes: Record<string, CachedPluginNode> = {};

  /**
   * Returns the node for `key`, rebuilding it only when `htmlSource` differs
   * from the HTML the cached node was built from.
   */
  nodeFor(key: string, htmlSource: string, tagName = 'div'): HTMLElement {
    const cached = this.nodes[key];
    if (cached !== undefined && cached.html === htmlSource) {
      return cached.node;
    }
    const node = document.createElement(tagName);
    node.innerHTML = htmlSource;
    this.nodes[key] = { html: htmlSource, node };
    return node;
  }

  /**
   * Drops every cached node. Call wherever the cached *HTML* is dropped: a node
   * outliving the HTML it was built from is the stale-render bug this class
   * exists to avoid, pointing the wrong way round.
   */
  clear(): void {
    this.nodes = {};
  }
}

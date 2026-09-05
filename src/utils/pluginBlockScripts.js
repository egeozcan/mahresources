// A block's runtime belongs to its registered plugin, never to user-authored
// block content. The server sends these URLs only after authorizing its render.
const scriptLoads = new Map();

export async function loadPluginBlockScripts(blockType, scripts = []) {
  const pluginName = /^plugin:([a-zA-Z0-9_-]+):/.exec(blockType)?.[1];
  const prefix = `/plugins/${pluginName}/public/`;
  for (const source of scripts) {
    const url = new URL(source, window.location.origin);
    if (!pluginName || url.origin !== window.location.origin || !source.startsWith(prefix)
      || !url.pathname.startsWith(prefix) || !url.pathname.endsWith('.js') || url.search || url.hash) {
      throw new Error('Invalid plugin block script');
    }
    let loaded = scriptLoads.get(url.href);
    if (!loaded) {
      loaded = new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = url.href;
        script.onload = () => resolve();
        script.onerror = () => {
          script.remove();
          scriptLoads.delete(url.href);
          reject(new Error(`Unable to load plugin block script: ${url.pathname}`));
        };
        document.head.append(script);
      });
      scriptLoads.set(url.href, loaded);
    }
    await loaded;
  }
}

/* Shared PM presentation rules for native controls and plugin views. */
(function () {
  'use strict';
  if (window.PMCore) return;
  function meta(task) {
    var value = task.meta || task.Meta || {};
    if (typeof value === 'string') { try { return JSON.parse(value); } catch (_) { return {}; } }
    return value;
  }
  function status(task, cfg) { return meta(task).status || cfg.default_status; }
  function localInput(iso) { return (String(iso || '').match(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/) || [''])[0]; }
  function overdue(task, cfg, today) {
    var due = task.end_date || task.EndDate;
    return !!due && status(task, cfg) !== cfg.done_status && due.slice(0, 10) < (today || new Date().toISOString().slice(0, 10));
  }
  function pill(entry) {
    var node = document.createElement('span');
    node.className = 'pm-pill';
    node.textContent = entry.label || entry.name;
    node.style.setProperty('--pm-color', entry.color || '#6b7280');
    node.style.color = entry.text_color || '#292524';
    return node;
  }
  async function api(path, body) {
    var headers = {};
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      headers['X-CSRF-Token'] = document.querySelector('meta[name="csrf-token"]')?.content || '';
    }
    var response = await fetch('/v1/plugins/project-management' + path, {
      method: body === undefined ? 'GET' : 'POST', headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    var result = await response.json();
    if (!response.ok || result.error) throw new Error(result.error || 'Request failed (' + response.status + ')');
    return result;
  }
  var configPromise;
  window.PMCore = { meta, status, localInput, overdue, pill, api,
    config() { return configPromise || (configPromise = api('/api/config').catch(error => { configPromise = null; throw error; })); },
  };
})();

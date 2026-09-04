/**
 * Project Management plugin — client-side views.
 *
 * The plugin page is a shell (#pm-app); this file renders the four views
 * (board, backlog, dashboard, timeline) into #pm-root. Data comes from the
 * host's own /v1/notes endpoint (the one read surface that carries a task's
 * Tags and dates); every mutation goes to the plugin's mah.api handlers,
 * which validate statuses, dates and ordering server-side.
 *
 * A classic, non-module script (see docs/features/plugin-hooks.md): it runs
 * before Alpine starts, so no work happens at parse time. All fetching and
 * wiring starts on DOMContentLoaded, by which point src/csrf.js has installed
 * its window.fetch wrapper. The CSRF token is also read from the <meta> tag
 * and sent explicitly, so a mutation never depends on that timing.
 */
(function () {
  'use strict';

  var PLUGIN_BASE = '/plugins/project-management';
  var API_BASE = '/v1/plugins/project-management';
  var NOTES_URL = '/v1/notes';
  var PAGE_SIZE = 50;

  // ---------------------------------------------------------------------
  // Small utilities
  // ---------------------------------------------------------------------

  function el(tag, attrs, children) {
    var node = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (k) {
        var v = attrs[k];
        if (v == null) return;
        if (k === 'class') node.className = v;
        else if (k === 'text') node.textContent = v;
        else if (k === 'html') node.innerHTML = v;
        else if (k.indexOf('data-') === 0) node.setAttribute(k, v);
        else if (k.indexOf('aria-') === 0 || k === 'role' || k === 'tabindex' || k === 'draggable') {
          node.setAttribute(k, v);
        } else {
          node[k] = v;
        }
      });
    }
    if (children) {
      (Array.isArray(children) ? children : [children]).forEach(function (c) {
        if (c == null) return;
        node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
      });
    }
    return node;
  }

  function escapeHtml(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function csrfToken() {
    var m = document.querySelector('meta[name="csrf-token"]');
    return m ? m.getAttribute('content') : '';
  }

  function getParam(name) {
    return new URLSearchParams(window.location.search).get(name);
  }

  function pushParams(params) {
    var url = new URL(window.location.href);
    Object.keys(params).forEach(function (k) {
      if (params[k] == null || params[k] === '') url.searchParams.delete(k);
      else url.searchParams.set(k, params[k]);
    });
    history.replaceState(null, '', url.toString());
  }

  async function apiFetch(path, opts) {
    opts = opts || {};
    var headers = opts.headers || {};
    if (opts.json !== undefined) {
      headers['Content-Type'] = 'application/json';
      headers['X-CSRF-Token'] = csrfToken();
    }
    var res = await fetch(API_BASE + path, {
      method: opts.method || 'GET',
      headers: headers,
      body: opts.json !== undefined ? JSON.stringify(opts.json) : undefined,
    });
    if (!res.ok) {
      var bodyText = '';
      try { bodyText = await res.text(); } catch (e) { /* ignore */ }
      var msg = 'Request failed (' + res.status + ')';
      try {
        var parsed = JSON.parse(bodyText);
        if (parsed && parsed.error) msg = parsed.error;
      } catch (e) { /* not json */ }
      throw new Error(msg);
    }
    if (res.status === 204) return null;
    return res.json();
  }

  /** Fetch task notes through the host listing endpoint. */
  async function fetchTasks(params) {
    var qs = new URLSearchParams();
    var extraMRQL = params && params.MRQL;
    if (state.container.type === 'project') {
      var scopeMRQL = '(owner.id = ' + state.container.id + ' OR ancestors.id = ' + state.container.id + ')';
      if (extraMRQL) scopeMRQL += ' AND (' + extraMRQL + ')';
      qs.append('MRQL', scopeMRQL);
    } else {
      qs.append('OwnerId', String(state.container.id));
      if (extraMRQL) qs.append('MRQL', extraMRQL);
    }
    if (state.cfg.task_type_id) qs.append('NoteTypeId', String(state.cfg.task_type_id));
    Object.keys(params || {}).forEach(function (k) {
      if (k === 'MRQL') return;
      var v = params[k];
      if (v == null) return;
      if (Array.isArray(v)) v.forEach(function (x) { qs.append(k, x); });
      else qs.append(k, v);
    });
    var res = await fetch(NOTES_URL + '?' + qs.toString(), {
      headers: { Accept: 'application/json' },
    });
    if (!res.ok) {
      throw new Error('could not load tasks (' + res.status + ')');
    }
    return asArray(await res.json());
  }

  /** Fetch every page of tasks matching the filter. */
  async function fetchTaskPages(params) {
    var all = [];
    for (var page = 1; ; page++) {
      var p = Object.assign({}, params || {});
      p.page = String(page);
      p.SortBy = "meta->>'order'";
      var batch = await fetchTasks(p);
      for (var i = 0; i < batch.length; i++) all.push(batch[i]);
      if (batch.length < PAGE_SIZE) break;
    }
    return all;
  }

  function byId(id) { return document.getElementById(id); }

  function asArray(value) { return Array.isArray(value) ? value : []; }

  function statusEntry(name) {
    for (var i = 0; i < state.cfg.statuses.length; i++) {
      if (state.cfg.statuses[i].name === name) return state.cfg.statuses[i];
    }
    return { name: name, label: name, color: '#78716c' };
  }

  function effectiveStatus(task) {
    return ((task.Meta || {}).status) || state.cfg.default_status;
  }

  function columnFetchParams(status, page) {
    var params = { SortBy: "meta->>'order'", page: String(page) };
    if (status.name === state.cfg.default_status) {
      params.MRQL = '(meta.status = ' + JSON.stringify(status.name) + ' OR meta.status IS EMPTY)';
    } else {
      params.MetaQuery = ['status:EQ:"' + status.name + '"'];
    }
    return params;
  }

  function priorityEntry(name) {
    for (var i = 0; i < state.cfg.priorities.length; i++) {
      if (state.cfg.priorities[i].name === name) return state.cfg.priorities[i];
    }
    return { name: name, label: name, color: '#78716c' };
  }

  // The host stores note start/end dates as the naive wall-clock value the
  // writer sent, parsed as UTC and serialized back as YYYY-MM-DDTHH:MM:SSZ.
  // All date work here therefore reads the wall-clock text directly — a Date
  // would shift it into the viewer's zone and misplace timeline buckets.
  function naiveParts(iso) {
    if (!iso) return null;
    var m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(iso);
    if (!m) return null;
    return { y: m[1], mo: m[2], d: m[3], h: m[4], mi: m[5] };
  }

  function pad2(n) { return (n < 10 ? '0' : '') + n; }

  function localNaiveNow() {
    var now = new Date();
    return now.getFullYear() + '-' + pad2(now.getMonth() + 1) + '-' + pad2(now.getDate()) +
      'T' + pad2(now.getHours()) + ':' + pad2(now.getMinutes());
  }

  function formatDate(iso) {
    var p = naiveParts(iso);
    if (!p) return iso || '';
    var months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    var out = months[Number(p.mo) - 1] + ' ' + Number(p.d) + ', ' + p.y;
    if (p.h !== '00' || p.mi !== '00') out += ' ' + Number(p.h) + ':' + p.mi;
    return out;
  }

  // Whole days from today (local calendar) to the due date; negative = past.
  function dueInDays(iso) {
    var p = naiveParts(iso);
    if (!p) return null;
    var now = new Date();
    var todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    var dueStart = new Date(Number(p.y), Number(p.mo) - 1, Number(p.d));
    return Math.round((dueStart - todayStart) / 86400000);
  }

  // ---------------------------------------------------------------------
  // State
  // ---------------------------------------------------------------------

  var state = {
    cfg: null,
    container: null,     // { type: 'project'|'epic', id, name }
    view: 'board',
    projects: [],
    unassigned: [],
    epics: [],           // epics of the current project
    allTags: [],         // global tags for filters
    live: null,          // aria-live node
  };

  function announce(msg) {
    if (!state.live) return;
    state.live.textContent = '';
    // force reflow so a repeat announcement is read again
    void state.live.offsetWidth;
    state.live.textContent = msg;
  }

  // ---------------------------------------------------------------------
  // Boot
  // ---------------------------------------------------------------------

  async function boot() {
    var root = byId('pm-root');
    state.live = el('div', { class: 'pm-announce', 'aria-live': 'polite', role: 'status' });
    byId('pm-app').appendChild(state.live);

    var cfg = null;
    try {
      cfg = await apiFetch('/api/config');
    } catch (e) {
      renderError(root, 'Could not reach the plugin API: ' + e.message);
      return;
    }
    state.cfg = cfg;
    state.cfg.statuses = asArray(state.cfg.statuses);
    state.cfg.priorities = asArray(state.cfg.priorities);

    if (!cfg.configured) {
      renderSetup(root);
      return;
    }

    try {
      var projectsRes = await apiFetch('/api/projects');
      state.projects = asArray(projectsRes.projects);
      state.unassigned = asArray(projectsRes.unassigned);
    } catch (e) { /* picker renders empty */ }

    // Container resolution: ?epic wins over ?project.
    var epicParam = getParam('epic');
    var projectParam = getParam('project');
    var container = null;
    if (epicParam && !isNaN(parseInt(epicParam, 10))) {
      container = { type: 'epic', id: parseInt(epicParam, 10), name: '' };
    } else if (projectParam && !isNaN(parseInt(projectParam, 10))) {
      container = { type: 'project', id: parseInt(projectParam, 10), name: '' };
    }
    if (container) {
      state.container = container;
      var viewParam = getParam('view');
      if (viewParam && ['board', 'backlog', 'dashboard', 'timeline'].indexOf(viewParam) >= 0) {
        state.view = viewParam;
      }
      await enterContainer();
    } else {
      renderPicker(root);
    }
  }

  async function enterContainer() {
    var root = byId('pm-root');
    root.textContent = '';
    var c = state.container;
    try {
      if (c.type === 'project') {
        var epicsRes = await apiFetch('/api/epics?project=' + c.id);
        c.name = (epicsRes.project && epicsRes.project.name) || c.name;
        state.epics = asArray(epicsRes.epics);
      } else {
        state.epics = [];
        var epicRes = await apiFetch('/api/epic?epic=' + c.id);
        c.name = (epicRes.epic && epicRes.epic.name) || c.name;
        c.project = epicRes.epic && epicRes.epic.project;
      }
      if (!state.allTags.length) {
        try {
          var tagsRes = await fetch('/v1/tags', { headers: { Accept: 'application/json' } });
          state.allTags = tagsRes.ok ? asArray(await tagsRes.json()) : [];
        } catch (e) { state.allTags = []; }
      }
    } catch (e) {
      renderError(root, e.message);
      return;
    }
    renderApp();
  }

  // ---------------------------------------------------------------------
  // Screens
  // ---------------------------------------------------------------------

  function renderError(root, message) {
    root.textContent = '';
    root.appendChild(el('div', { class: 'pm-error', role: 'alert' }, message));
  }

  function renderSetup(root) {
    root.textContent = '';
    root.appendChild(el('h3', { text: 'Not set up yet' }));
    root.appendChild(el('p', null, [
      'Project Management stores its projects, epics and tasks as ordinary ',
      'groups and notes. One-time setup creates the PM Project and PM Epic ',
      'group categories and the PM Task note type.',
    ]));
    var btn = el('button', { class: 'pm-btn pm-btn-primary', type: 'button', 'data-testid': 'pm-setup' }, 'Set up Project Management');
    btn.addEventListener('click', async function () {
      btn.disabled = true;
      try {
        await apiFetch('/api/setup', { method: 'POST', json: {} });
        window.location.reload();
      } catch (e) {
        btn.disabled = false;
        root.appendChild(el('div', { class: 'pm-error', role: 'alert' }, e.message));
      }
    });
    root.appendChild(btn);
  }

  function renderPicker(root) {
    root.textContent = '';
    root.appendChild(el('p', null, 'Pick a project to open its board.'));

    var list = el('ul', { class: 'pm-picker' });
    if (state.projects.length === 0) {
      list.appendChild(el('li', null, 'No projects yet.'));
    }
    state.projects.forEach(function (p) {
      var key = p.key ? el('span', { class: 'pm-picker-key', text: p.key }) : null;
      var status = p.status ? el('span', { class: 'pm-picker-status', text: statusEntry(p.status).label }) : null;
      var row = el('li', null, [
        el('a', { href: PLUGIN_BASE + '/board?project=' + p.id + '&view=board' },
          [el('span', { class: 'pm-picker-name', text: p.name }), key, status]),
      ]);
      list.appendChild(row);
    });

    if (state.unassigned.length) {
      list.appendChild(el('li', { class: 'pm-picker-group', 'data-testid': 'pm-unassigned' },
        el('strong', { text: 'Unassigned epics (their project group was deleted)' })));
      state.unassigned.forEach(function (ep) {
        list.appendChild(el('li', null, [
          el('a', { href: PLUGIN_BASE + '/board?epic=' + ep.id + '&view=board' },
            [el('span', { class: 'pm-picker-name', text: ep.name })]),
        ]));
      });
    }

    root.appendChild(list);
    root.appendChild(el('p', null, [
      'Start a new project by creating a group with the ',
      el('a', { href: '/group/new', text: 'PM Project category' }),
      '.',
    ]));
  }

  function viewTabs() {
    var views = [
      { key: 'board', label: 'Board' },
      { key: 'backlog', label: 'Backlog' },
      { key: 'dashboard', label: 'Dashboard' },
      { key: 'timeline', label: 'Timeline' },
    ];
    var tabs = el('div', { class: 'pm-view-tabs', role: 'tablist', 'aria-label': 'Project views' });
    views.forEach(function (v) {
      var tab = el('button', {
        class: 'pm-view-tab', type: 'button', 'data-view': v.key, text: v.label,
        role: 'tab', 'aria-selected': state.view === v.key ? 'true' : 'false',
        id: 'pm-tab-' + v.key, 'aria-controls': 'pm-panel-' + v.key,
        tabindex: state.view === v.key ? '0' : '-1',
      });
      tab.addEventListener('click', function () {
        switchView(v.key, true);
      });
      tab.addEventListener('keydown', function (event) {
        var index = views.findIndex(function (candidate) { return candidate.key === v.key; });
        var next = index;
        if (event.key === 'ArrowRight') next = (index + 1) % views.length;
        else if (event.key === 'ArrowLeft') next = (index - 1 + views.length) % views.length;
        else if (event.key === 'Home') next = 0;
        else if (event.key === 'End') next = views.length - 1;
        else return;
        event.preventDefault();
        switchView(views[next].key, true);
      });
      tabs.appendChild(tab);
    });
    return tabs;
  }

  function switchView(view, focusTab) {
    state.view = view;
    pushParams({ view: view });
    renderApp();
    if (focusTab) {
      var activeTab = byId('pm-tab-' + view);
      if (activeTab) activeTab.focus();
    }
  }

  function renderApp() {
    var root = byId('pm-root');
    root.textContent = '';

    var title = el('div', { class: 'pm-toolbar' });
    title.appendChild(viewTabs());

    var c = state.container;
    var heading = el('h3', { class: 'pm-container-title', text: c.name || (c.type === 'epic' ? 'Epic' : 'Project') });
    if (c.type === 'project') {
      heading = el('h3', { class: 'pm-container-title' });
      var link = el('a', { href: '/group?id=' + c.id, text: c.name || ('Project ' + c.id) });
      heading.appendChild(link);
    } else {
      heading = el('h3', { class: 'pm-container-title' });
      var epicLink = el('a', { href: '/group?id=' + c.id, text: c.name || ('Epic ' + c.id) });
      heading.appendChild(epicLink);
      var backTarget = c.project ? PLUGIN_BASE + '/board?project=' + c.project.id + '&view=board' : PLUGIN_BASE + '/board';
      var backLabel = c.project ? '← ' + c.project.name : '← All projects';
      var back = el('a', { href: backTarget, text: backLabel, class: 'pm-back' });
      title.appendChild(back);
    }
    title.insertBefore(heading, title.firstChild);

    var newEpic = null;
    if (c.type === 'project') {
      newEpic = el('button', { class: 'pm-btn', type: 'button', text: '+ New epic', 'data-testid': 'pm-new-epic' });
      newEpic.addEventListener('click', function () { openEpicModal(); });
    }
    if (newEpic) title.appendChild(newEpic);

    root.appendChild(title);

    var content = el('div', {
      class: 'pm-view', 'data-testid': 'pm-' + state.view,
      id: 'pm-panel-' + state.view, role: 'tabpanel',
      'aria-labelledby': 'pm-tab-' + state.view,
    });
    root.appendChild(content);

    var renderer = {
      board: renderBoard,
      backlog: renderBacklog,
      dashboard: renderDashboard,
      timeline: renderTimeline,
    }[state.view];
    renderer(content).catch(function (e) {
      content.textContent = '';
      content.appendChild(el('div', { class: 'pm-error', role: 'alert' }, e.message));
    });
  }

  // ---------------------------------------------------------------------
  // Task card markup shared by several views
  // ---------------------------------------------------------------------

  // Darken a hex colour for text-on-tint contrast: the pill background is a
  // 15% tint of the same colour, so the text must be much darker than the
  // colour itself to pass WCAG AA.
  function darken(hex) {
    var m = /^#?([0-9a-f]{6})$/i.exec(hex || '');
    if (!m) return '#1c1917';
    var n = parseInt(m[1], 16);
    var r = Math.round(((n >> 16) & 255) * 0.45);
    var g = Math.round(((n >> 8) & 255) * 0.45);
    var b = Math.round((n & 255) * 0.45);
    var pad = function (x) { return (x < 16 ? '0' : '') + x.toString(16); };
    return '#' + pad(r) + pad(g) + pad(b);
  }

  function pill(entry) {
    var textColor = darken(entry.color);
    return el('span', { class: 'pm-pill', style: '--pm-color:' + entry.color + '; color:' + textColor + ';', text: entry.label });
  }

  function tagChips(tags) {
    var chips = [];
    asArray(tags).forEach(function (t) {
      chips.push(el('span', { class: 'pm-tag', text: '#' + t.Name }));
    });
    return chips;
  }

  function epicName(ownerId) {
    if (state.container && state.container.type === 'epic') return '';
    for (var i = 0; i < state.epics.length; i++) {
      if (state.epics[i].id === ownerId) return state.epics[i].name;
    }
    return null; // owned directly by the project
  }

  function moveAnnouncement(task, statusEntryResolved, columnSize) {
    announce('Moved "' + task.Name + '" to ' + statusEntryResolved.label +
      (columnSize ? ', position ' + columnSize + ' of ' + columnSize : ''));
  }

  // ---------------------------------------------------------------------
  // Mutations
  // ---------------------------------------------------------------------

  async function mutate(path, payload) {
    return apiFetch(path, { method: 'POST', json: payload });
  }

  async function moveTask(task, status, beforeId, afterId, focusControl, message) {
    var payload = { id: task.ID, status: status };
    if (beforeId) payload.before_id = beforeId;
    if (afterId) payload.after_id = afterId;
    await mutate('/api/task/move', payload);
    state.pendingFocus = { id: task.ID, control: focusControl || 'move-status' };
    state.pendingAnnounce = message;
    await refreshBoard();
  }

  function refreshBoard() {
    var content = document.querySelector('.pm-view[data-testid="pm-board"]');
    if (content) {
      var renderer = renderBoard(content);
      return renderer;
    }
    return renderApp();
  }

  // ---------------------------------------------------------------------
  // Board view
  // ---------------------------------------------------------------------

  async function renderBoard(content) {
    content.textContent = '';
    content.appendChild(el('div', { class: 'pm-live-msg' }));

    var board = el('div', {
      class: 'pm-board',
      role: 'region',
      'aria-label': 'Kanban board',
      tabindex: '0',
      'data-testid': 'pm-board',
    });
    content.appendChild(board);

    // Fetch every column in parallel, one page each; a column that fills its
    // page gains a Load more control that pages through the rest itself.
    var results = state.cfg.statuses.map(async function (s) {
      var tasks = await fetchTasks(columnFetchParams(s, 1));
      return { status: s, tasks: tasks };
    });
    var settled = await Promise.all(results);
    var byStatus = {};
    settled.forEach(function (r) {
      byStatus[r.status.name] = r.tasks;
    });
    rememberTasks(settled.reduce(function (acc, r) { return acc.concat(r.tasks); }, []));

    state.cfg.statuses.forEach(function (s) {
      board.appendChild(buildColumn(s, byStatus[s.name] || []));
    });

    wireDnD(board);
  }

  // buildColumn owns its own paging: `all` grows with each Load more click, so
  // a second click pages on from where the first stopped instead of refetching
  // the same page.
  function buildColumn(status, tasks) {
    var all = tasks.slice();
    var col = el('section', {
      class: 'pm-column',
      'data-status': status.name,
      'aria-label': status.label + ' column',
    });
    var header = el('div', { class: 'pm-column-header' });
    header.appendChild(el('h4', { class: 'pm-column-title', text: status.label }));
    header.appendChild(el('span', { class: 'pm-column-count', text: all.length + (all.length === 1 ? ' task' : ' tasks') }));
    var add = el('button', {
      class: 'pm-iconbtn', type: 'button', text: '+',
      'aria-label': 'Add task to ' + status.label, 'data-testid': 'pm-add-' + status.name,
    });
    add.addEventListener('click', function () { openTaskModal(status); });
    header.appendChild(add);
    col.appendChild(header);

    var exhausted = false;
    function refreshBody() {
      var old = col.querySelector('.pm-column-body');
      var fresh = buildColumnBody(status, all);
      if (old) old.replaceWith(fresh);
      else col.insertBefore(fresh, col.querySelector('.pm-column-footer'));
      var count = col.querySelector('.pm-column-count');
      if (count) count.textContent = all.length + (all.length === 1 ? ' task' : ' tasks');
      var footer = col.querySelector('.pm-column-footer');
      var pageFull = !exhausted && all.length > 0 && all.length % PAGE_SIZE === 0;
      if (pageFull) {
        if (!footer) {
          footer = el('div', { class: 'pm-column-footer' });
          col.appendChild(footer);
        } else {
          footer.textContent = '';   // drop the previous (possibly disabled) button
        }
        var more = el('button', { class: 'pm-btn pm-load-more', type: 'button', text: 'Load more…' });
        more.addEventListener('click', async function () {
          if (more.disabled) return;      // one fetch in flight
          more.disabled = true;
          try {
            var nextPage = Math.floor(all.length / PAGE_SIZE) + 1;
            var next = await fetchTasks(columnFetchParams(status, nextPage));
            all = all.concat(next);
            rememberTasks(next);
            if (next.length < PAGE_SIZE) exhausted = true;
            refreshBody();
          } catch (e) {
            more.disabled = false;
            announce(e.message);
          }
        });
        footer.appendChild(more);
      } else if (footer) {
        footer.remove();
      }
    }

    col.appendChild(buildColumnBody(status, all));
    refreshBody(); // installs the footer when the first page filled
    return col;
  }

  function buildColumnBody(status, tasks) {
    var body = el('div', {
      class: 'pm-column-body',
      'aria-label': status.label + ' tasks',
      tabindex: '0',
      'data-status': status.name,
    });
    tasks.forEach(function (t) {
      body.appendChild(buildCard(t, status));
    });
    if (!tasks.length) {
      body.appendChild(el('p', { class: 'pm-empty', text: 'No tasks' }));
    }
    return body;
  }

  function buildCard(task, columnStatus) {
    var card = el('article', {
      class: 'pm-card',
      'data-id': String(task.ID),
      draggable: 'true',
      'aria-label': task.Name + (columnStatus ? ', ' + statusEntry(columnStatus.name).label : ''),
    });

    var title = el('div', { class: 'pm-card-title' });
    var link = el('a', { class: 'pm-task-link', href: '/note?id=' + task.ID, text: task.Name });
    title.appendChild(link);
    card.appendChild(title);

    var meta = el('div', { class: 'pm-card-meta' });
    var m = task.Meta || {};
    var effectiveStatus = m.status || state.cfg.default_status;
    if (m.priority) meta.appendChild(pill(priorityEntry(m.priority)));
    if (task.EndDate) {
      var dueDays = dueInDays(task.EndDate);
      var dueLabel = 'Due ' + formatDate(task.EndDate);
      if (dueDays !== null && dueDays < 0 && effectiveStatus !== state.cfg.done_status) {
        dueLabel += ' (overdue)';
      }
      meta.appendChild(el('span', { class: 'pm-due', text: dueLabel }));
    }
    var epic = epicName(task.OwnerId);
    if (epic) meta.appendChild(el('span', { class: 'pm-epic-chip', text: epic }));
    tagChips(task.Tags).forEach(function (c) { meta.appendChild(c); });
    if (meta.childNodes.length) card.appendChild(meta);

    var actions = el('div', { class: 'pm-card-actions' });
    var up = el('button', {
      class: 'pm-iconbtn pm-move-up', type: 'button', text: '↑',
      'aria-label': 'Move "' + task.Name + '" up in ' + statusEntry((columnStatus || {}).name || '').label,
    });
    up.addEventListener('click', async function () {
      try {
        var siblings = columnCards(columnStatus);
        var idx = siblings.findIndex(function (s) { return s.ID === task.ID; });
        if (idx <= 0) return;
        var above = siblings[idx - 1];
        var abovePrev = idx >= 2 ? siblings[idx - 2] : null;
        var dest = columnStatus || statusEntry((task.Meta || {}).status || state.cfg.default_status);
        await moveTask(task, dest.name, abovePrev ? abovePrev.ID : null, above.ID, 'up',
          'Moved "' + task.Name + '" up in ' + dest.label + ', position ' + (idx) + ' of ' + siblings.length);
      } catch (e) { announce(e.message); }
    });
    var down = el('button', {
      class: 'pm-iconbtn pm-move-down', type: 'button', text: '↓',
      'aria-label': 'Move "' + task.Name + '" down in ' + statusEntry((columnStatus || {}).name || '').label,
    });
    down.addEventListener('click', async function () {
      try {
        var siblings2 = columnCards(columnStatus);
        var idx2 = siblings2.findIndex(function (s) { return s.ID === task.ID; });
        if (idx2 < 0 || idx2 >= siblings2.length - 1) return;
        var below = siblings2[idx2 + 1];
        var belowNext = idx2 + 2 < siblings2.length ? siblings2[idx2 + 2] : null;
        var dest2 = columnStatus || statusEntry((task.Meta || {}).status || state.cfg.default_status);
        await moveTask(task, dest2.name, below.ID, belowNext ? belowNext.ID : null, 'down',
          'Moved "' + task.Name + '" down in ' + dest2.label + ', position ' + (idx2 + 2) + ' of ' + siblings2.length);
      } catch (e) { announce(e.message); }
    });

    var select = el('select', {
      class: 'pm-status-move',
      'aria-label': 'Move "' + task.Name + '" to status',
      'data-testid': 'pm-move-' + task.ID,
    });
    var placeholder = el('option', { value: '', disabled: 'disabled', selected: 'selected' },
      'Move to…');
    select.appendChild(placeholder);
    state.cfg.statuses.forEach(function (s) {
      var opt = el('option', { value: s.name, text: s.label });
      if (s.name === ((columnStatus || {}).name)) opt.selected = true;
      select.appendChild(opt);
    });
    select.addEventListener('change', async function () {
      var target = select.value;
      if (!target) return;
      select.disabled = true;
      try {
        // Append at the destination column's true tail. The server owns the
        // tail computation (inside its column lock), so this works however
        // deep the column is — the client never has to page to find the end.
        await moveTask(task, target, null, null, 'move-status',
          'Moved "' + task.Name + '" to ' + statusEntry(target).label);
        select.selectedIndex = 0;
      } catch (e) {
        select.disabled = false;
        announce(e.message);
      }
    });
    actions.appendChild(up);
    actions.appendChild(down);
    actions.appendChild(select);
    card.appendChild(actions);
    return card;
  }

  function columnCards(status) {
    var content = document.querySelector('.pm-view[data-testid="pm-board"]');
    if (!content) return [];
    var col = content.querySelector('.pm-column[data-status="' + status.name + '"] .pm-column-body');
    if (!col) return [];
    var out = [];
    var cards = col.querySelectorAll('.pm-card');
    for (var i = 0; i < cards.length; i++) {
      var id = Number(cards[i].getAttribute('data-id'));
      var task = findCachedTask(id);
      if (task) out.push(task);
    }
    return out;
  }

  var _taskCache = [];
  function rememberTasks(tasks) {
    tasks.forEach(function (t) {
      var found = false;
      for (var i = 0; i < _taskCache.length; i++) {
        if (_taskCache[i].ID === t.ID) { _taskCache[i] = t; found = true; break; }
      }
      if (!found) _taskCache.push(t);
    });
  }
  function findCachedTask(id) {
    for (var i = 0; i < _taskCache.length; i++) {
      if (_taskCache[i].ID === id) return _taskCache[i];
    }
    return { ID: id, Name: 'Task ' + id, Meta: {} };
  }

  // ---- drag & drop (an enhancement on top of the keyboard controls) ----

  function wireDnD(board) {
    var draggedId = null;
    board.addEventListener('dragstart', function (e) {
      var card = e.target.closest('.pm-card');
      if (!card) return;
      draggedId = Number(card.getAttribute('data-id'));
      card.classList.add('pm-dragging');
      try { e.dataTransfer.setData('text/plain', String(draggedId)); } catch (err) { /* ie */ }
      if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
    });
    board.addEventListener('dragend', function (e) {
      var card = e.target.closest('.pm-card');
      if (card) card.classList.remove('pm-dragging');
      board.querySelectorAll('.pm-drop-target').forEach(function (c) {
        c.classList.remove('pm-drop-target');
      });
    });
    board.querySelectorAll('.pm-column-body').forEach(function (body) {
      body.addEventListener('dragover', function (e) {
        e.preventDefault();
        body.classList.add('pm-drop-target');
      });
      body.addEventListener('dragleave', function () {
        body.classList.remove('pm-drop-target');
      });
      body.addEventListener('drop', async function (e) {
        e.preventDefault();
        body.classList.remove('pm-drop-target');
        var id = draggedId || Number(e.dataTransfer.getData('text/plain'));
        draggedId = null;
        if (!id) return;
        var status = body.getAttribute('data-status');
        var cards = Array.prototype.slice.call(body.querySelectorAll('.pm-card'));
        var before = null;
        var after = null;
        var insertAt = cards.length; // default: end of column
        for (var i = 0; i < cards.length; i++) {
          var r = cards[i].getBoundingClientRect();
          if (e.clientY < r.top + r.height / 2) { insertAt = i; break; }
        }
        // neighbours as displayed
        var targetBefore = insertAt - 1 >= 0 ? cards[insertAt - 1] : null;
        var targetAfter = insertAt < cards.length ? cards[insertAt] : null;
        after = targetAfter ? Number(targetAfter.getAttribute('data-id')) : null;
        before = targetBefore ? Number(targetBefore.getAttribute('data-id')) : null;
        var task = findCachedTask(id);
        try {
          var pos = after || before ? ' to position ' + (insertAt + 1) : '';
          await moveTask(task, status, before, after, 'drag',
            'Moved "' + task.Name + '" to ' + statusEntry(status).label + pos);
        } catch (err) {
          announce(err.message);
        }
      });
    });
  }

  // ---------------------------------------------------------------------
  // Backlog view
  // ---------------------------------------------------------------------

  async function renderBacklog(content) {
    content.textContent = '';
    var filters = {
      statuses: {},      // name -> true
      priority: '',
      epic: '',
      tag: '',
    };
    state.cfg.statuses.forEach(function (s) { filters.statuses[s.name] = true; });

    var tasks = [];
    var listEl = el('ul', { class: 'pm-list' });

    function matches(t) {
      var m = t.Meta || {};
      var statusKey = effectiveStatus(t);
      if (!(statusKey in filters.statuses)) return false;
      if (filters.priority && m.priority !== filters.priority) return false;
      if (filters.epic) {
        var eid = Number(filters.epic);
        if (t.OwnerId !== eid) return false;
      }
      if (filters.tag) {
        var has = false;
        (t.Tags || []).forEach(function (tg) { if (tg.ID === Number(filters.tag)) has = true; });
        if (!has) return false;
      }
      return true;
    }

    function applyFilters() {
      listEl.textContent = '';
      var visible = tasks.filter(matches);
      if (!visible.length) {
        listEl.appendChild(el('li', { class: 'pm-empty' }, 'No tasks match the filters.'));
        return;
      }
      visible.sort(function (a, b) {
        var pa = prioIndex(a.Meta && a.Meta.priority);
        var pb = prioIndex(b.Meta && b.Meta.priority);
        if (pa !== pb) return pa - pb;
        return String(a.Name).localeCompare(String(b.Name));
      });
      visible.forEach(function (t) {
        listEl.appendChild(backlogRow(t));
      });
      var doneStatusName = state.cfg.done_status || 'done';
      var doneCount = visible.filter(function (t) { return effectiveStatus(t) === doneStatusName; }).length;
      announce(''); // silence counting
      var summary = content.querySelector('.pm-backlog-summary');
      if (summary) {
        summary.textContent = visible.length + ' task' + (visible.length === 1 ? '' : 's') +
          (doneCount ? ', ' + doneCount + ' done' : '');
      }
    }

    function prioIndex(name) {
      if (!name) return 99;
      for (var i = 0; i < state.cfg.priorities.length; i++) {
        if (state.cfg.priorities[i].name === name) return i;
      }
      return 98;
    }

    // Filter bar
    var bar = el('div', { class: 'pm-filters', 'data-testid': 'pm-backlog-filters' });

    var statusGroup = el('div', { class: 'pm-filter pm-filter-status' });
    statusGroup.appendChild(el('span', { class: 'pm-filter-label', text: 'Status' }));
    var statusChips = el('div', { class: 'pm-status-chips' });
    state.cfg.statuses.forEach(function (s) {
      var chip = el('button', {
        class: 'pm-chip', type: 'button', text: s.label,
        'aria-pressed': 'true', 'data-filter-status': s.name,
      });
      chip.addEventListener('click', function () {
        if (filters.statuses[s.name]) delete filters.statuses[s.name];
        else filters.statuses[s.name] = true;
        chip.setAttribute('aria-pressed', filters.statuses[s.name] ? 'true' : 'false');
        applyFilters();
      });
      statusChips.appendChild(chip);
    });
    statusGroup.appendChild(statusChips);
    bar.appendChild(statusGroup);

    var prioWrap = el('label', { class: 'pm-filter' });
    prioWrap.appendChild(el('span', { text: 'Priority' }));
    var prioSelect = el('select', { 'data-testid': 'pm-filter-priority' });
    prioSelect.appendChild(el('option', { value: '', text: 'All priorities' }));
    state.cfg.priorities.forEach(function (p) {
      prioSelect.appendChild(el('option', { value: p.name, text: p.label }));
    });
    prioSelect.addEventListener('change', function () {
      filters.priority = prioSelect.value;
      applyFilters();
    });
    prioWrap.appendChild(prioSelect);
    bar.appendChild(prioWrap);

    var epicWrap = el('label', { class: 'pm-filter' });
    epicWrap.appendChild(el('span', { text: 'Epic' }));
    var epicSelect = el('select', { 'data-testid': 'pm-filter-epic' });
    epicSelect.appendChild(el('option', { value: '', text: 'All epics' }));
    if (state.container.type === 'project') {
      state.epics.forEach(function (e) {
        epicSelect.appendChild(el('option', { value: String(e.id), text: e.name }));
      });
      epicSelect.appendChild(el('option', { value: String(state.container.id), text: '(No epic — project)' }));
    }
    epicSelect.addEventListener('change', function () {
      filters.epic = epicSelect.value;
      applyFilters();
    });
    epicWrap.appendChild(epicSelect);
    bar.appendChild(epicWrap);

    var tagWrap = el('label', { class: 'pm-filter' });
    tagWrap.appendChild(el('span', { text: 'Tag' }));
    var tagSelect = el('select', { 'data-testid': 'pm-filter-tag' });
    tagSelect.appendChild(el('option', { value: '', text: 'All tags' }));
    state.allTags.forEach(function (t) {
      tagSelect.appendChild(el('option', { value: String(t.ID), text: t.Name }));
    });
    tagSelect.addEventListener('change', function () {
      filters.tag = tagSelect.value;
      applyFilters();
    });
    tagWrap.appendChild(tagSelect);
    bar.appendChild(tagWrap);

    var addBtn = el('button', { class: 'pm-btn pm-btn-primary', type: 'button', text: '+ New task', 'data-testid': 'pm-add-task' });
    addBtn.addEventListener('click', function () {
      openTaskModal(statusEntry(state.cfg.default_status));
    });
    bar.appendChild(addBtn);

    content.appendChild(bar);
    var summary = el('p', { class: 'pm-backlog-summary' });
    content.appendChild(summary);
    content.appendChild(listEl);

    try {
      tasks = await fetchTaskPages({});
      rememberTasks(tasks);
      applyFilters();
    } catch (e) {
      content.appendChild(el('div', { class: 'pm-error', role: 'alert' }, e.message));
    }
  }

  function backlogRow(t) {
    var m = t.Meta || {};
    var cell = el('li', { 'data-testid': 'pm-backlog-row-' + t.ID });
    var left = el('div', { class: 'pm-backlog-title' });
    var link = el('a', { href: '/note?id=' + t.ID, text: t.Name, class: 'pm-task-link' });
    left.appendChild(link);
    cell.appendChild(left);
    cell.appendChild(pill(statusEntry(effectiveStatus(t))));
    if (m.priority) cell.appendChild(pill(priorityEntry(m.priority)));
    var epic = epicName(t.OwnerId);
    if (state.container.type === 'project') {
      cell.appendChild(el('span', { class: 'pm-tag', text: epic || '(no epic)' }));
    }
    tagChips(t.Tags).forEach(function (c) { cell.appendChild(c); });
    if (t.EndDate) cell.appendChild(el('span', { class: 'pm-due', text: formatDate(t.EndDate) }));
    return cell;
  }

  // ---------------------------------------------------------------------
  // Dashboard view
  // ---------------------------------------------------------------------

  async function renderDashboard(content) {
    content.textContent = '';
    content.appendChild(el('div', { class: 'pm-live-msg' }));

    var stats = await apiFetch('/api/stats' + containerQuery() + weekBoundsQuery());

    // by-status totals
    var dl = el('dl', { class: 'pm-summary' });
    var total = stats.total || 0;
    var doneStatus = state.cfg.done_status || 'done';
    var done = (stats.by_status && stats.by_status[doneStatus]) || 0;
    var cards = [
      { label: 'Total tasks', value: total },
      { label: 'Done', value: done + (total ? ' (' + Math.round((done / total) * 100) + '%)' : '') },
      { label: 'Overdue', value: stats.overdue || 0 },
      { label: 'Due this week', value: stats.due_this_week == null ? '—' : stats.due_this_week },
    ];
    cards.forEach(function (c) {
      var statKey = c.label.toLowerCase().replace(/\s+/g, '-');
      var box = el('div', { class: 'pm-summary-card' });
      box.appendChild(el('dt', { text: c.label }));
      box.appendChild(el('dd', { 'data-testid': 'pm-stat-' + statKey, 'data-stat': statKey, text: String(c.value) }));
      dl.appendChild(box);
    });
    content.appendChild(dl);

    // status bars
    var statusList = el('ul', { class: 'pm-bars', 'aria-label': 'Tasks by status' });
    state.cfg.statuses.forEach(function (s) {
      var n = (stats.by_status && stats.by_status[s.name]) || 0;
      var pct = total ? Math.round((n / total) * 100) : 0;
      var li = el('li');
      li.appendChild(el('span', { text: s.label }));
      var bar = el('div', { class: 'pm-bar', style: '--pm-color:' + s.color + ';' });
      bar.appendChild(el('span', { style: 'width:' + pct + '%' }));
      li.appendChild(bar);
      li.appendChild(el('span', { class: 'pm-bar-count', text: String(n) }));
      statusList.appendChild(li);
    });
    content.appendChild(statusList);

    // Epics strip (project container only)
    if (state.container.type === 'project' && state.epics.length) {
      var epicsEl = el('ul', { class: 'pm-epics', 'aria-label': 'Epics', 'data-testid': 'pm-epics' });
      var tasks = await fetchTaskPages({});
      state.epics.forEach(function (e) {
        var inEpic = tasks.filter(function (t) { return t.OwnerId === e.id; });
        var eDone = inEpic.filter(function (t) { return effectiveStatus(t) === doneStatus; }).length;
        var li = el('li');
        var a = el('a', { href: PLUGIN_BASE + '/board?epic=' + e.id + '&view=backlog', text: e.name });
        li.appendChild(a);
        var sub = el('div', { class: 'pm-epic-progress', 'data-testid': 'pm-epic-' + e.id });
        sub.textContent = inEpic.length ? eDone + ' / ' + inEpic.length + ' done' : 'no tasks';
        li.appendChild(sub);
        epicsEl.appendChild(li);
      });
      content.appendChild(el('h4', { text: 'Epics' }));
      content.appendChild(epicsEl);
    }

    content.setAttribute('data-testid', 'pm-dashboard');
  }

  function containerQuery() {
    if (state.container.type === 'epic') return '?epic=' + state.container.id;
    return '?project=' + state.container.id;
  }

  // Monday-midnight (local) naive bounds for the current week, in the wall-clock
  // format the host stores dates in. The stats endpoint has no date arithmetic,
  // so the client supplies the bounds it displays against.
  function weekBoundsQuery() {
    var now = new Date();
    var dow = now.getDay() || 7; // Mon=1 … Sun=7
    var monday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - (dow - 1));
    var fmt = function (d, h, mi) {
      return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate()) +
        'T' + pad2(h) + ':' + pad2(mi);
    };
    var start = fmt(monday, 0, 0);
    var endDate = new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + 7);
    var end = fmt(endDate, 0, 0);
    return '&now=' + encodeURIComponent(localNaiveNow()) +
      '&week_start=' + encodeURIComponent(start) +
      '&week_end=' + encodeURIComponent(end);
  }

  // ---------------------------------------------------------------------
  // Timeline view
  // ---------------------------------------------------------------------

  function timelineBuckets() {
    return [
      { key: 'overdue', label: 'Overdue' },
      { key: 'today', label: 'Today' },
      { key: 'tomorrow', label: 'Tomorrow' },
      { key: 'week', label: 'Next 7 days' },
      { key: 'later', label: 'Later' },
      { key: 'none', label: 'No due date' },
      { key: 'completed', label: 'Completed' },
    ];
  }

  function bucketFor(task) {
    var meta = task.Meta || {};
    if ((meta.status || state.cfg.default_status) === state.cfg.done_status) return 'completed';
    var days = dueInDays(task.EndDate);
    if (days === null) return 'none';
    if (days < 0) return 'overdue';
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days <= 7) return 'week';
    return 'later';
  }

  async function renderTimeline(content) {
    content.textContent = '';
    content.appendChild(el('div', { class: 'pm-live-msg' }));
    var tl = el('div', {
      class: 'pm-timeline',
      role: 'region',
      'aria-label': 'Timeline by due date',
      tabindex: '0',
      'data-testid': 'pm-timeline',
    });
    content.appendChild(tl);

    var tasks = await fetchTaskPages({});
    rememberTasks(tasks);

    var buckets = timelineBuckets();
    var byBucket = {};
    buckets.forEach(function (b) { byBucket[b.key] = []; });
    tasks.forEach(function (t) {
      var key = bucketFor(t);
      if (!byBucket[key]) key = 'later';
      byBucket[key].push(t);
    });

    buckets.forEach(function (b) {
      var items = byBucket[b.key];
      var col = el('section', { class: 'pm-day-col', 'aria-label': b.label + ' tasks' });
      col.appendChild(el('h4', { class: 'pm-day-head' }));
      col.querySelector('h4').appendChild(el('span', { text: b.label + ' (' + items.length + ')' }));
      var body = el('div', { class: 'pm-day-body' });
      items.forEach(function (t) {
        var m = t.Meta || {};
        var row = el('div', { class: 'pm-timeline-task' });
        row.appendChild(el('a', { href: '/note?id=' + t.ID, text: t.Name, class: 'pm-task-link' }));
        row.appendChild(pill(statusEntry(m.status || state.cfg.default_status)));
        if (t.EndDate && b.key !== 'none') row.appendChild(el('span', { class: 'pm-due', text: formatDate(t.EndDate) }));
        body.appendChild(row);
      });
      if (!items.length) body.appendChild(el('p', { class: 'pm-empty', text: '—' }));
      col.appendChild(body);
      tl.appendChild(col);
    });
  }

  // ---------------------------------------------------------------------
  // Modals: epic + task create/edit
  // ---------------------------------------------------------------------

  var modalSequence = 0;

  function modal(contents, title) {
    var previousFocus = document.activeElement;
    var headingId = 'pm-modal-title-' + (++modalSequence);
    var backdrop = el('div', { class: 'pm-modal-backdrop' });
    var box = el('div', {
      class: 'pm-modal', role: 'dialog', 'aria-modal': 'true',
      'aria-labelledby': headingId,
    });
    box.appendChild(el('h3', { id: headingId, text: title }));
    var error = el('div', { class: 'pm-error pm-modal-error', role: 'alert', hidden: true });
    box.appendChild(error);
    contents.forEach(function (c) { if (c) box.appendChild(c); });
    backdrop.appendChild(box);

    var closed = false;
    function close() {
      if (closed) return;
      closed = true;
      document.removeEventListener('keydown', handleKeydown);
      backdrop.remove();
      if (previousFocus && previousFocus.isConnected && typeof previousFocus.focus === 'function') {
        previousFocus.focus();
      }
    }

    function handleKeydown(event) {
      if (event.key === 'Escape') {
        event.preventDefault();
        close();
        return;
      }
      if (event.key !== 'Tab') return;
      var focusable = Array.prototype.slice.call(box.querySelectorAll(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )).filter(function (node) { return !node.hidden; });
      if (!focusable.length) {
        event.preventDefault();
        box.focus();
        return;
      }
      var first = focusable[0];
      var last = focusable[focusable.length - 1];
      if (event.shiftKey && (document.activeElement === first || !box.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    backdrop.addEventListener('click', function (e) {
      if (e.target === backdrop) close();
    });
    document.addEventListener('keydown', handleKeydown);
    document.body.appendChild(backdrop);
    return {
      box: box,
      backdrop: backdrop,
      close: close,
      setError: function (message) {
        error.textContent = message || '';
        error.hidden = !message;
      },
    };
  }

  function field(labelText, input) {
    var wrap = el('div', { class: 'pm-field' });
    var label = el('label', { text: labelText });
    if (input.id) label.setAttribute('for', input.id);
    wrap.appendChild(label);
    wrap.appendChild(input);
    return wrap;
  }

  function openEpicModal() {
    announce('');
    var name = el('input', { type: 'text', id: 'pm-new-epic-name', required: true, 'data-testid': 'pm-new-epic-name' });
    var win = modal([
      field('Epic name', name),
      el('div', { class: 'pm-modal-actions' }, [
        el('button', { class: 'pm-btn', type: 'button', text: 'Cancel', onclick: function () { win.close(); } }),
        el('button', { class: 'pm-btn pm-btn-primary', type: 'button', text: 'Create epic', 'data-testid': 'pm-add-epic',
          onclick: async function (event) {
            var button = event.currentTarget;
            win.setError('');
            if (!name.value.trim()) {
              win.setError('Epic name is required.');
              announce('Epic name is required.');
              name.focus();
              return;
            }
            button.disabled = true;
            try {
              await mutate('/api/epic/create', { project_id: state.container.id, name: name.value.trim() });
              win.close();
              await enterContainer();
            } catch (e) {
              win.setError(e.message);
              announce(e.message);
            } finally {
              button.disabled = false;
            }
          } }),
      ]),
    ], 'New epic');
    name.focus();
  }

  function defaultOwnerId() {
    if (state.container.type === 'epic') return state.container.id;
    // Tasks created from the board are placed directly on the project
    // (un-epic'd) unless the modal picks an epic.
    return state.container.id;
  }

  function openTaskModal(status, existing) {
    existing = existing || null;
    var isNew = !existing;
    announce('');
    var name = el('input', { type: 'text', id: 'pm-task-name', required: true, value: existing ? existing.Name : '' });
    var prioSelect = el('select', { id: 'pm-task-priority' });
    prioSelect.appendChild(el('option', { value: '', text: 'No priority' }));
    state.cfg.priorities.forEach(function (p) {
      var opt = el('option', { value: p.name, text: p.label });
      if (existing && (existing.Meta || {}).priority === p.name) opt.selected = true;
      prioSelect.appendChild(opt);
    });
    var due = el('input', { type: 'datetime-local', id: 'pm-task-due' });
    if (existing && existing.EndDate) due.value = toLocalInput(existing.EndDate);

    var epicSelect = null;
    if (state.container.type === 'project') {
      epicSelect = el('select', { id: 'pm-task-epic' });
      epicSelect.appendChild(el('option', { value: String(state.container.id), text: '(No epic)' }));
      state.epics.forEach(function (e) {
        var opt = el('option', { value: String(e.id), text: e.name });
        if (existing && existing.OwnerId === e.id) opt.selected = true;
        epicSelect.appendChild(opt);
      });
    }

    var win = modal([
      field('Task name', name),
      field('Priority', prioSelect),
      epicSelect ? field('Epic', epicSelect) : null,
      field('Due date', due),
      el('div', { class: 'pm-modal-actions' }, [
        el('button', { class: 'pm-btn', type: 'button', text: 'Cancel', onclick: function () { win.close(); } }),
        el('button', { class: 'pm-btn pm-btn-primary', type: 'button', text: isNew ? 'Create task' : 'Save changes',
          'data-testid': isNew ? 'pm-create-task' : 'pm-save-task',
          onclick: async function (event) {
            var button = event.currentTarget;
            win.setError('');
            if (!name.value.trim()) {
              win.setError('Task name is required.');
              announce('Task name is required.');
              name.focus();
              return;
            }
            button.disabled = true;
            try {
              if (isNew) {
                await mutate('/api/task/create', {
                  owner_id: Number(epicSelect ? epicSelect.value : defaultOwnerId()),
                  name: name.value.trim(),
                  priority: prioSelect.value || undefined,
                  due: due.value || undefined,
                  status: status.name,
                });
              } else {
                await mutate('/api/task/update', {
                  id: existing.ID,
                  name: name.value.trim(),
                  priority: prioSelect.value || '',
                  owner_id: epicSelect ? Number(epicSelect.value) : undefined,
                  due: due.value || '',
                });
              }
              win.close();
              await refreshBoard();
            } catch (e) {
              announce(e.message);
              win.setError(e.message);
            } finally {
              button.disabled = false;
            }
          } }),
      ]),
    ], isNew ? 'New task in ' + status.label : 'Edit task');
    name.focus();
  }

  function toLocalInput(iso) {
    // The host stores note dates as the naive wall-clock value the writer sent
    // (parsed as UTC). Round-tripping through the local Date would shift the
    // clock; the input wants the same wall-clock string back.
    if (!iso) return '';
    var m = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})/.exec(iso);
    return m ? m[1] : '';
  }

  // ---------------------------------------------------------------------
  // Focus restoration after a board refresh
  // ---------------------------------------------------------------------

  function restoreFocus() {
    if (state.pendingAnnounce) {
      announce(state.pendingAnnounce);
      state.pendingAnnounce = null;
    }
    if (!state.pendingFocus) return;
    var pf = state.pendingFocus;
    state.pendingFocus = null;
    var control = null;
    if (pf.control === 'move-status') {
      control = document.querySelector('[data-testid="pm-move-' + pf.id + '"]');
    } else if (pf.control === 'up') {
      control = document.querySelector('.pm-card[data-id="' + pf.id + '"] .pm-move-up');
    } else if (pf.control === 'down') {
      control = document.querySelector('.pm-card[data-id="' + pf.id + '"] .pm-move-down');
    }
    if (control) control.focus();
  }

  // ---------------------------------------------------------------------
  // Startup
  // ---------------------------------------------------------------------

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { boot().catch(reportFatal); });
  } else {
    boot().catch(reportFatal);
  }

  function reportFatal(e) {
    var root = byId('pm-root');
    if (root) renderError(root, e && e.message ? e.message : String(e));
  }

  // refreshBoard must restore focus after rebuild.
  var _origRenderBoard = renderBoard;
  renderBoard = async function (content) {
    var out = await _origRenderBoard(content);
    restoreFocus();
    return out;
  };
})();

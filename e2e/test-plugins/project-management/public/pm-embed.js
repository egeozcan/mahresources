/* Native PM controls: server badges remain usable until the element upgrades. */
(function () {
  'use strict';
  if (customElements.get('pm-status-control')) return;
  const core = window.PMCore;
  const values = { status: task => task.status || core.meta(task).status || '', due: task => task.end_date || '', owner: task => task.owner_id || '' };
  function message(element, text) {
    let output = element.querySelector(':scope > [role="status"]');
    if (!output) { output = document.createElement('span'); output.setAttribute('role', 'status'); element.append(output); }
    output.textContent = text;
  }
  async function publish(task) {
    const cfg = await core.config();
    document.querySelectorAll('[data-pm-id="' + task.id + '"]').forEach(region => {
      if (region.applyTask) { region.applyTask(task); return; }
      if (region.dataset.pmRegion === 'badges') {
        const status = cfg.statuses.find(entry => entry.name === core.status(task, cfg));
        const priority = cfg.priorities.find(entry => entry.name === core.meta(task).priority);
        region.replaceChildren(...[status, priority].filter(Boolean).map(core.pill));
      }
      if (region.dataset.pmRegion === 'avatar') {
        const status = cfg.statuses.find(entry => entry.name === core.status(task, cfg));
        if (status) { const badge = core.pill(status); badge.setAttribute('aria-label', status.label + ' task'); region.replaceChildren(badge); }
      }
      if (region.dataset.pmRegion === 'date') {
        region.textContent = task.end_date ? 'Due ' + task.end_date.slice(0, 10) + (core.overdue(task, cfg) ? ' (overdue)' : '') : '';
      }
      if (region.dataset.pmRegion === 'context') {
        const link = document.createElement('a'); link.href = '/group?id=' + task.owner_id; link.textContent = 'Owning epic or project';
        region.replaceChildren(...(task.owner_id ? [link] : []));
      }
    });
    document.querySelectorAll('.pm-mini-board article[data-pm-id="' + task.id + '"]').forEach(card => {
      const source = card.closest('.pm-mini-column'), board = source.parentElement;
      const target = board.querySelector('.pm-mini-column[data-status="' + core.status(task,cfg) + '"]');
      if (!target || target === source) return;
      const focus = document.activeElement;
      for (const [column, delta] of [[source,-1],[target,1]]) {
        const count = column.querySelector('.pm-mini-count'); count.textContent = String(Math.max(0,Number(count.textContent)+delta));
      }
      if (target.moveBefore) target.moveBefore(card,null); else target.append(card);
      if (focus?.isConnected && document.activeElement !== focus) focus.focus();
    });
    document.dispatchEvent(new CustomEvent('pm:task-updated', {detail: task}));
  }
  document.addEventListener('meta-shortcode-updated', async event => {
    if (event.detail?.entityType !== 'note') return;
    const id = Number(event.detail.entityId);
    if (!document.querySelector('[data-pm-id="' + id + '"]')) return;
    try {
      const response = await fetch('/v1/note?id=' + id);
      if (!response.ok) return;
      const note = await response.json();
      await publish({id:note.ID,meta:note.Meta,owner_id:note.OwnerId,start_date:note.StartDate,end_date:note.EndDate});
    } catch (_) { /* The host reports the save; a later reload refreshes other regions. */ }
  });
  class TaskControl extends HTMLElement {
    connectedCallback() { if (!this.control && !this.loading) this.upgrade(); }
    disconnectedCallback() {
      queueMicrotask(() => {
        if (this.isConnected) return; // A board move retains the mounted selector.
        this.ownerLoad?.abort();
        this.selector?.destroy();
        if (this.selector) { this.selector = null; this.control = null; }
      });
    }
    async upgrade() {
      this.loading = true;
      try {
        const kind = this.dataset.pmKind;
        let options = [];
        if (kind === 'status') options = JSON.parse(this.dataset.options || '[]');
        if (kind === 'owner') {
          const cfg = await core.config();
          if (!window.mahSelectors) await new Promise(resolve => document.addEventListener('alpine:initialized', resolve, {once:true}));
          if (!this.isConnected) return;
          this.ownerLoad = new AbortController();
          const value = this.dataset.value;
          this.selector = await window.mahSelectors.mountSingle(this, {
            entity: 'group', title: 'Task epic or project',
            selected: value ? [{ID:Number(value),Name:this.textContent.trim() || '#' + value}] : [],
            parameters: () => ({Categories: [cfg.project_category_id, cfg.epic_category_id]}),
            signal: this.ownerLoad.signal,
            onChange: () => this.save(),
          });
          this.control = this.querySelector('[role="combobox"]');
          this.setValue(this.dataset.value || '');
          return;
        }
        if (!this.isConnected) return;
        const label = document.createElement('label');
        label.append({status:'Status ',due:'Due ',owner:'Epic or project '}[kind]);
        const control = document.createElement(kind === 'due' ? 'input' : 'select');
        control.setAttribute('aria-label', {status:'Task status',due:'Task due date',owner:'Task epic or project'}[kind]);
        if (kind === 'due') control.type = 'datetime-local';
        else {
          options.forEach(option => { const el = document.createElement('option'); el.value = option.name; el.textContent = option.label; control.append(el); });
        }
        this.control = control;
        this.setValue(this.dataset.value || '');
        control.addEventListener('change', () => this.save());
        label.append(control); this.replaceChildren(label);
      } catch (error) { if (error.name !== 'AbortError') message(this, error.message); }
      finally { this.loading = false; }
    }
    setValue(value) {
      this.dataset.value = String(value);
      if (this.selector) {
        if (String(this.ownerSelection?.[0]?.ID) === String(value)) this.selector.replaceRawValues(this.ownerSelection);
        else this.selector.replaceByKeys(value ? [Number(value)] : []);
        this.ownerSelection = this.selector.getRawValues();
        return;
      }
      if (this.control) {
        const rendered = this.dataset.pmKind === 'due' ? core.localInput(value) : String(value);
        if (this.control.tagName === 'SELECT' && rendered && !Array.from(this.control.options).some(option => option.value === rendered)) {
          const option = document.createElement('option'); option.value = rendered; option.textContent = 'Current: ' + rendered; this.control.append(option);
        }
        this.control.value = rendered;
      }
    }
    applyTask(task) { this.setValue(values[this.dataset.pmKind](task)); }
    refreshFromMorph() { if (!this.busy && this.control) this.setValue(this.dataset.value || ''); }
    async save() {
      if (this.busy) return;
      const previous = this.dataset.value || '';
      const kind = this.dataset.pmKind;
      const body = {id: Number(this.dataset.pmId)};
      body[{status:'status',due:'due',owner:'owner_id'}[kind]] = kind === 'owner' ? Number(this.selector.getRawValues()[0]?.ID) : this.control.value;
      if (kind === 'owner' && !body.owner_id) { this.setValue(previous); return; }
      this.busy = true; this.control.disabled = true;
      this.selector?.setDisabled(true);
      try {
        await publish(await core.api('/api/task/update', body));
        message(this, 'Saved. Other note details update on reload.');
      } catch (error) { this.setValue(previous); message(this, error.message); }
      finally { if (this.control) this.control.disabled = false; this.selector?.setDisabled(false); this.busy = false; }
    }
  }
  ['status','due','owner'].forEach(kind => customElements.define('pm-' + kind + '-control', class extends TaskControl {}));

  // Serialize the whole read-modify-save operation per block. The host bridge
  // publishes a replacement only after its response; reading before the previous
  // save completes would lose another field edit, row addition or checked item.
  const blockWrites = new Map();
  function mutateBlock(id, operation) {
    const run = () => {
      const block = window.mahBlock?.getBlock(id);
      if (!block) throw new Error('This block is no longer available. Reload the note.');
      return operation(block);
    };
    const previous = blockWrites.get(id);
    const pending = previous ? previous.catch(() => {}).then(run) : Promise.resolve().then(run);
    blockWrites.set(id,pending);
    return pending.finally(() => { if (blockWrites.get(id) === pending) blockWrites.delete(id); });
  }

  // Block fields use the host's bridge; Lua emits data and markup, never JS.
  document.addEventListener('change', async event => {
    const field = event.target.closest('[data-pm-field]');
    if (!field || field.closest('[data-pm-block-busy]')) return;
    const id = Number(field.dataset.pmBlock), path = field.dataset.pmField.split('.');
    // Capture the event's value: a prior save can replace this editor's DOM.
    const value = field.type === 'number' ? Number(field.value) : field.value;
    try {
      await mutateBlock(id, async block => {
        const content = JSON.parse(JSON.stringify(block.content || {}));
        let target = content;
        for (const part of path.slice(0,-1)) target = target[part];
        target[path.at(-1)] = value;
        await window.mahBlock.saveContent(id, content);
      });
    } catch (error) { message(field.closest('section'), error.message); }
  });
  document.addEventListener('click', async event => {
    const button = event.target.closest('[data-pm-block-action]');
    if (!button || button.disabled || button.closest('[data-pm-block-busy]')) return;
    const id = Number(button.dataset.pmBlock), action = button.dataset.pmBlockAction;
    const key = button.dataset.pmCollection || 'items', index = Number(button.dataset.pmIndex);
    const reference = Number(button.closest('section').querySelector('[data-pm-new-reference="' + key + '"]')?.value);
    const noteID = Number(button.dataset.pmNote);
    // Row positions change on these writes. Freeze the persistent host wrapper
    // (including replacement render HTML) until the old positions are obsolete.
    const structural = ['add','remove','up','down'].includes(action);
    const root = button.closest('[data-block-id]') || button.closest('section');
    if (structural) { root.inert = true; root.setAttribute('data-pm-block-busy',''); root.setAttribute('aria-busy','true'); }
    button.disabled = true;
    try {
      await mutateBlock(id, async block => {
        const content = JSON.parse(JSON.stringify(block.content || {}));
        if (action === 'toggle') {
          const checked = new Set(Array.isArray(block.state?.checked) ? block.state.checked : []), item = content.items[index];
          if (checked.has(item.id)) checked.delete(item.id); else checked.add(item.id);
          await window.mahBlock.updateState(id, {...block.state, checked: [...checked]});
          return;
        }
        if (action === 'promote') {
          const item = content.items[index];
          const task = await core.api('/api/task/promote', {id:noteID, block_id:id, item_id:item.id});
          item.task_id = task.id;
        } else if (action === 'add') {
          if (!Array.isArray(content[key])) content[key] = [];
          if (!['items','entries'].includes(key) && (!Number.isInteger(reference) || reference < 1)) throw new Error('Enter a note ID first.');
          content[key].push(key === 'items' ? {id:crypto.randomUUID(),label:'New subtask'} : key === 'entries' ? {date:new Date().toISOString().slice(0,10),hours:1,note:''} : reference);
        } else if (action === 'remove') content[key].splice(index,1);
        else if (action === 'up' && index > 0) [content[key][index-1],content[key][index]] = [content[key][index],content[key][index-1]];
        else if (action === 'down' && index < content[key].length-1) [content[key][index+1],content[key][index]] = [content[key][index],content[key][index+1]];
        await window.mahBlock.saveContent(id,content);
      });
    } catch (error) { message(button.closest('section'),error.message); }
    finally {
      button.disabled = false;
      if (structural) { root.inert = false; root.removeAttribute('data-pm-block-busy'); root.removeAttribute('aria-busy'); }
    }
  });
})();

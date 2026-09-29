// Host glue between the Ketchup editor and mahresources: loads the resource
// into <drawing-app embedded>, and answers its save requests through the
// resource API as the signed-in user. The page's markup and config come from
// plugin.lua's "edit" page.
import './ketchup.js';

const config = JSON.parse(document.getElementById('ketchup-config').textContent);
const host = document.querySelector('.ketchup-host');
const app = host.querySelector('drawing-app');
const title = host.querySelector('.ketchup-title');
const nameLabel = host.querySelector('.ketchup-name-label');
const nameInput = host.querySelector('.ketchup-name');
const statusEl = host.querySelector('.ketchup-status');
const dirty = host.querySelector('.ketchup-dirty');
const back = host.querySelector('.ketchup-back');
const saveButton = host.querySelector('.ketchup-save');
const saveCopyButton = host.querySelector('.ketchup-save-copy');

const EXTENSIONS = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp' };

// The document being edited: `resource` is null until a new drawing is first saved.
let resource = config.mode === 'edit' ? config.resource : null;
let saveType = config.saveType || 'image/png';
let saving = false;

function setStatus(message, { error = false, link = null } = {}) {
    statusEl.replaceChildren(message);
    statusEl.classList.toggle('ketchup-status--error', error);
    if (link) {
        statusEl.append(' ');
        const a = document.createElement('a');
        a.href = link.href;
        a.textContent = link.label;
        statusEl.append(a);
    }
}

function csrfToken() {
    return document.querySelector('meta[name="csrf-token"]')?.content || '';
}

async function postForm(url, form) {
    const resp = await fetch(url, {
        method: 'POST',
        body: form,
        credentials: 'same-origin',
        // Without Accept the upload endpoints answer with a redirect for a browser.
        headers: { Accept: 'application/json', 'X-CSRF-Token': csrfToken() },
    });
    let data = null;
    try { data = await resp.json(); } catch { /* not JSON */ }
    if (!resp.ok) {
        const err = new Error(data?.error || `The server answered ${resp.status}`);
        err.details = data?.details;
        throw err;
    }
    return data;
}

function fileName(name) {
    const base = (name || 'drawing').trim().replace(/[\\/:*?"<>|]+/g, '_') || 'drawing';
    return `${base}.${EXTENSIONS[saveType] || 'png'}`;
}

function defaultName() {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, '0');
    return `Drawing ${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}.${pad(now.getMinutes())}`;
}

/** Show the page as editing `r`, and make a reload open it again. */
function showEditing(r) {
    resource = r;
    title.textContent = `Editing ${r.name || `resource ${r.id}`}`;
    nameLabel.hidden = true;
    back.hidden = false;
    back.href = `/resource?id=${r.id}`;
    back.textContent = 'Back to resource';
    saveButton.textContent = 'Save as new version';
    saveCopyButton.hidden = false;
    document.title = `${title.textContent} - mahresources`;
    const url = new URL(window.location.href);
    url.search = `?id=${r.id}`;
    history.replaceState(null, '', url);
}

function showNew() {
    title.textContent = config.owner ? `New image in ${config.owner.name}` : 'New image';
    nameLabel.hidden = false;
    nameInput.value = defaultName();
    if (config.owner) {
        back.hidden = false;
        back.href = `/group?id=${config.owner.id}`;
        back.textContent = 'Back to group';
    }
    saveButton.textContent = 'Save to library';
}

async function exportImage() {
    return app.exportImage(saveType === 'image/png' ? { type: saveType } : { type: saveType, quality: 0.92 });
}

async function createResource(name, ownerId) {
    const blob = await exportImage();
    const form = new FormData();
    form.append('resource', blob, fileName(name));
    form.append('Name', name);
    if (ownerId) form.append('OwnerId', String(ownerId));
    const created = await postForm('/v1/resource', form);
    const r = Array.isArray(created) ? created[0] : created;
    return { id: r.ID, name: r.Name, ownerId: r.OwnerId || 0 };
}

async function save({ asCopy = false } = {}) {
    if (saving) return;
    saving = true;
    saveButton.disabled = true;
    saveCopyButton.disabled = true;
    setStatus('Saving…');
    try {
        if (resource && !asCopy) {
            const blob = await exportImage();
            const form = new FormData();
            form.append('file', blob, fileName(resource.name));
            form.append('comment', 'Edited in Ketchup');
            const version = await postForm(`/v1/resource/versions?resourceId=${resource.id}`, form);
            app.markSaved();
            setStatus(`Saved as version ${version?.versionNumber ?? ''}.`.replace(' .', '.'));
        } else if (resource && asCopy) {
            const created = await createResource(`${resource.name || 'Drawing'} (edited)`, resource.ownerId);
            app.markSaved();
            showEditing(created);
            setStatus(`Saved as a new resource; you are now editing ${created.name}.`, {
                link: { href: `/resource?id=${created.id}`, label: 'Open it' },
            });
        } else {
            const name = nameInput.value.trim() || defaultName();
            const created = await createResource(name, config.owner?.id);
            app.markSaved();
            showEditing(created);
            setStatus('Saved to the library. Saving again adds a new version.', {
                link: { href: `/resource?id=${created.id}`, label: 'Open it' },
            });
        }
    } catch (err) {
        // A duplicate names the resource that already holds these exact bytes.
        const existing = err.details?.find?.((d) => d.resourceId)?.resourceId;
        setStatus(`Not saved: ${err.message}`, {
            error: true,
            link: existing ? { href: `/resource?id=${existing}`, label: 'Open the existing resource' } : null,
        });
    } finally {
        saving = false;
        saveButton.disabled = false;
        saveCopyButton.disabled = false;
    }
}

async function start() {
    if (config.mode === 'edit') showEditing(resource); else showNew();
    await app.whenReady();
    if (config.mode === 'edit') {
        setStatus('Loading image…');
        const resp = await fetch(config.fileUrl, { credentials: 'same-origin' });
        if (!resp.ok) throw new Error(`The image could not be loaded (${resp.status})`);
        await app.openImage(await resp.blob(), { name: resource.name });
        setStatus('');
    } else {
        await app.newDocument(config.width, config.height, { name: nameInput.value });
    }
    saveButton.disabled = false;
    app.focus();
}

// Fill the viewport below the page header, whatever the header's height.
function fitHost() {
    const top = host.getBoundingClientRect().top + window.scrollY;
    host.style.height = `${Math.max(448, window.innerHeight - top - 16)}px`;
}
fitHost();
window.addEventListener('resize', fitHost);

saveButton.addEventListener('click', () => save());
saveCopyButton.addEventListener('click', () => save({ asCopy: true }));
app.addEventListener('save-request', () => save());
app.addEventListener('modified-change', (e) => {
    dirty.hidden = !e.detail.modified;
});
window.addEventListener('beforeunload', (e) => {
    if (!app.modified) return;
    e.preventDefault();
    e.returnValue = '';
});

start().catch((err) => {
    setStatus(`Ketchup could not start: ${err.message}`, { error: true });
});

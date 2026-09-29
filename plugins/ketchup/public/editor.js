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
// Nothing is saved until the image (or blank page) is on the canvas: before
// that, the editor holds a placeholder that would overwrite the resource.
let loaded = false;

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
    // A proxy in front of the server can answer an expired session with a
    // redirect to its login page, which reads as a 200. That is not a save.
    if (resp.redirected || data === null) {
        throw new Error('The server did not confirm the save; sign in again and retry');
    }
    return data;
}

// A resource name often carries its file's extension; the saved file gets the
// extension of what was actually encoded instead.
function withoutExtension(name) {
    return (name || '').replace(/\.(png|jpe?g|webp|bmp)$/i, '');
}

function fileName(name, type) {
    const base = withoutExtension(name).trim().replace(/[\\/:*?"<>|]+/g, '_') || 'drawing';
    return `${base}.${EXTENSIONS[type] || 'png'}`;
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

// A browser that cannot encode the requested type (Safari and WebP) hands back
// PNG, so callers name the file after `blob.type`, not after what was asked for.
async function exportImage() {
    return app.exportImage(saveType === 'image/png' ? { type: saveType } : { type: saveType, quality: 0.92 });
}

// Resolves to the resource holding the saved bytes. `existing` is true when
// the library already had those exact bytes under another owner: the server
// then links the requested owner to that resource and answers with it rather
// than creating one, which is told apart by the owner it answers with.
async function createResource(name, ownerId) {
    const blob = await exportImage();
    const form = new FormData();
    form.append('resource', blob, fileName(name, blob.type));
    form.append('Name', name);
    if (ownerId) form.append('OwnerId', String(ownerId));
    const created = await postForm('/v1/resource', form);
    const r = Array.isArray(created) ? created[0] : created;
    const resource = { id: r.ID, name: r.Name, ownerId: r.OwnerId || 0 };
    return { resource, blob, existing: resource.ownerId !== (ownerId || 0) };
}

// The bytes are in the library, so the drawing is saved, but the resource is
// someone else's: say so, and keep editing what was being edited.
function reportExisting(r) {
    setStatus(`This exact image is already in the library as ${r.name || `resource ${r.id}`}, so it was linked to the group instead of saved twice.`, {
        link: { href: `/resource?id=${r.id}`, label: `Open ${r.name || `resource ${r.id}`}` },
    });
}

async function save({ asCopy = false } = {}) {
    if (saving || !loaded) return;
    saving = true;
    saveButton.disabled = true;
    saveCopyButton.disabled = true;
    setStatus('Saving…');
    try {
        if (resource && !asCopy) {
            const blob = await exportImage();
            const form = new FormData();
            form.append('file', blob, fileName(resource.name, blob.type));
            form.append('comment', 'Edited in Ketchup');
            const version = await postForm(`/v1/resource/versions?resourceId=${resource.id}`, form);
            app.markSaved(blob);
            setStatus(`Saved as version ${version?.versionNumber ?? ''}.`.replace(' .', '.'));
        } else if (resource && asCopy) {
            const { resource: created, blob, existing } = await createResource(`${withoutExtension(resource.name) || 'Drawing'} (edited)`, resource.ownerId);
            if (existing) {
                // The edit lives in that other resource, not in the one open here,
                // so this one keeps reading as unsaved.
                reportExisting(created);
                return;
            }
            app.markSaved(blob);
            showEditing(created);
            setStatus(`Saved as a new resource; you are now editing ${created.name}.`, {
                link: { href: `/resource?id=${created.id}`, label: `Open ${created.name}` },
            });
        } else {
            const name = nameInput.value.trim() || defaultName();
            const { resource: created, blob, existing } = await createResource(name, config.owner?.id);
            app.markSaved(blob);
            if (existing) {
                reportExisting(created);
                return;
            }
            showEditing(created);
            setStatus('Saved to the library. Saving again adds a new version.', {
                link: { href: `/resource?id=${created.id}`, label: `Open ${created.name}` },
            });
        }
    } catch (err) {
        // A duplicate names the resource that already holds these exact bytes.
        const existing = err.details?.find?.((d) => d.existingResourceId)?.existingResourceId;
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

// Whether the signed-in account may save at all. A guest, or an account an
// operator made read-only, may still open the editor, so it is told up front
// rather than after drawing.
async function canWrite() {
    try {
        const resp = await fetch('/v1/auth/me', { credentials: 'same-origin', headers: { Accept: 'application/json' } });
        if (!resp.ok) return true;
        return (await resp.json()).canWrite !== false;
    } catch {
        // Unknown: let the save itself answer.
        return true;
    }
}

async function start() {
    if (config.mode === 'edit') showEditing(resource); else showNew();
    const [writable] = await Promise.all([canWrite(), app.whenReady()]);
    if (config.mode === 'edit') {
        setStatus('Loading image…');
        const resp = await fetch(config.fileUrl, { credentials: 'same-origin' });
        if (!resp.ok) throw new Error(`The image could not be loaded (${resp.status})`);
        await app.openImage(await resp.blob(), { name: resource.name });
        setStatus('');
    } else {
        await app.newDocument(config.width, config.height, { name: nameInput.value });
    }
    app.focus();
    if (!writable) {
        setStatus('Your account can view images but not save them.');
        return;
    }
    loaded = true;
    saveButton.disabled = false;
    saveCopyButton.disabled = false;
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

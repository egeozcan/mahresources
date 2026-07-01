import { setCheckBox } from '../index.js';
import * as userSettings from '../userSettings.js';

function safeParse(raw) {
    try {
        return JSON.parse(raw || '{}');
    } catch {
        return {};
    }
}

// Registered local-setting inputs, kept module-level so DOM nodes are never placed in the
// reactive Alpine store. They are re-painted once the server-backed settings hydrate.
const registeredLocalEls = [];

// Persistence is gated until the server copy hydrates. A local edit made before the
// initial GET resolves would otherwise flush a partial uiSettings object (only the changed
// field) and drop the user's other server-saved fields. Instead, pre-load edits are held
// and merged with the server copy on load, then persisted once as a full object.
let hydrated = false;
let pendingLocalEdit = false;

function applyToEl(el, defVal, settings) {
    const value = settings[el.name] ?? defVal;
    if (typeof el.checked !== "undefined") {
        setCheckBox(el, value?.toString() === "true");
    } else {
        el.value = value;
    }
}

export function registerSavedSettingStore(Alpine) {
    Alpine.store('savedSetting', {
        // sessionSettings remain ephemeral/per-session (sessionStorage), unchanged.
        sessionSettings: safeParse(sessionStorage.getItem("settings")),
        // localSettings are now server-backed (user-setting key "uiSettings"). They start
        // from defaults and are populated once the settings load resolves.
        localSettings: {},
        /** @param {HTMLInputElement} el
         @param {boolean} isLocal
         @param {boolean} defVal */
        registerEl(el, isLocal = true, defVal = true) {
            if (isLocal) {
                registeredLocalEls.push({ el, defVal });
                applyToEl(el, defVal, this.localSettings);
            } else {
                applyToEl(el, defVal, this.sessionSettings);
            }

            el.addEventListener("change", () => {
                const value = el.checked ?? el.value;
                if (isLocal) {
                    this.localSettings[el.name] = value;
                    if (hydrated) {
                        // Persist the full field set so no other saved field is dropped.
                        userSettings.set('uiSettings', { ...this.localSettings });
                    } else {
                        // Held: merged with the server copy and persisted once on load.
                        pendingLocalEdit = true;
                    }
                } else {
                    this.sessionSettings[el.name] = value;
                    sessionStorage.setItem("settings", JSON.stringify(this.sessionSettings));
                }
            });
        }
    });

    // Hydrate localSettings from the server once, re-paint inputs registered before the
    // load resolved, and persist any pre-load edit as a full merged object.
    const store = Alpine.store('savedSetting');
    userSettings.whenLoaded().then(() => {
        const stored = userSettings.get('uiSettings');
        if (stored && typeof stored === 'object') {
            // Server fields first; local edits made during the load window override
            // field-by-field, so neither the user's in-flight change nor other
            // server-saved fields are lost.
            store.localSettings = { ...stored, ...store.localSettings };
        }
        hydrated = true;
        for (const { el, defVal } of registeredLocalEls) {
            applyToEl(el, defVal, store.localSettings);
        }
        if (pendingLocalEdit) {
            userSettings.set('uiSettings', { ...store.localSettings });
        }
    });
}

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
                    // Fire-and-forget: persists to the server (debounced). Before the
                    // initial load succeeds this only caches, never clobbering the server.
                    userSettings.set('uiSettings', { ...this.localSettings });
                } else {
                    this.sessionSettings[el.name] = value;
                    sessionStorage.setItem("settings", JSON.stringify(this.sessionSettings));
                }
            });
        }
    });

    // Hydrate localSettings from the server once, then re-paint any inputs that were
    // registered before the load resolved. Runs regardless of Alpine's store-init timing.
    const store = Alpine.store('savedSetting');
    userSettings.whenLoaded().then(() => {
        const stored = userSettings.get('uiSettings');
        if (stored && typeof stored === 'object') {
            store.localSettings = stored;
            for (const { el, defVal } of registeredLocalEls) {
                applyToEl(el, defVal, store.localSettings);
            }
        }
    });
}

import { setCheckBox } from '../index.js';

export function registerSavedSettingStore(Alpine) {
    Alpine.store('savedSetting', {
        sessionSettings: JSON.parse(sessionStorage.getItem("settings") || '{}'),
        localSettings: JSON.parse(localStorage.getItem("settings") || '{}'),
        /** @param {HTMLInputElement} el
         @param {boolean} isLocal
         @param {boolean} defVal */
        registerEl(el, isLocal = true, defVal = true) {
            const settings = isLocal ? this.localSettings : this.sessionSettings;
            const store = isLocal ? localStorage : sessionStorage;

            if (typeof el.checked !== "undefined") {
                setCheckBox(el, (settings[el.name] ?? defVal)?.toString() === "true");
            } else {
                el.value = settings[el.name] ?? defVal;
            }

            el.addEventListener("change", () => {
                const value = el.checked ?? el.value;
                store.setItem("settings", JSON.stringify({ ...settings, [el.name]: value }));
                settings[el.name] = value;
            });
        }
    });
}

document.addEventListener('alpine:init', () => {
    const listeners = new WeakMap();

    window.Alpine.store('savedSetting', {
        sessionSettings: JSON.parse(sessionStorage.getItem("settings") || '{}'),
        localSettings: JSON.parse(localStorage.getItem("settings") || '{}'),
        /** @param {HTMLInputElement} el
         * @param {boolean} isLocal */
        registerEl(el, isLocal = true) {
            const settings = isLocal ? this.localSettings : this.sessionSettings;
            const store = isLocal ? localStorage : sessionStorage;

            if (typeof el.checked !== "undefined") {
                setCheckBox(el, settings[el.name].toString() === "true");
            } else {
                el.value = settings[el.name];
            }

            const listener = () => {
                const value = el.checked ?? el.value;
                store.setItem("settings", JSON.stringify({ ...settings, [el.name]: value }));
                settings[el.name] = value;
            };

            if (listeners.has(el)) {
                el.removeEventListener("change", listeners.get(el))
            }

            el.addEventListener("change", listener);
        }
    });
});
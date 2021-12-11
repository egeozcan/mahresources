document.addEventListener('alpine:init', () => {
    window.Alpine.data('accordion', ({
        title = "",
        collapsed = false,
    } = {}) => {
        return {
            title,
            collapsed,
            loaded: false,
            load() {
                this.buttons = Array.from(this.$el.closest("section")?.querySelectorAll(".accordion-button"));
                this.index = this.buttons.indexOf(this.$el);
                this.loaded = true;
            },
            events: {
                ["@click.prevent"]() {
                    this.collapsed = !this.collapsed;
                },
                ["@keyup.prevent"]() {},
                ["@keydown.enter.prevent"]() {
                    this.collapsed = !this.collapsed;
                },
                ["@keydown.left.prevent"]() {
                    this.collapsed = true;
                },
                ["@keydown.right.prevent"]() {
                    this.collapsed = false;
                },
                ["@keydown.down.prevent"]() {
                    this.load();
                    this.buttons[(this.index + 1) % this.buttons.length].focus();
                },
                ["@keydown.up.prevent"]() {
                    this.load();
                    const startIndex = this.index === 0 ? this.buttons.length : this.index;
                    this.buttons[(startIndex - 1) % this.buttons.length].focus();
                },
                [':class']() { return 'accordion-button' },
            },

            async init() {
                window.Alpine.effect(() => {
                    [...this.$el.children]
                        .filter(x => x.tagName !== "BUTTON")
                        .forEach(x => x.style.display = this.collapsed ? 'none' : '');
                });
            }
        }
    })
})
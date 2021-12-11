document.addEventListener('alpine:init', () => {
    window.Alpine.data('confirmAction', ({
        message = "Are you sure you want to delete?"
    } = {}) => {
        return {
            message,
            events: {
                ["@submit"](e) {
                    if (e.shiftKey) {
                        return;
                    }

                    if (confirm(message)) {
                        return;
                    }

                    e.preventDefault();
                }
            }
        }
    })
})
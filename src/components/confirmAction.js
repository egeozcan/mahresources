export function confirmAction({
    message = "Are you sure you want to delete?"
} = {}) {
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
}

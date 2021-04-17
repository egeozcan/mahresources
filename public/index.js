function generatePreviewFromFile(e){
    const reader = new FileReader();

    return new Promise((resolve, reject) => {

        const file = e.target.files[0];

        if (!file) {
            resolve('');
            return;
        }

        reader.onload = function(event){
            const img = new Image();

            img.onload = function(){
                try {
                    const canvas = document.createElement('canvas');
                    const ctx = canvas.getContext("2d");

                    canvas.width = 165;
                    canvas.height = 130;

                    ctx.drawImage(img, 0, 0, 165, 130);
                    resolve(canvas.toDataURL().split(';')[1].replace(/^base64,/, ''));
                } catch (e) {
                    reject(e);
                }
            }

            img.src = event.target.result;
        };
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}

function abortableFetch(request, opts) {
    const controller = new AbortController();
    const signal = controller.signal;

    return {
        abort: () => controller.abort(),
        ready: fetch(request, { ...opts, signal })
    };
}

window.Spruce.store('bulkSelection', {
    selected: new Set(),
    select(num) {
        if (this.selected.has(num)) {
            this.selected.delete(num);
            return;
        }
        this.selected.add(num);
    },
});
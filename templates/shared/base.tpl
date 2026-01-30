<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ pageTitle }}</title>
    <link rel="stylesheet" href="/public/tailwind.css">
    <link rel="stylesheet" href="/public/index.css">
    <link rel="apple-touch-icon" sizes="180x180" href="/public/favicon/apple-icon-180x180.png">
    <link rel="icon" type="image/png" sizes="32x32" href="/public/favicon/favicon-32x32.png">
    <link rel="icon" type="image/png" sizes="16x16" href="/public/favicon/favicon-16x16.png">
    <meta name="theme-color" content="#ffffff">
    {% block head %}{% endblock %}
</head>
<body class="bg-gray-50 min-h-screen">
    <div class="max-w-4xl mx-auto py-8 px-4">
        {% block content %}{% endblock %}
    </div>

    {# Simple lightbox for shared galleries #}
    <div id="shared-lightbox" class="fixed inset-0 z-[9999] hidden" style="background: rgba(0,0,0,0.95);">
        <div class="absolute inset-0 flex items-center justify-center" onclick="if(event.target === this) window.sharedLightbox.close()">
            <button onclick="window.sharedLightbox.close()" class="absolute top-4 right-4 text-white text-4xl hover:text-gray-300 z-10" aria-label="Close">&times;</button>
            <button onclick="window.sharedLightbox.prev()" class="absolute left-4 top-1/2 -translate-y-1/2 text-white text-5xl hover:text-gray-300 p-4 z-10" aria-label="Previous">&lsaquo;</button>
            <button onclick="window.sharedLightbox.next()" class="absolute right-4 top-1/2 -translate-y-1/2 text-white text-5xl hover:text-gray-300 p-4 z-10" aria-label="Next">&rsaquo;</button>
            <img id="lightbox-img" class="max-h-[90vh] max-w-[90vw] object-contain" src="" alt="Gallery image">
        </div>
        <div id="lightbox-counter" class="absolute bottom-4 left-1/2 -translate-x-1/2 text-white text-sm z-10"></div>
    </div>

    <script type="module" src="/public/dist/main.js"></script>
    <script>
        window.sharedLightbox = {
            images: [],
            currentIndex: 0,

            open: function(images, index) {
                this.images = images;
                this.currentIndex = index;
                this.show();
            },

            show: function() {
                var lightbox = document.getElementById('shared-lightbox');
                var img = document.getElementById('lightbox-img');
                var counter = document.getElementById('lightbox-counter');
                img.src = this.images[this.currentIndex];
                counter.textContent = (this.currentIndex + 1) + ' / ' + this.images.length;
                lightbox.classList.remove('hidden');
                document.body.style.overflow = 'hidden';
            },

            close: function() {
                var lightbox = document.getElementById('shared-lightbox');
                lightbox.classList.add('hidden');
                document.body.style.overflow = '';
            },

            next: function() {
                if (this.currentIndex < this.images.length - 1) {
                    this.currentIndex++;
                    this.show();
                }
            },

            prev: function() {
                if (this.currentIndex > 0) {
                    this.currentIndex--;
                    this.show();
                }
            }
        };

        // Initialize galleries on page load
        document.addEventListener('DOMContentLoaded', function() {
            document.querySelectorAll('.shared-gallery').forEach(function(gallery) {
                var items = gallery.querySelectorAll('a.gallery-item');
                items.forEach(function(link, index) {
                    link.addEventListener('click', function(e) {
                        e.preventDefault();
                        var images = Array.from(items).map(function(a) { return a.href; });
                        window.sharedLightbox.open(images, index);
                    });
                });
            });
        });

        // Keyboard navigation
        document.addEventListener('keydown', function(e) {
            var lightbox = document.getElementById('shared-lightbox');
            if (lightbox.classList.contains('hidden')) return;
            if (e.key === 'Escape') window.sharedLightbox.close();
            if (e.key === 'ArrowRight') window.sharedLightbox.next();
            if (e.key === 'ArrowLeft') window.sharedLightbox.prev();
        });

        // Group reference tooltips
        document.addEventListener('DOMContentLoaded', function() {
            document.querySelectorAll('.group-reference-tooltip').forEach(function(el) {
                var tooltip = el.querySelector('.tooltip-content');
                if (!tooltip) return;

                // Only show tooltip if there's meaningful content (description or category)
                var hasDescription = el.dataset.groupDescription && el.dataset.groupDescription.trim();
                var hasCategory = el.dataset.groupCategory && el.dataset.groupCategory.trim();
                if (!hasDescription && !hasCategory) {
                    tooltip.remove();
                    el.style.cursor = 'default';
                    return;
                }

                function showTooltip() {
                    tooltip.classList.remove('hidden');
                }
                function hideTooltip() {
                    tooltip.classList.add('hidden');
                }

                el.addEventListener('mouseenter', showTooltip);
                el.addEventListener('mouseleave', hideTooltip);
                el.addEventListener('focus', showTooltip);
                el.addEventListener('blur', hideTooltip);
            });
        });
    </script>
</body>
</html>

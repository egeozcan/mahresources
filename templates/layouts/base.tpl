<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css">
    <link rel="stylesheet" href="/public/tailwind.css">
    <script src="/public/index.js" defer></script>
    <script src="https://cdn.jsdelivr.net/gh/alpinejs/alpine@v2.8.2/dist/alpine.min.js" defer></script>
    {% block head %}{% endblock %}
</head>
<body class="site">
    <header class="header">
        <nav class="menu" x-data="{ path: window.location.pathname }">
            {% for menuEntry in menu %}
            <a href="{{ menuEntry.Url }}" class="menu-item" :class="{ selected: path == '{{ menuEntry.Url }}' }">
                {{ menuEntry.Name }}
            </a>
            {% endfor %}
        </nav>
        {% block header %}{% endblock %}
    </header>
    <article class="content">
        {% include "../partials/title.tpl" %}

        {% if tags %}
        <div class="tags mb-10" style="margin-left: -0.5rem">
            {% for tag in tags.Tags %}
                {% include "../partials/tag.tpl" %}
            {% endfor %}
        </div>
        {% endif %}
        {% block search %}{% endblock %}
        {% block body %}{% endblock %}
    </article>
    <footer class="footer">
        {% include "../partials/pagination.tpl" %}
        {% block footer %}{% endblock %}
    </footer>
</body>
</html>
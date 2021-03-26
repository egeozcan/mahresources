<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>{{ title }}</title>
    <style>
        :root {
            --bg-accent: #d6f8f8;
            --spacing: 0.5rem;
        }

        html,
        body {
            padding: 0;
            margin: 0;
            font-family: -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Oxygen,
            Ubuntu, Cantarell, Fira Sans, Droid Sans, Helvetica Neue, sans-serif;
            font-size: 16px;
        }

        a {
            color: inherit;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        .site {
            padding: 1rem;
            display: grid;
            grid-template-rows: auto 1fr auto;
            height: 100vh;
            grid-gap: 1rem;
        }

        .header {
            grid-row: 1 / 2;
            background-color: var(--bg-accent);
        }

        .main {
            grid-row: 2 / 3;
            display: grid;
            grid-gap: var(--spacing);
            grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
        }

        .main > * > img {
            object-fit: cover;
            width: 100%;
            max-height: 100%;
        }

        .footer {
            grid-row: 3 / 4;
        }

        .menu {
            display: grid;
            gap: var(--spacing);
            grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
            place-items: start;
            place-content: start;
            grid-auto-flow: column;
        }

        .menuItem {
            display: inline-grid;
            place-content: center;
            place-items: center;
            height: 2rem;
            padding: var(--spacing);
        }
    </style>
    {% block head %}{% endblock %}
</head>
<body class="site">
    <header class="header">
        <nav class="menu">
            {% for menuEntry in menu %}
            <a href="{{ menuEntry.Url }}" class="menuItem">
                {{ menuEntry.Name }}
            </a>
            {% endfor %}
        </nav>
        {% block header %}{% endblock %}
    </header>
    <article class="main">{% block body %}{% endblock %}</article>
    <footer class="footer">{% block footer %}{% endblock %}</footer>
</body>
</html>
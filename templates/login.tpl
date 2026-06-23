<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{% if pageTitle %}{{ pageTitle }} - {% endif %}{{ title }}</title>
    <link rel="stylesheet" href="/public/index.css?v={{ assetVersion }}">
    <link rel="stylesheet" href="/public/tailwind.css?v={{ assetVersion }}">
    <link rel="icon" type="image/png" sizes="32x32" href="/public/favicon/favicon-32x32.png">
</head>
<body class="bg-stone-50 min-h-screen flex items-center justify-center p-4">
    <main class="w-full max-w-sm bg-white shadow ring-1 ring-black/5 rounded p-6" id="main-content">
        <h1 class="text-xl font-mono font-semibold mb-1">{{ title }}</h1>
        <p class="text-stone-500 text-sm mb-4">Sign in to continue</p>

        {% if loginError %}
        <div class="mb-4 px-3 py-2 rounded bg-red-50 text-red-700 text-sm" role="alert">{{ loginError }}</div>
        {% endif %}

        <form method="POST" action="/login" class="space-y-4">
            {% if next %}<input type="hidden" name="next" value="{{ next }}" />{% endif %}
            <div>
                <label for="username" class="block text-sm font-mono text-stone-700 mb-1">Username</label>
                <input id="username" name="username" type="text" autocomplete="username" autofocus required
                       class="w-full border border-stone-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-amber-500" />
            </div>
            <div>
                <label for="password" class="block text-sm font-mono text-stone-700 mb-1">Password</label>
                <input id="password" name="password" type="password" autocomplete="current-password" required
                       class="w-full border border-stone-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-amber-500" />
            </div>
            <button type="submit"
                    class="w-full bg-amber-600 hover:bg-amber-700 text-white font-mono py-2 rounded focus:outline-none focus:ring-2 focus:ring-amber-500">
                Sign in
            </button>
        </form>
    </main>
</body>
</html>

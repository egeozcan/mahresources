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
        <h1 class="text-xl font-mono font-semibold mb-1">Sign out</h1>
        <p class="text-stone-500 text-sm mb-4">Sign out of {{ title }} in this browser?</p>

        <form method="POST" action="/logout" class="space-y-3">
            <input type="hidden" name="csrf_token" value="{{ csrfToken }}">
            <button type="submit"
                    class="w-full bg-amber-700 hover:bg-amber-800 text-white font-mono py-2 rounded focus:outline-hidden focus:ring-2 focus:ring-amber-500">
                Sign out
            </button>
        </form>
        <p class="mt-4 text-sm"><a href="/dashboard" class="text-amber-700 hover:underline">Stay signed in</a></p>
    </main>
</body>
</html>

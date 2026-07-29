{% extends "/layouts/base.tpl" %}

{# The heading comes from partials/title.tpl (pageTitle); the message comes from #}
{# layouts/base.tpl's role="alert" region. What an error page needs beyond the #}
{# message is somewhere to go, which is what this block adds. It lives here #}
{# rather than in base.tpl because base.tpl's alert also serves 200-status pages #}
{# (/admin/users, /group/compare, /account), where "where to go next" would be #}
{# nonsense. Keep every comment on one line: pongo2 rejects a newline inside one. #}
{% block body %}
{% if errorRecovery %}
{# A plain div, not a <nav>: a second navigation landmark on every error page #}
{# would make `locator('nav')` ambiguous app-wide, and the header nav is already #}
{# the page's navigation. The links are in the accessibility tree either way. #}
<div class="error-recovery" data-testid="error-recovery">
    <p class="text-sm font-mono text-stone-600" id="error-recovery-label">Where to go next</p>
    <ul class="mt-2 flex flex-wrap gap-x-6 gap-y-1" aria-labelledby="error-recovery-label">
        {% for link in errorRecovery %}
        <li>
            <a href="{{ link.Url }}"
               class="text-sm text-amber-700 underline hover:text-amber-900 focus:outline-none focus:ring-2 focus:ring-amber-600 rounded">{{ link.Name }}</a>
        </li>
        {% endfor %}
    </ul>
</div>
{% endif %}
{% endblock %}

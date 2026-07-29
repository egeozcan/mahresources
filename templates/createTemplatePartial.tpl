{% extends "/layouts/base.tpl" %}

{% block body %}
{# The Name pattern is spelled with an alternation rather than the obvious #}
{# [a-z][a-z0-9-]* because the browser compiles `pattern` with the regex `v` #}
{# flag, under which an unescaped hyphen in a character class is a syntax #}
{# error — and an invalid pattern is ignored outright, so the field would #}
{# validate nothing. Pongo2 rejects the escaped form (\- is not a valid #}
{# escape in a template string), which is what rules that spelling out. #}
{% if queryValues.error.0 %}
<div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
  <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
</div>
{% endif %}
<form class="space-y-8" method="post" action="/v1/templatePartial{% if templatePartial.ID %}/edit{% endif %}">
    {% if templatePartial.ID %}
    <input type="hidden" value="{{ templatePartial.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="Name" value=queryValues.Name.0|default:templatePartial.Name required=true pattern="[a-z](?:[a-z0-9]|-)*" patternHint="Lowercase letters, digits and hyphens, starting with a letter." description="Kebab-case identifier used in the partial shortcode, e.g. status-badge. Lowercase letters, digits, and hyphens, starting with a letter." %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=queryValues.Description.0|default:templatePartial.Description description="Optional note describing what this partial renders and where to use it." %}

    {% include "/partials/form/createFormCodeEditorInput.tpl" with title="Content" name="Content" value=queryValues.Content.0|default:templatePartial.Content mode="html" description="HTML plus shortcodes. Expanded wherever a partial shortcode references this name, using that carrier entity's context." shortcodes=true %}

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}

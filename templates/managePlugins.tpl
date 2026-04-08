{% extends "/layouts/base.tpl" %}

{% block head %}
    <title>Manage Plugins - mahresources</title>
{% endblock %}

{% block body %}
<div class="content-wrap">
    {% if not plugins %}
    <p class="text-stone-500 italic">No plugins discovered. Place plugin directories in the plugins folder.</p>
    {% endif %}

    {% for plugin in plugins %}
    <div class="card mb-4" data-testid="plugin-card-{{ plugin.Name }}">
        <div class="card-header">
            <div class="flex items-start justify-between gap-4 w-full">
                <div class="min-w-0">
                    <div class="flex items-center gap-2">
                        <h2 class="text-lg font-semibold">{{ plugin.Name }}</h2>
                        <span class="card-badge">v{{ plugin.Version }}</span>
                        {% if plugin.Enabled %}
                        <span class="card-badge card-badge--relation">Enabled</span>
                        {% endif %}
                    </div>
                    {% if plugin.Description %}
                    <p class="text-sm text-stone-600 mt-1 whitespace-pre-line">{{ plugin.Description }}</p>
                    {% endif %}
                </div>
                <div class="flex gap-2 shrink-0">
                    <form method="POST"
                          action="{% if plugin.Enabled %}/v1/plugin/disable{% else %}/v1/plugin/enable{% endif %}">
                        <input type="hidden" name="name" value="{{ plugin.Name }}">
                        <button type="submit"
                                class="inline-flex justify-center py-2 px-4 border shadow-sm text-sm font-medium font-mono rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 {% if plugin.Enabled %}border-stone-300 text-stone-700 bg-white hover:bg-stone-50 focus:ring-amber-600{% else %}border-transparent text-white bg-amber-700 hover:bg-amber-800 focus:ring-amber-600{% endif %}"
                                data-testid="plugin-toggle-{{ plugin.Name }}">
                            {% if plugin.Enabled %}Disable{% else %}Enable{% endif %}
                        </button>
                    </form>
                    {% if not plugin.Enabled %}
                    <form method="POST" action="/v1/plugin/purge-data"
                          onsubmit="return confirm('Purge all stored data for {{ plugin.Name }}? This cannot be undone.')">
                        <input type="hidden" name="name" value="{{ plugin.Name }}">
                        <button type="submit"
                                class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-red-700 hover:bg-red-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-600"
                                data-testid="plugin-purge-{{ plugin.Name }}">
                            Purge Data
                        </button>
                    </form>
                    {% endif %}
                </div>
            </div>
        </div>

        {% if plugin.Settings %}
        <div class="card-body">
            <h3 class="text-sm font-semibold mb-2 text-stone-700 font-mono">Settings</h3>
            <form method="POST" action="/v1/plugin/settings"
                  x-data="pluginSettings('{{ plugin.Name }}')"
                  @submit.prevent="saveSettings"
                  data-testid="plugin-settings-{{ plugin.Name }}">

                {% for setting in plugin.Settings %}
                <div class="mb-3">
                    <label class="form-label font-mono" for="setting-{{ plugin.Name }}-{{ setting.Name }}">
                        {{ setting.Label }}
                        {% if setting.Required %}<span class="text-red-500" title="Required">*</span>{% endif %}
                    </label>

                    {% if setting.Type == "boolean" %}
                    <input type="checkbox"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           {% if plugin.Values %}{% with val=plugin.Values|lookup:setting.Name %}{% if val %}checked{% endif %}{% endwith %}{% elif setting.DefaultValue %}checked{% endif %}
                           class="form-checkbox"
                           data-testid="setting-{{ setting.Name }}">

                    {% elif setting.Type == "select" %}
                    <select id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                            name="{{ setting.Name }}"
                            class="form-input"
                            data-testid="setting-{{ setting.Name }}">
                        {% for option in setting.Options %}
                        <option value="{{ option }}"
                                {% with val=plugin.Values|lookup:setting.Name %}{% if val == option %}selected{% endif %}{% endwith %}>
                            {{ option }}
                        </option>
                        {% endfor %}
                    </select>

                    {% elif setting.Type == "password" %}
                    <input type="password"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% with val=plugin.Values|lookup:setting.Name %}{{ val }}{% endwith %}"
                           class="form-input"
                           {% if setting.Required %}required aria-required="true"{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">

                    {% elif setting.Type == "number" %}
                    <input type="number"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% if plugin.Values %}{% with val=plugin.Values|lookup:setting.Name %}{{ val }}{% endwith %}{% elif setting.DefaultValue %}{{ setting.DefaultValue }}{% endif %}"
                           class="form-input"
                           step="any"
                           {% if setting.Required %}required aria-required="true"{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">

                    {% else %}
                    <input type="text"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% if plugin.Values %}{% with val=plugin.Values|lookup:setting.Name %}{{ val }}{% endwith %}{% elif setting.DefaultValue %}{{ setting.DefaultValue }}{% endif %}"
                           class="form-input"
                           {% if setting.Required %}required aria-required="true"{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">
                    {% endif %}
                </div>
                {% endfor %}

                <button type="submit"
                        class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600"
                        data-testid="save-settings-{{ plugin.Name }}">
                    Save Settings
                </button>
                <span x-show="saved" x-transition role="status" class="text-amber-700 text-sm ml-2">Saved!</span>
                <span x-show="error" x-transition role="alert" class="text-red-700 text-sm ml-2" x-text="error"></span>
            </form>
        </div>
        {% else %}
        <div class="card-body">
            <p class="text-sm text-stone-500 italic">No settings declared.</p>
        </div>
        {% endif %}
    </div>
    {% endfor %}
</div>
{% endblock %}

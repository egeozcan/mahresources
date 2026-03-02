{% extends "/layouts/base.tpl" %}

{% block head %}
    <title>Manage Plugins - mahresources</title>
{% endblock %}

{% block body %}
<div class="content-wrap">
    <h1 class="page-title">Manage Plugins</h1>

    {% if not plugins %}
    <p class="text-gray-500 italic">No plugins discovered. Place plugin directories in the plugins folder.</p>
    {% endif %}

    {% for plugin in plugins %}
    <div class="card mb-4" data-testid="plugin-card-{{ plugin.Name }}">
        <div class="card-header flex items-center justify-between">
            <div>
                <h2 class="text-lg font-semibold">{{ plugin.Name }}
                    <span class="text-sm text-gray-500 font-normal">v{{ plugin.Version }}</span>
                </h2>
                {% if plugin.Description %}
                <p class="text-sm text-gray-600">{{ plugin.Description }}</p>
                {% endif %}
            </div>
            <form method="POST"
                  action="{% if plugin.Enabled %}/v1/plugin/disable{% else %}/v1/plugin/enable{% endif %}">
                <input type="hidden" name="name" value="{{ plugin.Name }}">
                <button type="submit"
                        class="btn {% if plugin.Enabled %}btn-danger{% else %}btn-primary{% endif %}"
                        data-testid="plugin-toggle-{{ plugin.Name }}">
                    {% if plugin.Enabled %}Disable{% else %}Enable{% endif %}
                </button>
            </form>
        </div>

        {% if plugin.Settings %}
        <div class="card-body">
            <h3 class="text-sm font-semibold mb-2 text-gray-700">Settings</h3>
            <form method="POST" action="/v1/plugin/settings"
                  x-data="pluginSettings('{{ plugin.Name }}')"
                  @submit.prevent="saveSettings"
                  data-testid="plugin-settings-{{ plugin.Name }}">

                {% for setting in plugin.Settings %}
                <div class="mb-3">
                    <label class="form-label" for="setting-{{ plugin.Name }}-{{ setting.Name }}">
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
                           {% if setting.Required %}required{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">

                    {% elif setting.Type == "number" %}
                    <input type="number"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% if plugin.Values %}{% with val=plugin.Values|lookup:setting.Name %}{{ val }}{% endwith %}{% elif setting.DefaultValue %}{{ setting.DefaultValue }}{% endif %}"
                           class="form-input"
                           step="any"
                           data-testid="setting-{{ setting.Name }}">

                    {% else %}
                    <input type="text"
                           id="setting-{{ plugin.Name }}-{{ setting.Name }}"
                           name="{{ setting.Name }}"
                           value="{% if plugin.Values %}{% with val=plugin.Values|lookup:setting.Name %}{{ val }}{% endwith %}{% elif setting.DefaultValue %}{{ setting.DefaultValue }}{% endif %}"
                           class="form-input"
                           {% if setting.Required %}required{% endif %}
                           placeholder="{{ setting.Label }}"
                           data-testid="setting-{{ setting.Name }}">
                    {% endif %}
                </div>
                {% endfor %}

                <button type="submit" class="btn btn-primary" data-testid="save-settings-{{ plugin.Name }}">
                    Save Settings
                </button>
                <span x-show="saved" x-transition class="text-green-600 text-sm ml-2">Saved!</span>
                <span x-show="error" x-transition class="text-red-600 text-sm ml-2" x-text="error"></span>
            </form>
        </div>
        {% else %}
        <div class="card-body">
            <p class="text-sm text-gray-500 italic">No settings declared.</p>
        </div>
        {% endif %}
    </div>
    {% endfor %}
</div>
{% endblock %}

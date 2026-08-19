{% extends "/layouts/base.tpl" %}

{% block head %}
    <title>Manage Plugins - mahresources</title>
{% endblock %}

{% block body %}
<div class="content-wrap">
    {% if docsLinksEnabled %}
    <p class="mb-4 text-sm text-stone-500">
        <a href="{{ docsURL("features/plugin-system") }}" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Plugin system docs</a>
    </p>
    {% endif %}

    {% if not plugins %}
    <p class="text-stone-500 italic">No plugins discovered. Place plugin directories in the plugins folder.</p>
    {% endif %}

    {% for plugin in plugins %}
    <div class="card mb-4" data-testid="plugin-card-{{ plugin.Name }}">
        <div class="card-header">
            <div class="flex items-start justify-between gap-4 w-full">
                <div class="min-w-0">
                    <div class="flex items-center flex-wrap gap-2">
                        <h2 class="text-lg font-semibold">{{ plugin.Name }}</h2>
                        <span class="card-badge">v{{ plugin.Version }}</span>
                        {% if plugin.Enabled %}
                        <span class="card-badge card-badge--relation">Enabled</span>
                        {% endif %}
                        {% if plugin.AllowScopedPrincipals %}
                        <span class="card-badge" data-testid="plugin-scoped-badge-{{ plugin.Name }}">Open to limited users</span>
                        {% endif %}
                        {% if plugin.Legacy %}
                        {# The badge says it in words, not only in red: no manifest means the full plugin surface. #}
                        <span class="card-badge card-badge--danger" data-testid="plugin-legacy-badge-{{ plugin.Name }}">No manifest &mdash; full access</span>
                        {% endif %}
                    </div>
                    {% if plugin.Description %}
                    <p class="text-sm text-stone-600 mt-1 whitespace-pre-line">{{ plugin.Description }}</p>
                    {% endif %}
                    {% if plugin.HasDocs %}
                    <a href="/plugins/{{ plugin.Name }}/docs" class="text-sm text-amber-700 hover:underline mt-1 inline-block" data-testid="plugin-docs-{{ plugin.Name }}">View documentation</a>
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
                    {% if plugin.Enabled %}
                    {# Who may reach the plugin, which is a different question from what it may do. #}
                    <form method="POST" action="/v1/plugin/scopedAccess">
                        <input type="hidden" name="name" value="{{ plugin.Name }}">
                        <input type="hidden" name="allowed" value="{% if plugin.AllowScopedPrincipals %}0{% else %}1{% endif %}">
                        <button type="submit"
                                class="inline-flex justify-center py-2 px-4 border shadow-sm text-sm font-medium font-mono rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 border-stone-300 text-stone-700 bg-white hover:bg-stone-50 focus:ring-amber-600"
                                data-testid="plugin-scoped-access-{{ plugin.Name }}"
                                title="Group-limited users and guests are refused this plugin's pages, endpoints and shortcodes unless this is on. It does not widen what the plugin may do on their behalf.">
                            {% if plugin.AllowScopedPrincipals %}Hide from limited users{% else %}Allow limited users{% endif %}
                        </button>
                    </form>
                    {% endif %}
                    {% if not plugin.Enabled %}
                    <form method="POST" action="/v1/plugin/purge-data"
                          x-data="confirmAction()" x-bind="events"
                          data-confirm-message="Purge all stored data for {{ plugin.Name }}? This cannot be undone.">
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

        {# Access: what the plugin is granted, always rendered, so enabling is an informed decision. #}
        <div class="card-body border-t border-stone-200 pt-3" data-testid="plugin-access-{{ plugin.Name }}">
            <h3 class="text-sm font-semibold mb-2 text-stone-700 font-mono">Access</h3>

            {% if plugin.Legacy %}
            <p class="mb-3 rounded-md bg-red-50 border border-red-200 p-3 text-sm text-red-800"
               data-testid="plugin-legacy-warning-{{ plugin.Name }}">
                <span aria-hidden="true">&#9888;</span>
                <strong class="font-semibold">Warning: this plugin declares no manifest.</strong>
                It says nothing about what it needs, so nothing is withheld from it: it receives
                the full plugin surface &mdash; every capability listed below &mdash; and it may
                make outbound requests to any public host.
            </p>
            {% endif %}

            <dl class="text-sm text-stone-700 space-y-3">
                <div>
                    <dt class="font-semibold text-stone-800">
                        {% if plugin.Enabled %}This plugin is granted:{% else %}Enabling grants:{% endif %}
                    </dt>
                    <dd class="mt-1">
                        {% if plugin.Capabilities %}
                        <ul class="list-disc pl-5 space-y-1" data-testid="plugin-capabilities-{{ plugin.Name }}">
                            {% for capability in plugin.Capabilities %}
                            <li>{{ plugin.CapabilityLabels|lookup:capability|default:capability }}</li>
                            {% endfor %}
                        </ul>
                        {% else %}
                        <p data-testid="plugin-capabilities-{{ plugin.Name }}">
                            Nothing. This plugin asks for no capabilities, so none of the plugin APIs are installed for it.
                        </p>
                        {% endif %}
                    </dd>
                </div>

                <div>
                    <dt class="font-semibold text-stone-800">Outbound network</dt>
                    <dd class="mt-1" data-testid="plugin-network-{{ plugin.Name }}">
                        {% if plugin.Network %}
                        <p>It may connect only to these hosts:</p>
                        <ul class="list-disc pl-5 space-y-1 font-mono text-xs mt-1">
                            {% for host in plugin.Network %}
                            <li>{{ host }}</li>
                            {% endfor %}
                        </ul>
                        {% else %}
                        <p>
                            No allowlist is declared, so it may reach
                            <strong class="font-semibold">any public host</strong>.
                            Private network addresses stay blocked.
                        </p>
                        {% endif %}
                    </dd>
                </div>

                {% if plugin.AllowPrivateHosts %}
                <div>
                    <dt class="font-semibold text-stone-800">Private network addresses</dt>
                    <dd class="mt-1">
                        <p class="rounded-md bg-red-50 border border-red-200 p-3 text-red-800"
                           data-testid="plugin-private-hosts-{{ plugin.Name }}">
                            <span aria-hidden="true">&#9888;</span>
                            <strong class="font-semibold">Warning:</strong>
                            it may reach private network addresses, including services running on this machine.
                            Only the IP addresses and CIDR blocks listed above lift that block; the host names do not.
                        </p>
                    </dd>
                </div>
                {% endif %}

                {% if plugin.Dependencies %}
                <div>
                    <dt class="font-semibold text-stone-800">Requires these plugins</dt>
                    <dd class="mt-1" data-testid="plugin-dependencies-{{ plugin.Name }}">
                        <ul class="list-disc pl-5 space-y-1 font-mono text-xs">
                            {% for dependency in plugin.Dependencies %}
                            <li>{{ dependency }}</li>
                            {% endfor %}
                        </ul>
                        <p class="text-xs text-stone-500 mt-1">Each one must be enabled first, or enabling this plugin is refused.</p>
                    </dd>
                </div>
                {% endif %}

                {% if not plugin.Legacy %}
                <div>
                    <dt class="font-semibold text-stone-800">Plugin API version</dt>
                    <dd class="mt-1" data-testid="plugin-api-version-{{ plugin.Name }}">{{ plugin.APIVersion }}</dd>
                </div>
                {% endif %}

                {% if plugin.MinAppVersion %}
                <div>
                    <dt class="font-semibold text-stone-800">Minimum app version</dt>
                    <dd class="mt-1" data-testid="plugin-min-app-version-{{ plugin.Name }}">
                        <span class="font-mono text-xs">{{ plugin.MinAppVersion }}</span>
                        <span class="text-stone-500">&mdash; informational only: it is declared and shown here, but never checked or enforced.</span>
                    </dd>
                </div>
                {% endif %}
            </dl>
        </div>

        {% if plugin.Settings %}
        <div class="card-body border-t border-stone-200 pt-3">
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
                {# Finding 4: the status region stays in the tree and only its text changes. #}
                {# A role="status" that is display:none until the moment it has something to say #}
                {# is the unreliable half of that finding — the app's own announcer #}
                {# (src/utils/ariaLiveRegion.js) inserts the region first and sets the text a task #}
                {# later for exactly this reason. x-show + x-transition did the opposite. #}
                <span role="status" class="text-amber-700 text-sm ml-2" x-text="saved ? 'Saved!' : ''"></span>
                <span x-show="error" x-transition role="alert" class="text-red-700 text-sm ml-2" x-text="error"></span>
            </form>
        </div>
        {% else %}
        <div class="card-body border-t border-stone-200 pt-3">
            <p class="text-sm text-stone-500 italic">No settings declared.</p>
        </div>
        {% endif %}

        {% if plugin.Schedules %}
        {# Rendered from the stored rows, not the live registry: the two differ #}
        {# exactly where an operator needs to look. A disabled plugin's schedule #}
        {# is still here and inert, and an unowned one has stopped for good. #}
        <div class="card-body border-t border-stone-200 pt-3" data-testid="plugin-schedules-{{ plugin.Name }}">
            <h3 class="text-sm font-semibold text-stone-700 mb-2">Schedules</h3>
            <div class="overflow-x-auto">
                <table class="min-w-full text-sm">
                    <thead>
                        <tr class="text-left text-xs uppercase tracking-wide text-stone-500">
                            <th class="py-1 pr-4 font-medium" scope="col">Id</th>
                            <th class="py-1 pr-4 font-medium" scope="col">Every</th>
                            <th class="py-1 pr-4 font-medium" scope="col">Next due</th>
                            <th class="py-1 pr-4 font-medium" scope="col">Runs</th>
                            <th class="py-1 pr-4 font-medium" scope="col">Last outcome</th>
                        </tr>
                    </thead>
                    <tbody>
                        {% for schedule in plugin.Schedules %}
                        <tr class="border-t border-stone-100" data-testid="plugin-schedule-{{ plugin.Name }}-{{ schedule.ScheduleID }}">
                            <td class="py-1 pr-4 font-mono">{{ schedule.ScheduleID }}</td>
                            <td class="py-1 pr-4 font-mono">{{ schedule.Every }}{% if schedule.Overlap != "skip" %} <span class="card-badge">{{ schedule.Overlap }}</span>{% endif %}</td>
                            <td class="py-1 pr-4">
                                {% if not schedule.Owned %}
                                <span class="card-badge card-badge--danger">Stopped &mdash; no owner</span>
                                {% elif not schedule.Registered %}
                                <span class="card-badge">Not declared</span>
                                {% else %}
                                {{ schedule.NextDueAt|date:"2006-01-02 15:04" }}
                                {% endif %}
                            </td>
                            <td class="py-1 pr-4 font-mono">{{ schedule.Runs }}</td>
                            <td class="py-1 pr-4">
                                {% if schedule.LastStatus %}
                                <span class="{% if schedule.LastStatus == "failed" %}text-red-700{% else %}text-stone-600{% endif %}">{{ schedule.LastStatus }}</span>
                                {% if schedule.LastError %}
                                <span class="text-stone-500">&mdash; {{ schedule.LastError }}</span>
                                {% endif %}
                                {% else %}
                                <span class="text-stone-500 italic">never run</span>
                                {% endif %}
                            </td>
                        </tr>
                        {% endfor %}
                    </tbody>
                </table>
            </div>
            <p class="text-xs text-stone-500 mt-2">
                Each run executes as the operator who enabled this plugin. A schedule whose owner
                has been deleted stops rather than falling back to an administrator.
            </p>
        </div>
        {% endif %}
    </div>
    {% endfor %}
</div>
{% endblock %}

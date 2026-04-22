{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-6" data-testid="admin-settings">
  <header class="space-y-1">
    <h1 class="text-lg font-semibold font-mono text-stone-800">Runtime Settings</h1>
    <p class="text-sm text-stone-500">Changes take effect immediately without a restart. Boot defaults are shown as reference and can be restored with Reset.</p>
  </header>

  {% for group in settingsByGroup %}
  <section class="rounded-lg bg-white border border-stone-200 p-5" aria-labelledby="grp-{{ group.Group }}">
    <h2 id="grp-{{ group.Group }}" class="text-base font-semibold font-mono text-stone-800 mb-4 capitalize">{{ group.Group }}</h2>
    <div class="space-y-4">
      {% for s in group.Items %}
      <div class="border border-stone-200 rounded-md p-4"
           data-setting-key="{{ s.Key }}"
           data-testid="setting-row-{{ s.Key }}"
           x-data="settingRow({{ s|json }})">
        <label :for="'setting-' + key" class="block text-sm font-medium text-stone-800">
          {{ s.Label }}
          <template x-if="overridden">
            <span class="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800">Override</span>
          </template>
        </label>
        <p class="text-xs text-stone-500 mt-0.5">{{ s.Description }}</p>
        <div class="mt-2 flex flex-wrap gap-2 items-start">
          <input :id="'setting-' + key"
                 type="text"
                 x-model="value"
                 class="border border-stone-300 rounded px-2 py-1 text-sm flex-1 min-w-[12rem] font-mono focus:outline-none focus:ring-2 focus:ring-amber-500"
                 :aria-describedby="'hint-' + key" />
          <input type="text"
                 placeholder="Reason (optional)"
                 x-model="reason"
                 class="border border-stone-300 rounded px-2 py-1 text-sm w-48 focus:outline-none focus:ring-2 focus:ring-amber-500"
                 :aria-label="'Reason for ' + label" />
          <button type="button"
                  @click="save()"
                  class="inline-flex items-center px-3 py-1 text-sm font-medium text-white bg-amber-700 rounded hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600">
            Save
          </button>
          <template x-if="overridden">
            <button type="button"
                    @click="reset()"
                    class="inline-flex items-center px-3 py-1 text-sm font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-stone-400">
              Reset
            </button>
          </template>
        </div>
        <p :id="'hint-' + key" class="text-xs text-stone-500 mt-1 font-mono">
          Boot default: <span x-text="bootDefaultDisplay"></span>
          <template x-if="minDisplay">
            <span class="ml-4">Min: <span x-text="minDisplay"></span></span>
          </template>
          <template x-if="maxDisplay">
            <span class="ml-4">Max: <span x-text="maxDisplay"></span></span>
          </template>
        </p>
        <div class="text-xs mt-1 min-h-[1.25rem]" role="status" aria-live="polite">
          <span x-show="flash" x-text="flash" class="text-green-700"></span>
          <span x-show="error" x-text="error" class="text-red-700"></span>
        </div>
      </div>
      {% endfor %}
    </div>
  </section>
  {% endfor %}

  {% if bootOnly %}
  <details class="rounded-lg bg-stone-50 border border-stone-200 p-5">
    <summary class="cursor-pointer text-sm font-medium font-mono text-stone-700 select-none">Boot-only settings (require restart to change)</summary>
    <table class="mt-3 text-sm w-full" aria-label="Boot-only settings">
      <thead>
        <tr>
          <th class="text-left p-2 text-xs font-medium text-stone-500 uppercase tracking-wider">Setting</th>
          <th class="text-left p-2 text-xs font-medium text-stone-500 uppercase tracking-wider">Value</th>
        </tr>
      </thead>
      <tbody>
        {% for f in bootOnly %}
        <tr class="border-t border-stone-200">
          <td class="p-2 text-stone-700">{{ f.Label }}</td>
          <td class="p-2 font-mono text-stone-900">{{ f.Value }}</td>
        </tr>
        {% endfor %}
      </tbody>
    </table>
  </details>
  {% endif %}
</div>

<script>
(function () {
  function nanosToShort(n) {
    if (typeof n !== 'number' || n === 0) return '0s';
    const ms = Math.floor(n / 1e6);
    if (ms < 1000) return ms + 'ms';
    const s = Math.floor(ms / 1000);
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    if (m < 60) return m + 'm' + (s % 60 ? (s % 60) + 's' : '');
    const h = Math.floor(m / 60);
    return h + 'h' + (m % 60 ? (m % 60) + 'm' : '');
  }

  function formatValue(type, val) {
    if (val === null || val === undefined) return '';
    if (type === 'duration') return nanosToShort(Number(val));
    return String(val);
  }

  window.settingRow = function settingRow(initial) {
    return {
      key: initial.key,
      label: initial.label,
      type: initial.type,
      value: formatValue(initial.type, initial.current),
      reason: '',
      overridden: initial.overridden,
      flash: '',
      error: '',

      get bootDefaultDisplay() {
        return formatValue(this.type, initial.bootDefault);
      },
      get minDisplay() {
        if (initial.minNumeric == null) return null;
        return formatValue(this.type, initial.minNumeric);
      },
      get maxDisplay() {
        if (initial.maxNumeric == null) return null;
        return formatValue(this.type, initial.maxNumeric);
      },

      async save() {
        this.error = '';
        this.flash = '';
        let res;
        try {
          res = await fetch('/v1/admin/settings/' + encodeURIComponent(this.key), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ value: this.value, reason: this.reason }),
          });
        } catch (e) {
          this.error = 'Network error: ' + e.message;
          return;
        }
        let body = null;
        try { body = await res.json(); } catch (_) { body = { error: 'HTTP ' + res.status }; }
        if (!res.ok) {
          this.error = (body && (body.error || body.message)) || 'HTTP ' + res.status;
          return;
        }
        this.overridden = body.overridden;
        this.value = formatValue(this.type, body.current);
        this.flash = 'Saved — took effect at ' + new Date().toLocaleTimeString();
      },

      async reset() {
        this.error = '';
        this.flash = '';
        let res;
        try {
          res = await fetch('/v1/admin/settings/' + encodeURIComponent(this.key), {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ reason: this.reason }),
          });
        } catch (e) {
          this.error = 'Network error: ' + e.message;
          return;
        }
        let body = null;
        try { body = await res.json(); } catch (_) { body = { error: 'HTTP ' + res.status }; }
        if (!res.ok) {
          this.error = (body && (body.error || body.message)) || 'HTTP ' + res.status;
          return;
        }
        this.overridden = false;
        this.value = formatValue(this.type, body.current);
        this.flash = 'Reset to boot default';
      },
    };
  };
}());
</script>
{% endblock %}

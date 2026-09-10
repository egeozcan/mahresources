export function reductionCreateForm() {
  return {
    busy: false,
    error: '',

    async submit(form) {
      if (this.busy) return;
      const data = new FormData(form);
      // Empty selectors submit an empty hidden control; it is not an entity ID.
      const resourceIds = data.getAll('resourceIds').filter(Boolean).map(Number);
      const groupIds = data.getAll('groupIds').filter(Boolean).map(Number);
      if (!resourceIds.length && !groupIds.length) {
        this.error = 'Choose at least one group or resource.';
        return;
      }

      this.busy = true;
      this.error = '';
      try {
        const response = await fetch(form.action, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({
            name: data.get('name').trim(),
            resourceIds,
            groupIds,
            excludeExternalResources: data.has('excludeExternalResources'),
          }),
        });
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        const result = await response.json();
        window.location.href = result.url;
      } catch (err) {
        this.error = err.message;
        this.busy = false;
      }
    },
  };
}

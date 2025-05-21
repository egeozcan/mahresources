document.addEventListener("alpine:init", () => {
  window.Alpine.data(
    "freeFields",
    ({ fields, name, url, jsonOutput, id, title, fromJSON }) => {
      return {
        fields,
        name,
        url,
        jsonOutput,
        id,
        title,
        fromJSON,
        remoteFields: [],
        jsonText: "",

        async init() {
          if (this.jsonOutput) {
            window.Alpine.effect(() => {
              // Serialize the whole fields array, ensuring names are present
              this.jsonText = JSON.stringify(
                this.fields
                  .filter((field) => field.name && field.name.trim() !== "")
                  .map(field => ({
                    name: field.name,
                    label: field.label,
                    type: field.type,
                    options: field.options || {} // Ensure options is always an object
                  }))
              );
            });
          }

          // If fromJSON is provided, parse it as an array of field definitions
          if (this.fromJSON) {
            try {
              let parsedFromJSON = this.fromJSON;
              if (typeof parsedFromJSON === 'string') {
                parsedFromJSON = JSON.parse(parsedFromJSON);
              }
              if (Array.isArray(parsedFromJSON)) {
                this.fields = parsedFromJSON.map(f => ({
                  name: f.name || '',
                  label: f.label || '',
                  type: f.type || 'text',
                  options: f.options || {}
                }));
              } else {
                // Attempt to convert from old object format if necessary, though not ideal
                console.warn("freeFields: fromJSON was an object, expected an array. Attempting conversion.");
                this.fields = Object.entries(parsedFromJSON).map(([name, value]) => ({
                  name: name,
                  label: name, // Default label to name if converting from old format
                  type: 'text', // Default type
                  options: {},
                  // Note: 'value' from old format is not directly applicable here
                }));
              }
            } catch (e) {
              console.error("Failed to parse fromJSON in freeFields:", e);
              // Initialize with empty or default fields if fromJSON is invalid
              if (!this.fields) this.fields = [];
            }
          } else if (!this.fields) {
             this.fields = [];
          }


          if (this.url) {
            // this.remoteFields is used for <datalist> for field names,
            // assuming it provides an array of objects with a "Key" property.
            // This might need adjustment if the structure of remoteFields is different.
            try {
                this.remoteFields = await fetch(this.url).then((x) => x.json());
            } catch(e) {
                console.error("Failed to fetch remote fields:", e);
                this.remoteFields = [];
            }
          }
        },
        // inputEvents is not used in the new template, can be removed if not needed elsewhere.
        inputEvents: {},
      };
    }
  );
});

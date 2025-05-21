document.addEventListener("alpine:init", () => {
  window.Alpine.data(
    "customDataFields",
    ({ definitions, initialMeta, inputName }) => {
      return {
        definitions: definitions || [],
        initialMeta: initialMeta || {},
        inputName: inputName || "Meta",
        values: {},
        jsonOutput: "",

        init() {
          // Initialize values based on definitions and initialMeta
          this.definitions.forEach((def) => {
            if (this.initialMeta.hasOwnProperty(def.name)) {
              this.values[def.name] = this.initialMeta[def.name];
            } else {
              // Set default value based on type
              switch (def.type) {
                case "number":
                case "rating":
                  this.values[def.name] =
                    def.options && def.options.min !== undefined
                      ? def.options.min
                      : null; // Or 0, depending on desired default
                  break;
                case "text":
                case "reference":
                  this.values[def.name] = "";
                  break;
                // Add cases for other types like boolean (false) or select (first option)
                default:
                  this.values[def.name] = "";
              }
            }
          });

          // Watch for changes in values and update jsonOutput
          window.Alpine.effect(() => {
            const output = {};
            this.definitions.forEach((def) => {
              // Only include values that are defined and not null/empty string if desired
              // For now, include all defined fields
              if (this.values.hasOwnProperty(def.name)) {
                 // Ensure numbers are stored as numbers if they are valid
                if ((def.type === 'number' || def.type === 'rating') && this.values[def.name] !== null && this.values[def.name] !== "") {
                    const numVal = parseFloat(this.values[def.name]);
                    output[def.name] = isNaN(numVal) ? null : numVal;
                } else {
                    output[def.name] = this.values[def.name];
                }
              }
            });
            this.jsonOutput = JSON.stringify(output);
          });
        },
      };
    }
  );
});

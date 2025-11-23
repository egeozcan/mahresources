document.addEventListener("alpine:init", () => {
    window.Alpine.data("schemaForm", ({ schema, value, name }) => {
        return {
            schema: typeof schema === 'string' ? JSON.parse(schema || '{}') : schema,
            value: typeof value === 'string' ? JSON.parse(value || '{}') : value,
            name,
            jsonText: '',

            init() {
                this.value = this.value || {};
                this.updateJson();
                this.renderForm();
            },

            updateJson() {
                this.jsonText = JSON.stringify(this.value);
            },

            renderForm() {
                this.$refs.container.innerHTML = '';
                if (!this.schema || Object.keys(this.schema).length === 0) return;

                const element = generateFormElement(this.schema, this.value, (newVal) => {
                    this.value = newVal;
                    this.updateJson();
                });

                this.$refs.container.appendChild(element);
            }
        }
    });
});

function inferType(val) {
    if (Array.isArray(val)) return 'array';
    if (val === null) return 'string';
    const t = typeof val;
    if (t === 'number') {
        return Number.isInteger(val) ? 'integer' : 'number';
    }
    return t;
}

function inferSchema(val) {
    const type = inferType(val);
    if (type === 'object') return { type: 'object', properties: {} };
    if (type === 'array') return { type: 'array', items: val.length ? inferSchema(val[0]) : {type: 'string'} };
    return { type };
}

function generateFormElement(schema, data, onChange) {
    const type = schema.type || inferType(data);

    if (type === 'object') {
        const container = document.createElement('div');
        container.className = "space-y-4 border-l-2 border-gray-200 pl-4 my-2";

        // Ensure data is object
        if (typeof data !== 'object' || data === null || Array.isArray(data)) {
            data = {};
            onChange(data);
        }

        if (schema.title) {
            const title = document.createElement('h4');
            title.className = "font-bold text-gray-900 text-sm";
            title.innerText = schema.title;
            container.appendChild(title);
        }

        if (schema.description) {
            const desc = document.createElement('p');
            desc.className = "text-xs text-gray-500 mb-2";
            desc.innerText = schema.description;
            container.appendChild(desc);
        }

        // 1. Render Schema-Defined Properties (Fixed Keys)
        if (schema.properties) {
            for (const key in schema.properties) {
                const propSchema = schema.properties[key];
                const wrapper = document.createElement('div');

                const label = document.createElement('label');
                label.className = "block text-sm font-medium text-gray-700";
                label.innerText = propSchema.title || key;
                wrapper.appendChild(label);

                if (propSchema.description && propSchema.type !== 'object') {
                    const desc = document.createElement('p');
                    desc.className = "text-xs text-gray-500";
                    desc.innerText = propSchema.description;
                    wrapper.appendChild(desc);
                }

                const propData = data[key];
                const inputEl = generateFormElement(propSchema, propData, (val) => {
                    data[key] = val;
                    onChange(data);
                });

                wrapper.appendChild(inputEl);
                container.appendChild(wrapper);
            }
        }

        // 2. Render Extra Properties (Editable Keys) aka Free Fields
        const extraContainer = document.createElement('div');
        container.appendChild(extraContainer);

        const renderExtras = () => {
            extraContainer.innerHTML = '';

            const knownKeys = new Set(schema.properties ? Object.keys(schema.properties) : []);
            const extraKeys = Object.keys(data).filter(k => !knownKeys.has(k));

            if (extraKeys.length > 0) {
                const divider = document.createElement('div');
                divider.className = "relative py-2";
                divider.innerHTML = '<div class="absolute inset-0 flex items-center" aria-hidden="true"><div class="w-full border-t border-gray-300"></div></div><div class="relative flex justify-start"><span class="pr-2 bg-gray-50 text-xs text-gray-500">Additional Properties</span></div>';
                extraContainer.appendChild(divider);
            }

            extraKeys.forEach(key => {
                const row = document.createElement('div');
                row.className = "flex gap-2 items-start mb-2 bg-white p-2 rounded border border-gray-200 shadow-sm";

                // Key Input
                const keyWrapper = document.createElement('div');
                keyWrapper.className = "w-1/3";
                const keyInput = document.createElement('input');
                keyInput.type = "text";
                keyInput.value = key;
                keyInput.className = "shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md";
                keyInput.placeholder = "Key";
                keyInput.onchange = (e) => {
                    const newKey = e.target.value;
                    if (newKey && newKey !== key) {
                        if (data[newKey] !== undefined) {
                            alert('Key already exists');
                            e.target.value = key;
                            return;
                        }
                        const val = data[key];
                        delete data[key];
                        data[newKey] = val;
                        onChange(data);
                        renderExtras();
                    }
                };
                keyWrapper.appendChild(keyInput);

                // Value Input
                const valWrapper = document.createElement('div');
                valWrapper.className = "flex-grow";

                const propData = data[key];

                // Check if complex type
                if (typeof propData === 'object' && propData !== null) {
                    const inferredSchema = inferSchema(propData);
                    const inputEl = generateFormElement(inferredSchema, propData, (val) => {
                        data[key] = val;
                        onChange(data);
                    });
                    valWrapper.appendChild(inputEl);
                } else {
                    // Smart Input for primitives
                    const inputEl = document.createElement('input');
                    inputEl.type = "text";
                    inputEl.className = "shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md";

                    let displayVal = propData;
                    if (typeof propData === 'string') {
                        // Check if it looks like JSON but not string
                        try {
                            const parsed = JSON.parse(propData);
                            if (typeof parsed !== 'string') {
                                displayVal = JSON.stringify(propData);
                            }
                        } catch {
                            // keep raw string
                        }
                    } else {
                        // Number, boolean, null
                        if (propData === undefined) displayVal = "";
                        else displayVal = JSON.stringify(propData);
                    }

                    inputEl.value = displayVal;

                    inputEl.oninput = (e) => {
                        // Sync raw string value during typing so data is not lost on submit before blur
                        data[key] = e.target.value;
                        onChange(data);
                    };

                    inputEl.onblur = (e) => {
                        const val = e.target.value;
                        let finalVal = val;
                        try {
                            finalVal = JSON.parse(val);
                        } catch {
                            // keep as string
                        }

                        // Update if changed (type or value)
                        if (data[key] !== finalVal) {
                            data[key] = finalVal;
                            onChange(data);
                            renderExtras();
                        }
                    };

                    inputEl.onkeyup = (e) => {
                        if(e.key === 'Enter') e.target.blur();
                    };

                    valWrapper.appendChild(inputEl);
                }

                // Remove Button
                const removeBtn = document.createElement('button');
                removeBtn.type = "button";
                removeBtn.innerText = "×";
                removeBtn.className = "text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50 self-start mt-0.5";
                removeBtn.title = "Remove field";
                removeBtn.onclick = () => {
                    delete data[key];
                    onChange(data);
                    renderExtras();
                };

                row.appendChild(keyWrapper);
                row.appendChild(valWrapper);
                row.appendChild(removeBtn);
                extraContainer.appendChild(row);
            });

            // Add Button
            const addBtn = document.createElement('button');
            addBtn.type = "button";
            addBtn.innerText = "Add Field";
            addBtn.className = "mt-2 inline-flex items-center px-2.5 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500";
            addBtn.onclick = () => {
                let newKey = "newField";
                let counter = 1;
                while (data[newKey] !== undefined) {
                    newKey = `newField${counter++}`;
                }
                data[newKey] = "";
                onChange(data);
                renderExtras();
            };
            extraContainer.appendChild(addBtn);
        };

        renderExtras();

        return container;
    }

    if (type === 'array') {
        const container = document.createElement('div');
        container.className = "space-y-2 border-l-2 border-indigo-200 pl-4 py-2 my-2";

        if (schema.title) {
            const title = document.createElement('h4');
            title.className = "font-bold text-gray-900 text-sm";
            title.innerText = schema.title;
            container.appendChild(title);
        }

        if (!Array.isArray(data)) {
            data = [];
            onChange(data);
        }

        const list = document.createElement('div');
        list.className = "space-y-2";

        const renderList = () => {
            list.innerHTML = '';
            data.forEach((item, index) => {
                const row = document.createElement('div');
                row.className = "flex gap-2 items-start";

                // For array items, we use schema.items.
                // If schema.items is generic (e.g. inferred), it works.
                const itemSchema = schema.items || inferSchema(item);
                const itemInput = generateFormElement(itemSchema, item, (val) => {
                    data[index] = val;
                    onChange(data);
                });

                // Wrap input to grow
                const inputWrapper = document.createElement('div');
                inputWrapper.className = "flex-grow";
                inputWrapper.appendChild(itemInput);

                const removeBtn = document.createElement('button');
                removeBtn.type = "button";
                removeBtn.innerText = "×";
                removeBtn.className = "text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50";
                removeBtn.title = "Remove item";
                removeBtn.onclick = () => {
                    data.splice(index, 1);
                    onChange(data);
                    renderList();
                };

                row.appendChild(inputWrapper);
                row.appendChild(removeBtn);
                list.appendChild(row);
            });
        };

        renderList();

        const addBtn = document.createElement('button');
        addBtn.type = "button";
        addBtn.innerText = "Add Item";
        addBtn.className = "mt-2 inline-flex items-center px-2.5 py-1.5 border border-transparent text-xs font-medium rounded text-indigo-700 bg-indigo-100 hover:bg-indigo-200";
        addBtn.onclick = () => {
            data.push(getDefaultValue(schema.items || {type:'string'}));
            onChange(data);
            renderList();
        };

        container.appendChild(list);
        container.appendChild(addBtn);
        return container;
    }

    // Primitives
    const input = document.createElement('input');

    if (type === 'boolean') {
        input.type = 'checkbox';
        input.className = "focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded mt-1";
        input.checked = !!data;
        input.onchange = (e) => onChange(e.target.checked);
    } else {
        input.className = "shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1";

        if (type === 'integer' || type === 'number') {
            input.type = 'number';
            if (type === 'integer') input.step = "1";
            input.value = data !== undefined && data !== null ? data : '';
            input.oninput = (e) => onChange(type === 'integer' ? parseInt(e.target.value) : parseFloat(e.target.value));
        } else {
            input.type = 'text';
            input.value = data || '';
            input.oninput = (e) => onChange(e.target.value);
        }
    }

    return input;
}

function getDefaultValue(schema) {
    if (schema.type === 'object') return {};
    if (schema.type === 'array') return [];
    if (schema.type === 'boolean') return false;
    if (schema.type === 'number' || schema.type === 'integer') return 0;
    return "";
}
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

        // Defined Properties
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

        // Extra Properties
        const knownKeys = new Set(schema.properties ? Object.keys(schema.properties) : []);
        const extraKeys = Object.keys(data).filter(k => !knownKeys.has(k));

        if (extraKeys.length > 0) {
            const extraContainer = document.createElement('div');
            extraContainer.className = "mt-4 pt-4 border-t border-gray-200";
            const extraTitle = document.createElement('h5');
            extraTitle.className = "text-xs font-bold text-gray-500 uppercase tracking-wider mb-2";
            extraTitle.innerText = "Additional Properties";
            extraContainer.appendChild(extraTitle);

            extraKeys.forEach(key => {
                const row = document.createElement('div');
                row.className = "mb-2 border border-dashed border-gray-300 p-2 rounded relative";

                const header = document.createElement('div');
                header.className = "flex justify-between items-center mb-1";

                const label = document.createElement('label');
                label.className = "block text-sm font-medium text-gray-500 italic";
                label.innerText = key;
                header.appendChild(label);

                const removeBtn = document.createElement('button');
                removeBtn.type = "button";
                removeBtn.innerText = "×";
                removeBtn.className = "text-gray-400 hover:text-red-600 font-bold";
                removeBtn.title = "Remove field";
                removeBtn.onclick = () => {
                    delete data[key];
                    onChange(data);
                    row.remove();
                    if (extraContainer.children.length <= 1) { // Title only
                        extraContainer.remove();
                    }
                };
                header.appendChild(removeBtn);
                row.appendChild(header);

                const propData = data[key];
                const inferredSchema = inferSchema(propData);

                const inputEl = generateFormElement(inferredSchema, propData, (val) => {
                    data[key] = val;
                    onChange(data);
                });

                row.appendChild(inputEl);
                extraContainer.appendChild(row);
            });
            container.appendChild(extraContainer);
        }

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
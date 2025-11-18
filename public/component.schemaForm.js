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

function generateFormElement(schema, data, onChange) {
    const type = schema.type;

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

                const itemInput = generateFormElement(schema.items, item, (val) => {
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
            data.push(getDefaultValue(schema.items));
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
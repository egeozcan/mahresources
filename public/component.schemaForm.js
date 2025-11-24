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
    if (val === null) return 'null';
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

function scoreSchemaMatch(schema, data) {
    if (schema.const !== undefined) return schema.const === data ? 100 : 0;

    const dataType = inferType(data);
    let schemaType = schema.type;

    // Handle array of types in schema e.g. ["string", "null"]
    if (Array.isArray(schemaType)) {
        if (schemaType.includes(dataType)) return 10;
        // Fuzzy match number/integer
        if (dataType === 'integer' && schemaType.includes('number')) return 9;
        // Fuzzy match null in primitives
        if (dataType === 'null' && (schemaType.includes('string') || schemaType.includes('number'))) return 5;
        return 0;
    }

    if (schemaType && schemaType !== dataType) {
        // Allow integer data for number schema
        if (schemaType === 'number' && dataType === 'integer') return 9;
        // Allow null data for complex schema if nullable not explicitly set but implied? (Unlikely in rigorous schema)
        return 0;
    }

    if (dataType === 'object' && schema.properties) {
        const dataKeys = Object.keys(data);
        const schemaKeys = Object.keys(schema.properties);
        const matchCount = dataKeys.filter(k => schemaKeys.includes(k)).length;
        return matchCount + 10;
    }

    // Default match for matching types
    return 10;
}

function generateFormElement(schema, data, onChange) {
    // Handle oneOf
    if (schema.oneOf && Array.isArray(schema.oneOf)) {
        const container = document.createElement('div');
        container.className = "space-y-2 border-l-4 border-indigo-100 pl-4 py-2 my-2";

        if (schema.title) {
            const title = document.createElement('h4');
            title.className = "font-bold text-gray-900 text-sm";
            title.innerText = schema.title;
            container.appendChild(title);
        }

        let activeIndex = 0;
        // Determine best match
        if (data !== undefined) {
            let maxScore = -1;
            schema.oneOf.forEach((s, idx) => {
                const score = scoreSchemaMatch(s, data);
                if (score > maxScore) {
                    maxScore = score;
                    activeIndex = idx;
                }
            });
        }

        const select = document.createElement('select');
        select.className = "block w-full pl-3 pr-10 py-2 text-base border-gray-300 focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm rounded-md mb-2";

        schema.oneOf.forEach((opt, idx) => {
            const option = document.createElement('option');
            option.value = idx;

            let typeLabel = opt.type;
            if(Array.isArray(opt.type)) typeLabel = opt.type.join('/');

            option.text = opt.title || opt.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`;
            if (idx === activeIndex) option.selected = true;
            select.appendChild(option);
        });

        const formWrapper = document.createElement('div');

        const renderOption = (idx) => {
            formWrapper.innerHTML = '';
            const optSchema = schema.oneOf[idx];

            // If switching schemas and data is incompatible, reset data
            // Simple check: if types mismatch drastically.
            // For now, rely on generateFormElement to adapt or reset if needed inside.
            // But strictly, we should maybe reset to default of new schema if score is 0.
            // Let's keep current data and let inner logic handle/cast it.

            const el = generateFormElement(optSchema, data, (val) => {
                data = val;
                onChange(val);
            });
            formWrapper.appendChild(el);
        };

        select.onchange = (e) => {
            const idx = parseInt(e.target.value);
            const optSchema = schema.oneOf[idx];
            // Reset data to default of new schema
            data = getDefaultValue(optSchema);
            onChange(data);
            renderOption(idx);
        };

        container.appendChild(select);
        container.appendChild(formWrapper);

        if (data === undefined) {
            data = getDefaultValue(schema.oneOf[activeIndex]);
            onChange(data);
        }

        renderOption(activeIndex);
        return container;
    }

    // Handle enum
    if (schema.enum) {
        const select = document.createElement('select');
        select.className = "shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1";

        // Check if current data is in enum. If not, we might need to prepend it or default.
        let hasValue = false;

        // Add a blank option if current data is null/undefined and null is allowed or not in enum
        if (data === null || data === undefined) {
            const nullOpt = document.createElement('option');
            nullOpt.value = "";
            nullOpt.text = "-- select --";
            nullOpt.selected = true;
            select.appendChild(nullOpt);
        }

        schema.enum.forEach(val => {
            const option = document.createElement('option');
            option.value = val;
            option.text = val;
            if (val === data) {
                option.selected = true;
                hasValue = true;
            }
            select.appendChild(option);
        });

        if (data !== null && data !== undefined && !hasValue) {
            const option = document.createElement('option');
            option.value = data;
            option.text = data + " (current)";
            option.selected = true;
            select.appendChild(option);
        }

        select.onchange = (e) => {
            const valStr = e.target.value;
            // Attempt to find correct typed value from enum
            // Note: this simple logic assumes string representation matches.
            // JSON enum can have mixed types.
            let val = valStr;
            const match = schema.enum.find(ev => String(ev) === valStr);
            if (match !== undefined) val = match;

            // Handle number conversion if schema implies it
            if (match === undefined && (schema.type === 'integer' || schema.type === 'number')) {
                val = parseFloat(valStr);
            }

            onChange(val);
        };
        return select;
    }

    // Handle const
    if (schema.const !== undefined) {
        const input = document.createElement('input');
        input.type = 'text';
        input.value = schema.const;
        input.disabled = true;
        input.className = "shadow-sm bg-gray-100 block w-full sm:text-sm border-gray-300 rounded-md mt-1 text-gray-500";
        if (data !== schema.const) {
            setTimeout(() => onChange(schema.const), 0);
        }
        return input;
    }

    // Normalize Type (handle array of types e.g. ["string", "null"])
    let type = schema.type;
    if (Array.isArray(type)) {
        const currentType = inferType(data);
        if (type.includes(currentType)) {
            type = currentType;
        } else {
            // Prefer non-null
            type = type.find(t => t !== 'null') || type[0];
        }
    }
    // Fallback inference
    type = type || inferType(data);

    // Null type handling
    if (type === 'null') {
        // If schema forces null, show it.
        // If schema is ["object", "null"] and we decided on "null" because data is null,
        // we might want to allow switching to object.
        // For now, simple display.
        const wrapper = document.createElement('div');
        wrapper.className = "mt-1 flex items-center text-sm text-gray-500 italic";
        wrapper.innerHTML = "<span>null</span>";

        // If schema allowed other types (from array), give a button to switch (primitive oneOf)
        if (Array.isArray(schema.type) && schema.type.length > 1) {
            const switchBtn = document.createElement('button');
            switchBtn.type = "button";
            switchBtn.className = "ml-2 text-xs text-indigo-600 hover:text-indigo-800 underline";
            switchBtn.innerText = "Initialize";
            switchBtn.onclick = () => {
                const nextType = schema.type.find(t => t !== 'null');
                if (nextType === 'object') onChange({});
                else if (nextType === 'array') onChange([]);
                else onChange(""); // default string
            };
            wrapper.appendChild(switchBtn);
        }
        return wrapper;
    }

    if (type === 'object') {
        const container = document.createElement('div');
        container.className = "space-y-4 border-l-2 border-gray-200 pl-4 my-2";

        if (typeof data !== 'object' || data === null || Array.isArray(data)) {
            // If data mismatches, we might want to reset, or just error.
            // Resetting safe default:
            data = {};
            setTimeout(() => onChange(data), 0);
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

        // 1. Render Schema-Defined Properties
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

                // Pass undefined if key missing, let child handle default if needed
                const inputEl = generateFormElement(propSchema, data[key], (val) => {
                    data[key] = val;
                    onChange(data);
                });

                wrapper.appendChild(inputEl);
                container.appendChild(wrapper);
            }
        }

        // 2. Render Extra Properties (Free Fields)
        // Only if additionalProperties is not false (default true)
        if (schema.additionalProperties !== false) {
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

                    if (typeof propData === 'object' && propData !== null) {
                        const inferredSchema = inferSchema(propData);
                        const inputEl = generateFormElement(inferredSchema, propData, (val) => {
                            data[key] = val;
                            onChange(data);
                        });
                        valWrapper.appendChild(inputEl);
                    } else {
                        // Smart Input
                        const inputEl = document.createElement('input');
                        inputEl.type = "text";
                        inputEl.className = "shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md";

                        let displayVal = propData;
                        if (typeof propData === 'string') {
                            try {
                                const parsed = JSON.parse(propData);
                                if (typeof parsed !== 'string') displayVal = JSON.stringify(propData);
                            } catch {}
                        } else {
                            if (propData === undefined) displayVal = "";
                            else displayVal = JSON.stringify(propData);
                        }

                        inputEl.value = displayVal;
                        inputEl.oninput = (e) => { data[key] = e.target.value; onChange(data); };
                        inputEl.onblur = (e) => {
                            const val = e.target.value;
                            let finalVal = val;
                            try { finalVal = JSON.parse(val); } catch {}
                            if (data[key] !== finalVal) {
                                data[key] = finalVal;
                                onChange(data);
                                renderExtras();
                            }
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
            setTimeout(() => onChange(data), 0);
        }

        const list = document.createElement('div');
        list.className = "space-y-2";

        const renderList = () => {
            list.innerHTML = '';
            data.forEach((item, index) => {
                const row = document.createElement('div');
                row.className = "flex gap-2 items-start";

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

        // Handle formats and patterns
        if (schema.format === 'date') input.type = 'date';
        else if (schema.format === 'date-time') input.type = 'datetime-local';
        else if (schema.format === 'email') input.type = 'email';
        else if (schema.format === 'uri' || schema.format === 'url') input.type = 'url';
        else input.type = 'text';

        if (schema.pattern) input.pattern = schema.pattern;

        if (type === 'integer' || type === 'number') {
            input.type = 'number';
            if (type === 'integer') input.step = "1";
            else input.step = "any";
            input.value = data !== undefined && data !== null ? data : '';
            input.oninput = (e) => {
                const val = e.target.value;
                if (val === '') {
                    // If field allowed null, maybe set null? But standard HTML number input clears to "".
                    // If schema says ["number", "null"], we might want null.
                    if (Array.isArray(schema.type) && schema.type.includes('null')) onChange(null);
                    else onChange(undefined);
                } else {
                    onChange(type === 'integer' ? parseInt(val) : parseFloat(val));
                }
            }
        } else {
            input.value = data || '';
            input.oninput = (e) => onChange(e.target.value);
        }
    }

    return input;
}

function getDefaultValue(schema) {
    if (schema.default !== undefined) return schema.default;
    if (schema.const !== undefined) return schema.const;
    if (schema.type === 'object') return {};
    if (schema.type === 'array') return [];
    if (schema.type === 'boolean') return false;
    if (schema.type === 'number' || schema.type === 'integer') return 0;

    if (Array.isArray(schema.type)) {
        if (schema.type.includes('string')) return "";
        if (schema.type.includes('number') || schema.type.includes('integer')) return 0;
        if (schema.type.includes('boolean')) return false;
        if (schema.type.includes('object')) return {};
        if (schema.type.includes('array')) return [];
        if (schema.type.includes('null')) return null;
    }

    if (schema.oneOf && schema.oneOf.length > 0) return getDefaultValue(schema.oneOf[0]);

    return "";
}
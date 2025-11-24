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

                const renderRoot = () => {
                    this.$refs.container.innerHTML = '';
                    const element = generateFormElement(this.schema, this.value, (newVal) => {
                        const oldVal = this.value;
                        this.value = newVal;
                        this.updateJson();
                        if ((oldVal === null && newVal !== null) || (oldVal !== null && newVal === null)) {
                            renderRoot();
                        }
                    }, this.schema);
                    this.$refs.container.appendChild(element);
                };

                renderRoot();
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

function resolveRef(ref, root) {
    if (typeof ref !== 'string' || !ref.startsWith('#/')) return null;
    const parts = ref.split('/').slice(1);
    let current = root;
    for (const part of parts) {
        if (current && typeof current === 'object' && part in current) {
            current = current[part];
        } else {
            return null;
        }
    }
    return current;
}

function scoreSchemaMatch(schema, data, rootSchema) {
    // Resolve ref if present for scoring
    if (schema.$ref) {
        const resolved = resolveRef(schema.$ref, rootSchema);
        if (resolved) {
            // Simple merge for scoring
            schema = { ...resolved, ...schema };
        }
    }

    if (schema.const !== undefined) return schema.const === data ? 100 : 0;

    const dataType = inferType(data);
    let schemaType = schema.type;

    if (Array.isArray(schemaType)) {
        if (schemaType.includes(dataType)) return 10;
        if (dataType === 'integer' && schemaType.includes('number')) return 9;
        if (dataType === 'null' && (schemaType.includes('string') || schemaType.includes('number'))) return 5;
        return 0;
    }

    if (schemaType && schemaType !== dataType) {
        if (schemaType === 'number' && dataType === 'integer') return 9;
        return 0;
    }

    if (dataType === 'object' && schema.properties) {
        const dataKeys = Object.keys(data);
        const schemaKeys = Object.keys(schema.properties);
        const matchCount = dataKeys.filter(k => schemaKeys.includes(k)).length;
        return matchCount + 10;
    }

    return 10;
}

function generateFormElement(schema, data, onChange, rootSchema) {
    rootSchema = rootSchema || schema;

    // Handle $ref
    if (schema.$ref) {
        const resolved = resolveRef(schema.$ref, rootSchema);
        if (resolved) {
            const mergedSchema = { ...resolved, ...schema };
            delete mergedSchema.$ref;
            return generateFormElement(mergedSchema, data, onChange, rootSchema);
        }
        // If ref not found, display error or fallback?
        const err = document.createElement('div');
        err.className = "text-red-500 text-xs";
        err.innerText = "Unresolvable reference: " + schema.$ref;
        return err;
    }

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

        if (schema.description) {
            const desc = document.createElement('p');
            desc.className = "text-xs text-gray-500 mb-2";
            desc.innerText = schema.description;
            container.appendChild(desc);
        }

        let activeIndex = 0;
        if (data !== undefined) {
            let maxScore = -1;
            schema.oneOf.forEach((s, idx) => {
                const score = scoreSchemaMatch(s, data, rootSchema);
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
            if(opt.$ref) typeLabel = "ref"; // Simplified label for refs

            option.text = opt.title || opt.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`;
            if (idx === activeIndex) option.selected = true;
            select.appendChild(option);
        });

        const formWrapper = document.createElement('div');

        const renderOption = (idx) => {
            formWrapper.innerHTML = '';
            const optSchema = schema.oneOf[idx];

            const el = generateFormElement(optSchema, data, (val) => {
                const oldVal = data;
                data = val;
                onChange(val);
                if ((oldVal === null && val !== null) || (oldVal !== null && val === null)) {
                    renderOption(idx);
                }
            }, rootSchema);
            formWrapper.appendChild(el);
        };

        select.onchange = (e) => {
            const idx = parseInt(e.target.value);
            const optSchema = schema.oneOf[idx];
            data = getDefaultValue(optSchema, rootSchema);
            onChange(data);
            renderOption(idx);
        };

        container.appendChild(select);
        container.appendChild(formWrapper);

        if (data === undefined) {
            data = getDefaultValue(schema.oneOf[activeIndex], rootSchema);
            onChange(data);
        }

        renderOption(activeIndex);
        return container;
    }

    // Handle enum
    if (schema.enum) {
        const select = document.createElement('select');
        select.className = "shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md mt-1";

        let hasValue = false;

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
            let val = valStr;
            const match = schema.enum.find(ev => String(ev) === valStr);
            if (match !== undefined) val = match;

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

    // Normalize Type
    let type = schema.type;
    if (Array.isArray(type)) {
        const currentType = inferType(data);
        if (type.includes(currentType)) {
            type = currentType;
        } else {
            type = type.find(t => t !== 'null') || type[0];
        }
    }
    type = type || inferType(data);

    // Null type handling
    if (type === 'null') {
        const wrapper = document.createElement('div');
        wrapper.className = "mt-1 flex items-center text-sm text-gray-500 italic";
        wrapper.innerHTML = "<span>null</span>";

        if (Array.isArray(schema.type) && schema.type.length > 1) {
            const switchBtn = document.createElement('button');
            switchBtn.type = "button";
            switchBtn.className = "ml-2 text-xs text-indigo-600 hover:text-indigo-800 underline";
            switchBtn.innerText = "Initialize";
            switchBtn.onclick = () => {
                const nextType = schema.type.find(t => t !== 'null');
                let newVal;
                if (nextType === 'object') newVal = {};
                else if (nextType === 'array') newVal = [];
                else if (nextType === 'boolean') newVal = false;
                else if (nextType === 'number' || nextType === 'integer') newVal = 0;
                else newVal = "";
                onChange(newVal);
            };
            wrapper.appendChild(switchBtn);
        }
        return wrapper;
    }

    if (type === 'object') {
        const container = document.createElement('div');
        container.className = "space-y-4 border-l-2 border-gray-200 pl-4 my-2";

        if (typeof data !== 'object' || data === null || Array.isArray(data)) {
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

                const inputContainer = document.createElement('div');
                wrapper.appendChild(inputContainer);

                const renderProp = () => {
                    inputContainer.innerHTML = '';
                    const propData = data[key];

                    const inputEl = generateFormElement(propSchema, propData, (val) => {
                        const oldVal = data[key];
                        data[key] = val;
                        onChange(data);
                        if ((oldVal === null && val !== null) || (oldVal !== null && val === null)) {
                            renderProp();
                        }
                    }, rootSchema);
                    inputContainer.appendChild(inputEl);
                };

                renderProp();
                container.appendChild(wrapper);
            }
        }

        // 2. Render Extra Properties (Free Fields)
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

                    const valWrapper = document.createElement('div');
                    valWrapper.className = "flex-grow";

                    const propData = data[key];

                    if (typeof propData === 'object' && propData !== null) {
                        const inferredSchema = inferSchema(propData);
                        const inputEl = generateFormElement(inferredSchema, propData, (val) => {
                            data[key] = val;
                            onChange(data);
                        }, rootSchema);
                        valWrapper.appendChild(inputEl);
                    } else {
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

                const inputWrapper = document.createElement('div');
                inputWrapper.className = "flex-grow";

                const renderItem = () => {
                    inputWrapper.innerHTML = '';
                    const currentItem = data[index];
                    const itemSchema = schema.items || inferSchema(currentItem);
                    const itemInput = generateFormElement(itemSchema, currentItem, (val) => {
                        const oldVal = data[index];
                        data[index] = val;
                        onChange(data);
                        if ((oldVal === null && val !== null) || (oldVal !== null && val === null)) {
                            renderItem();
                        }
                    }, rootSchema);
                    inputWrapper.appendChild(itemInput);
                };
                renderItem();

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
            data.push(getDefaultValue(schema.items || {type:'string'}, rootSchema));
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

function getDefaultValue(schema, rootSchema) {
    if (schema.$ref) {
        const resolved = resolveRef(schema.$ref, rootSchema || schema);
        if (resolved) {
            return getDefaultValue({...resolved, ...schema, $ref: undefined}, rootSchema);
        }
    }
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

    if (schema.oneOf && schema.oneOf.length > 0) return getDefaultValue(schema.oneOf[0], rootSchema);

    return "";
}
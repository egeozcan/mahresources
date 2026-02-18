export function schemaForm({ schema, value, name }) {
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
}

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

// Merge two schemas (for allOf)
function mergeSchemas(base, extension) {
    const merged = { ...base };
    for (const key in extension) {
        if (key === 'properties') {
            merged.properties = { ...(base.properties || {}), ...extension.properties };
        } else if (key === 'required') {
            merged.required = [...new Set([...(base.required || []), ...(extension.required || [])])];
        } else if (!['allOf', 'anyOf', 'oneOf', '$ref'].includes(key)) {
            merged[key] = extension[key];
        }
    }
    return merged;
}

// Evaluate if/then/else condition
function evaluateCondition(conditionSchema, data) {
    if (!conditionSchema || !conditionSchema.properties) return true;
    for (const key in conditionSchema.properties) {
        const propSchema = conditionSchema.properties[key];
        if (propSchema.const !== undefined && data?.[key] !== propSchema.const) return false;
        if (propSchema.enum && !propSchema.enum.includes(data?.[key])) return false;
    }
    return true;
}

// Create error message span
function createErrorSpan(message) {
    const span = document.createElement('span');
    span.className = "block text-sm text-red-500 mt-1";
    span.setAttribute('role', 'alert');
    span.innerText = message;
    return span;
}

// Create constraint hint span
function createConstraintHint(text) {
    const hint = document.createElement('span');
    hint.className = "text-xs text-gray-400 block mt-1";
    hint.innerText = text;
    return hint;
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

let uniqueIdCounter = 0;
function generateUniqueId(prefix = 'schema-field') {
    return `${prefix}-${++uniqueIdCounter}`;
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

    // Handle allOf - merge all schemas
    if (schema.allOf && Array.isArray(schema.allOf)) {
        let merged = { ...schema };
        delete merged.allOf;
        for (const sub of schema.allOf) {
            const resolved = sub.$ref ? resolveRef(sub.$ref, rootSchema) : sub;
            if (resolved) merged = mergeSchemas(merged, resolved);
        }
        return generateFormElement(merged, data, onChange, rootSchema);
    }

    // Handle anyOf - similar to oneOf with different styling
    if (schema.anyOf && Array.isArray(schema.anyOf)) {
        const container = document.createElement('div');
        container.className = "space-y-2 border-l-4 border-green-100 pl-4 py-2 my-2";

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
            schema.anyOf.forEach((s, idx) => {
                const score = scoreSchemaMatch(s, data, rootSchema);
                if (score > maxScore) {
                    maxScore = score;
                    activeIndex = idx;
                }
            });
        }

        const select = document.createElement('select');
        select.className = "block w-full pl-3 pr-10 py-2 text-base border-gray-300 focus:outline-none focus:ring-green-500 focus:border-green-500 sm:text-sm rounded-md mb-2";

        schema.anyOf.forEach((opt, idx) => {
            const option = document.createElement('option');
            option.value = idx;

            let typeLabel = opt.type;
            if(Array.isArray(opt.type)) typeLabel = opt.type.join('/');
            if(opt.$ref) typeLabel = "ref";

            option.text = opt.title || opt.description || `Option ${idx + 1} (${typeLabel || 'mixed'})`;
            if (idx === activeIndex) option.selected = true;
            select.appendChild(option);
        });

        const formWrapper = document.createElement('div');

        const renderOption = (idx) => {
            formWrapper.innerHTML = '';
            const optSchema = schema.anyOf[idx];

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
            const optSchema = schema.anyOf[idx];
            data = getDefaultValue(optSchema, rootSchema);
            onChange(data);
            renderOption(idx);
        };

        container.appendChild(select);
        container.appendChild(formWrapper);

        if (data === undefined) {
            data = getDefaultValue(schema.anyOf[activeIndex], rootSchema);
            onChange(data);
        }

        renderOption(activeIndex);
        return container;
    }

    // Handle if/then/else - conditional schema application
    if (schema.if) {
        const baseSchema = { ...schema };
        delete baseSchema.if;
        delete baseSchema.then;
        delete baseSchema.else;

        const container = document.createElement('div');
        let lastConditionMet = null;

        const renderConditional = () => {
            const conditionMet = evaluateCondition(schema.if, data);
            if (conditionMet === lastConditionMet && container.firstChild) return;
            lastConditionMet = conditionMet;

            container.innerHTML = '';
            const applicable = conditionMet
                ? mergeSchemas(baseSchema, schema.then || {})
                : mergeSchemas(baseSchema, schema.else || {});

            const el = generateFormElement(applicable, data, (val) => {
                data = val;
                onChange(val);
                renderConditional();
            }, rootSchema);
            container.appendChild(el);
        };

        renderConditional();
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
            onChange(schema.const);
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

        // 1. Render Schema-Defined Properties
        const requiredFields = new Set(schema.required || []);

        if (schema.properties) {
            for (const key in schema.properties) {
                const propSchema = schema.properties[key];
                const wrapper = document.createElement('div');
                const fieldId = generateUniqueId(`field-${key}`);
                const isRequired = requiredFields.has(key);

                const label = document.createElement('label');
                label.className = "block text-sm font-medium text-gray-700";
                label.setAttribute('for', fieldId);

                const labelText = document.createTextNode(propSchema.title || key);
                label.appendChild(labelText);

                if (isRequired) {
                    const asterisk = document.createElement('span');
                    asterisk.className = "text-red-500 ml-1";
                    asterisk.innerText = "*";
                    asterisk.setAttribute('aria-hidden', 'true');
                    label.appendChild(asterisk);
                }
                wrapper.appendChild(label);

                if (propSchema.description && propSchema.type !== 'object') {
                    const desc = document.createElement('p');
                    desc.className = "text-xs text-gray-500";
                    desc.id = `${fieldId}-desc`;
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

                    // Set ID, aria-describedby, and required on the generated input
                    if (inputEl.tagName && ['INPUT', 'SELECT', 'TEXTAREA'].includes(inputEl.tagName)) {
                        inputEl.id = fieldId;
                        if (propSchema.description) {
                            inputEl.setAttribute('aria-describedby', `${fieldId}-desc`);
                        }
                        if (isRequired) {
                            inputEl.required = true;
                            inputEl.setAttribute('aria-required', 'true');
                        }
                    }

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
                    keyInput.setAttribute('aria-label', 'Property name');
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
                        inputEl.setAttribute('aria-label', `Value for ${key}`);

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
                    removeBtn.setAttribute('aria-label', `Remove field ${key}`);
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
                addBtn.setAttribute('aria-label', 'Add new custom field');
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
            onChange(data);
        }

        // Check constraints
        const hasMinItems = schema.minItems !== undefined;
        const hasMaxItems = schema.maxItems !== undefined;
        const canRemove = () => !hasMinItems || data.length > schema.minItems;
        const canAdd = () => !hasMaxItems || data.length < schema.maxItems;

        // Item count display
        const countDisplay = document.createElement('span');
        countDisplay.className = "text-xs text-gray-500";
        const updateCount = () => {
            let text = `${data.length} item${data.length !== 1 ? 's' : ''}`;
            if (hasMinItems || hasMaxItems) {
                const min = schema.minItems || 0;
                const max = schema.maxItems !== undefined ? schema.maxItems : '∞';
                text += ` (${min}-${max})`;
            }
            countDisplay.innerText = text;

            // Show error state if out of bounds
            if ((hasMinItems && data.length < schema.minItems) || (hasMaxItems && data.length > schema.maxItems)) {
                countDisplay.className = "text-xs text-red-500";
            } else {
                countDisplay.className = "text-xs text-gray-500";
            }
        };

        const list = document.createElement('div');
        list.className = "space-y-2";

        let addBtn; // Declare early so renderList can reference it

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
                removeBtn.title = "Remove item";
                removeBtn.setAttribute('aria-label', `Remove item ${index + 1}`);

                // Update remove button state based on minItems
                if (canRemove()) {
                    removeBtn.className = "text-red-600 font-bold px-2 py-1 border rounded hover:bg-red-50";
                    removeBtn.disabled = false;
                    removeBtn.onclick = () => {
                        data.splice(index, 1);
                        onChange(data);
                        renderList();
                        updateCount();
                        updateAddButton();
                    };
                } else {
                    removeBtn.className = "text-gray-400 font-bold px-2 py-1 border rounded cursor-not-allowed opacity-50";
                    removeBtn.disabled = true;
                }

                row.appendChild(inputWrapper);
                row.appendChild(removeBtn);
                list.appendChild(row);
            });
        };

        const updateAddButton = () => {
            if (canAdd()) {
                addBtn.disabled = false;
                addBtn.className = "mt-2 inline-flex items-center px-2.5 py-1.5 border border-transparent text-xs font-medium rounded text-indigo-700 bg-indigo-100 hover:bg-indigo-200";
            } else {
                addBtn.disabled = true;
                addBtn.className = "mt-2 inline-flex items-center px-2.5 py-1.5 border border-transparent text-xs font-medium rounded text-gray-400 bg-gray-100 cursor-not-allowed opacity-50";
            }
        };

        renderList();

        addBtn = document.createElement('button');
        addBtn.type = "button";
        addBtn.innerText = "Add Item";
        addBtn.setAttribute('aria-label', `Add item to ${schema.title || 'list'}`);
        addBtn.onclick = () => {
            if (!canAdd()) return;
            data.push(getDefaultValue(schema.items || {type:'string'}, rootSchema));
            onChange(data);
            renderList();
            updateCount();
            updateAddButton();
        };

        // Initial button state
        updateAddButton();
        updateCount();

        container.appendChild(list);

        // Footer with add button and count
        const footer = document.createElement('div');
        footer.className = "flex items-center gap-3";
        footer.appendChild(addBtn);
        if (hasMinItems || hasMaxItems) {
            footer.appendChild(countDisplay);
        }
        container.appendChild(footer);

        return container;
    }

    // Primitives
    const input = document.createElement('input');
    const wrapper = document.createElement('div');
    let errorContainer = null;

    // Helper to show/hide validation errors
    const showError = (message) => {
        if (!errorContainer) {
            errorContainer = document.createElement('div');
            wrapper.appendChild(errorContainer);
        }
        errorContainer.innerHTML = '';
        if (message) {
            errorContainer.appendChild(createErrorSpan(message));
            input.classList.add('border-red-500');
            input.classList.remove('border-gray-300');
        } else {
            input.classList.remove('border-red-500');
            input.classList.add('border-gray-300');
        }
    };

    if (type === 'boolean') {
        input.type = 'checkbox';
        input.className = "focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded mt-1";
        input.checked = !!data;
        input.onchange = (e) => onChange(e.target.checked);
        return input; // Boolean doesn't need wrapper
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

            // Add min/max HTML5 attributes
            if (schema.minimum !== undefined) input.min = schema.minimum;
            if (schema.maximum !== undefined) input.max = schema.maximum;

            input.value = data !== undefined && data !== null ? data : '';

            // Validation function for exclusive bounds
            const validateNumber = (val) => {
                if (val === '' || val === undefined || val === null) return null;
                const num = parseFloat(val);
                if (isNaN(num)) return null;
                if (schema.exclusiveMinimum !== undefined && num <= schema.exclusiveMinimum) {
                    return `Must be greater than ${schema.exclusiveMinimum}`;
                }
                if (schema.exclusiveMaximum !== undefined && num >= schema.exclusiveMaximum) {
                    return `Must be less than ${schema.exclusiveMaximum}`;
                }
                if (schema.minimum !== undefined && num < schema.minimum) {
                    return `Must be at least ${schema.minimum}`;
                }
                if (schema.maximum !== undefined && num > schema.maximum) {
                    return `Must be at most ${schema.maximum}`;
                }
                return null;
            };

            input.oninput = (e) => {
                const val = e.target.value;
                if (val === '') {
                    if (Array.isArray(schema.type) && schema.type.includes('null')) onChange(null);
                    else onChange(undefined);
                } else {
                    onChange(type === 'integer' ? parseInt(val) : parseFloat(val));
                }
            };

            input.onblur = (e) => {
                showError(validateNumber(e.target.value));
            };

            // Add constraint hint
            const constraints = [];
            if (schema.minimum !== undefined || schema.exclusiveMinimum !== undefined) {
                const min = schema.exclusiveMinimum !== undefined ? `>${schema.exclusiveMinimum}` : `≥${schema.minimum}`;
                constraints.push(min);
            }
            if (schema.maximum !== undefined || schema.exclusiveMaximum !== undefined) {
                const max = schema.exclusiveMaximum !== undefined ? `<${schema.exclusiveMaximum}` : `≤${schema.maximum}`;
                constraints.push(max);
            }
            if (constraints.length > 0) {
                wrapper.appendChild(input);
                wrapper.appendChild(createConstraintHint(constraints.join(', ')));
                return wrapper;
            }
        } else {
            // String type
            if (schema.minLength !== undefined) input.minLength = schema.minLength;
            if (schema.maxLength !== undefined) input.maxLength = schema.maxLength;

            input.value = data || '';

            // Validation function for length constraints
            const validateString = (val) => {
                if (val === undefined || val === null) val = '';
                if (schema.minLength !== undefined && val.length < schema.minLength) {
                    return `Must be at least ${schema.minLength} characters`;
                }
                if (schema.maxLength !== undefined && val.length > schema.maxLength) {
                    return `Must be at most ${schema.maxLength} characters`;
                }
                return null;
            };

            input.oninput = (e) => onChange(e.target.value);

            input.onblur = (e) => {
                showError(validateString(e.target.value));
            };

            // Add constraint hint
            if (schema.minLength !== undefined || schema.maxLength !== undefined) {
                let hintText;
                if (schema.minLength !== undefined && schema.maxLength !== undefined) {
                    hintText = `${schema.minLength}-${schema.maxLength} characters`;
                } else if (schema.minLength !== undefined) {
                    hintText = `Min ${schema.minLength} characters`;
                } else {
                    hintText = `Max ${schema.maxLength} characters`;
                }
                wrapper.appendChild(input);
                wrapper.appendChild(createConstraintHint(hintText));
                return wrapper;
            }
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

    // Handle allOf - merge schemas and get default
    if (schema.allOf && Array.isArray(schema.allOf)) {
        let merged = { ...schema };
        delete merged.allOf;
        for (const sub of schema.allOf) {
            const resolved = sub.$ref ? resolveRef(sub.$ref, rootSchema) : sub;
            if (resolved) merged = mergeSchemas(merged, resolved);
        }
        return getDefaultValue(merged, rootSchema);
    }

    // Handle if/then/else - use 'then' branch for default since condition is usually met initially
    if (schema.if) {
        const baseSchema = { ...schema };
        delete baseSchema.if;
        delete baseSchema.then;
        delete baseSchema.else;
        const merged = mergeSchemas(baseSchema, schema.then || {});
        return getDefaultValue(merged, rootSchema);
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
    if (schema.anyOf && schema.anyOf.length > 0) return getDefaultValue(schema.anyOf[0], rootSchema);

    return "";
}

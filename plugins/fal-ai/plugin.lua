-- FAL.AI Image Processing Plugin for mahresources
-- AI-powered image processing using fal.ai

plugin = {
    name = "fal-ai",
    version = "1.0.0",
    description = "AI-powered image processing using fal.ai - colorize, upscale, restore, AI edit, and vectorize.",
    settings = {
        { name = "api_key", type = "password", label = "FAL.AI API Key" },
    }
}

-- FAL.AI endpoints
local FAL_ENDPOINTS = {
    colorize = "fal-ai/ddcolor",
    clarity = "fal-ai/clarity-upscaler",
    esrgan = "fal-ai/esrgan",
    creative = "fal-ai/creative-upscaler",
    restore = "fal-ai/image-apps-v2/photo-restoration",
    flux2 = "fal-ai/flux-2/turbo/edit",
    flux2pro = "fal-ai/flux-2-pro/edit",
    flux1dev = "fal-ai/flux/dev/image-to-image",
    nanobanana2 = "fal-ai/nano-banana-2/edit",
    vectorize = "fal-ai/recraft/vectorize",
    nanobanana2_generate = "fal-ai/nano-banana-2",
    imagen4 = "fal-ai/imagen4/preview",
    imagen4_fast = "fal-ai/imagen4/preview/fast",
    imagen4_ultra = "fal-ai/imagen4/preview/ultra",
}

-- Supported raster image content types
local SUPPORTED_TYPES = {
    ["image/png"] = true,
    ["image/jpeg"] = true,
    ["image/webp"] = true,
    ["image/gif"] = true,
    ["image/tiff"] = true,
    ["image/bmp"] = true,
}

-- Build API request payload based on action and options
local function build_request(action_id, data_uri, params)
    if action_id == "colorize" then
        return FAL_ENDPOINTS.colorize, {image_url = data_uri}

    elseif action_id == "upscale" then
        local model = params.model or "clarity"
        if model == "esrgan" then
            return FAL_ENDPOINTS.esrgan, {
                image_url = data_uri,
                scale = 4,
                model = "RealESRGAN_x4plus",
            }
        elseif model == "creative" then
            return FAL_ENDPOINTS.creative, {image_url = data_uri}
        else
            return FAL_ENDPOINTS.clarity, {
                image_url = data_uri,
                prompt = "masterpiece, best quality, highres",
                negative_prompt = "(worst quality, low quality, normal quality:2)",
                enable_safety_checker = false,
            }
        end

    elseif action_id == "restore" then
        local fix_colors = true
        local remove_scratches = true
        if params.fix_colors ~= nil then fix_colors = (params.fix_colors == "true" or params.fix_colors == true) end
        if params.remove_scratches ~= nil then remove_scratches = (params.remove_scratches == "true" or params.remove_scratches == true) end
        return FAL_ENDPOINTS.restore, {
            image_url = data_uri,
            enhance_resolution = true,
            fix_colors = fix_colors,
            remove_scratches = remove_scratches,
            enable_safety_checker = false,
        }

    elseif action_id == "edit" then
        local model = params.model or "flux2"
        local prompt = params.prompt or ""
        if model == "flux1dev" then
            local strength = tonumber(params.strength) or 0.75
            return FAL_ENDPOINTS.flux1dev, {
                image_url = data_uri,
                prompt = prompt,
                strength = strength,
                num_inference_steps = 40,
                guidance_scale = 3.5,
                safety_tolerance = 5,
            }
        elseif model == "nanobanana2" then
            return FAL_ENDPOINTS.nanobanana2, {
                image_urls = {data_uri},
                prompt = prompt,
                safety_tolerance = 6,
            }
        else
            local endpoint = FAL_ENDPOINTS[model] or FAL_ENDPOINTS.flux2
            return endpoint, {
                image_urls = {data_uri},
                prompt = prompt,
                guidance_scale = 2.5,
                safety_tolerance = 5,
            }
        end

    elseif action_id == "vectorize" then
        return FAL_ENDPOINTS.vectorize, {image_url = data_uri}

    else
        return nil, nil
    end
end

-- Extract result image URL from API response
local function get_result_url(result)
    if result.image and result.image.url then
        return result.image.url
    end
    if result.images and type(result.images) == "table" then
        if result.images[1] and result.images[1].url then
            return result.images[1].url
        end
    end
    return nil
end

-- Generate output resource name
local function generate_name(original, action_id)
    local name = original:match("^(.+)%.[^%.]+$") or original
    local ext = original:match("%.([^%.]+)$") or "png"
    if action_id == "vectorize" then
        ext = "svg"
    end
    return name .. "_" .. action_id .. "." .. ext
end

-- Call fal.ai API and create a new resource from the result
local function process_image(resource_id, action_id, params, api_key, job_id)
    -- Get resource data
    local base64_data, mime_type = mah.db.get_resource_data(resource_id)
    if not base64_data then
        error("Failed to read resource file data")
    end

    -- Validate supported format
    if not SUPPORTED_TYPES[mime_type] then
        error("Unsupported image format: " .. mime_type .. ". Only raster images (PNG, JPEG, WebP) are supported.")
    end

    if job_id then
        mah.job_progress(job_id, 10, "Preparing image...")
    end

    -- Build data URI
    local data_uri = "data:" .. mime_type .. ";base64," .. base64_data

    -- Build API request
    local endpoint, payload = build_request(action_id, data_uri, params)
    if not endpoint then
        error("Unknown action: " .. action_id)
    end

    if job_id then
        mah.job_progress(job_id, 20, "Calling fal.ai API...")
    end

    -- Call fal.ai API (sync — needed because async callbacks can't fire during action execution)
    local resp = mah.http.post_sync(
        "https://fal.run/" .. endpoint,
        mah.json.encode(payload),
        {
            headers = {
                Authorization = "Key " .. api_key,
                ["Content-Type"] = "application/json",
            },
            timeout = 120,
        }
    )

    if resp.error then
        error("HTTP request failed: " .. resp.error)
    end
    if resp.status_code ~= 200 then
        error("API error (status " .. tostring(resp.status_code) .. "): " .. (resp.body or ""))
    end

    if job_id then
        mah.job_progress(job_id, 70, "Processing result...")
    end

    -- Parse response
    local result = mah.json.decode(resp.body)
    if not result then
        error("Failed to parse API response")
    end
    if result.msg and result.msg ~= "" then
        error(result.msg)
    end

    -- Get result URL
    local result_url = get_result_url(result)
    if not result_url then
        error("No image URL in API response")
    end

    if job_id then
        mah.job_progress(job_id, 85, "Saving result...")
    end

    -- Get original resource info for naming
    local resource_info = mah.db.get_resource(resource_id)
    local original_name = (resource_info and resource_info.name) or ("resource_" .. tostring(resource_id) .. ".png")
    local new_name = generate_name(original_name, action_id)

    -- Create new resource from the result URL
    local new_resource, create_err = mah.db.create_resource_from_url(result_url, {
        name = new_name,
        description = "Generated by fal.ai " .. action_id .. " from resource #" .. tostring(resource_id),
    })

    if not new_resource then
        error("Failed to save result: " .. (create_err or "unknown error"))
    end

    return new_resource
end

-- Common action handler for image processing actions
local function make_handler(action_id)
    return function(ctx)
        local api_key = mah.get_setting("api_key")
        if not api_key or api_key == "" then
            return {success = false, message = "FAL.AI API key not configured. Set it in plugin settings."}
        end

        local resource_id = ctx.entity_id
        local params = ctx.params or {}
        local job_id = ctx.job_id

        local ok, result = pcall(process_image, resource_id, action_id, params, api_key, job_id)

        if ok then
            if job_id then
                mah.job_complete(job_id, "Done! Created resource #" .. tostring(result.id))
            end
            return {
                success = true,
                message = "Created resource #" .. tostring(result.id),
                redirect = "/v1/resource?id=" .. tostring(result.id),
            }
        else
            local err_msg = tostring(result)
            if job_id then
                mah.job_fail(job_id, err_msg)
            end
            return {success = false, message = err_msg}
        end
    end
end

-- Image content types for filters (detail view filtering)
local IMAGE_CONTENT_TYPES = {
    "image/jpeg", "image/png", "image/webp", "image/gif",
    "image/tiff", "image/bmp", "image/svg+xml",
}

function init()
    -- Colorize: detail + card
    mah.action({
        id = "colorize",
        label = "Colorize",
        description = "Colorize a black and white image using AI",
        icon = "wand",
        entity = "resource",
        placement = {"detail", "card"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        handler = make_handler("colorize"),
    })

    -- Upscale: detail + card
    mah.action({
        id = "upscale",
        label = "Upscale",
        description = "Upscale image resolution using AI",
        icon = "arrows-expand",
        entity = "resource",
        placement = {"detail", "card"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        params = {
            {name = "model", type = "select", label = "Model", default = "clarity",
                options = {"clarity", "esrgan", "creative"}},
        },
        handler = make_handler("upscale"),
    })

    -- Restore: detail + card
    mah.action({
        id = "restore",
        label = "Restore",
        description = "Restore and enhance old or damaged photos",
        icon = "refresh",
        entity = "resource",
        placement = {"detail", "card"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        params = {
            {name = "fix_colors", type = "boolean", label = "Fix Colors", default = true},
            {name = "remove_scratches", type = "boolean", label = "Remove Scratches", default = true},
        },
        handler = make_handler("restore"),
    })

    -- AI Edit: detail only
    mah.action({
        id = "edit",
        label = "AI Edit",
        description = "Edit image using AI with a text prompt",
        icon = "pencil",
        entity = "resource",
        placement = {"detail"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        params = {
            {name = "prompt", type = "text", label = "Edit Prompt", required = true},
            {name = "model", type = "select", label = "Model", default = "flux2",
                options = {"flux2", "flux2pro", "nanobanana2", "flux1dev"}},
            {name = "strength", type = "number", label = "Strength", default = 0.75,
                min = 0.1, max = 1.0, step = 0.05},
        },
        handler = make_handler("edit"),
    })

    -- Vectorize: detail + card
    mah.action({
        id = "vectorize",
        label = "Vectorize",
        description = "Convert raster image to SVG vector format",
        icon = "sparkles",
        entity = "resource",
        placement = {"detail", "card"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        handler = make_handler("vectorize"),
    })

    -- Generate Image page
    mah.page("generate", function(ctx)
        local api_key = mah.get_setting("api_key")
        if not api_key or api_key == "" then
            return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                .. '<p class="text-red-600">FAL.AI API key not configured. Please set it in plugin settings.</p></div>'
        end

        -- Check if this is a form submission
        local params = ctx.params or {}
        local prompt = params.prompt

        if prompt and prompt ~= "" then
            -- Process generation
            local model = params.model or "nanobanana2"
            local endpoint = FAL_ENDPOINTS[model] or FAL_ENDPOINTS.nanobanana2_generate

            -- Map model to endpoint
            if model == "nanobanana2" then
                endpoint = FAL_ENDPOINTS.nanobanana2_generate
            end

            local payload = {
                prompt = prompt,
                aspect_ratio = params.aspect_ratio or "1:1",
                output_format = "jpeg",
                safety_tolerance = 6,
            }

            -- Resolution handling per model
            if model == "nanobanana2" then
                payload.resolution = params.resolution or "1K"
            elseif model ~= "imagen4_fast" then
                local res = params.resolution or "1K"
                if res == "1K" or res == "2K" then
                    payload.resolution = res
                else
                    payload.resolution = "1K"
                end
            end

            local resp = mah.http.post_sync(
                "https://fal.run/" .. endpoint,
                mah.json.encode(payload),
                {
                    headers = {
                        Authorization = "Key " .. api_key,
                        ["Content-Type"] = "application/json",
                    },
                    timeout = 120,
                }
            )

            if resp.error then
                return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                    .. '<p class="text-red-600">HTTP error: ' .. resp.error .. '</p></div>'
            end
            if resp.status_code ~= 200 then
                return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                    .. '<p class="text-red-600">API error (status ' .. tostring(resp.status_code) .. '): ' .. (resp.body or "") .. '</p></div>'
            end

            local result = mah.json.decode(resp.body)
            if not result then
                return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                    .. '<p class="text-red-600">Failed to parse API response</p></div>'
            end

            local result_url = get_result_url(result)
            if not result_url then
                return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                    .. '<p class="text-red-600">No image URL in API response</p></div>'
            end

            -- Save as resource
            local safe_prompt = prompt:gsub("[^%w%s_-]", ""):gsub("%s+", "_"):sub(1, 40)
            local filename = "generated_" .. safe_prompt .. ".jpg"

            local new_resource, create_err = mah.db.create_resource_from_url(result_url, {
                name = filename,
                description = "Generated by fal.ai: " .. prompt,
            })

            if not new_resource then
                return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                    .. '<p class="text-red-600">Failed to save: ' .. (create_err or "unknown") .. '</p></div>'
            end

            return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Image Generated</h2>'
                .. '<div class="mb-4"><img src="/v1/resource_file/' .. tostring(new_resource.id) .. '" '
                .. 'alt="Generated image" class="max-w-lg rounded shadow" /></div>'
                .. '<p class="mb-2">Saved as resource <a href="/v1/resource?id=' .. tostring(new_resource.id)
                .. '" class="text-blue-600 underline">#' .. tostring(new_resource.id) .. ' - ' .. filename .. '</a></p>'
                .. '<p class="text-gray-500 text-sm">Prompt: ' .. prompt .. '</p>'
                .. '<hr class="my-6" /><h3 class="text-lg font-bold mb-4">Generate Another</h3>'
                .. generate_form()
                .. '</div>'
        end

        return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
            .. generate_form()
            .. '</div>'
    end)

    mah.menu("Generate Image", "generate")
end

function generate_form()
    return '<form method="POST" class="space-y-4 max-w-lg">'
        .. '<div><label class="block font-medium mb-1" for="prompt">Prompt</label>'
        .. '<textarea id="prompt" name="prompt" required class="w-full border rounded p-2" rows="3" '
        .. 'placeholder="Describe the image you want to generate..."></textarea></div>'
        .. '<div><label class="block font-medium mb-1" for="model">Model</label>'
        .. '<select id="model" name="model" class="w-full border rounded p-2">'
        .. '<option value="nanobanana2">Nano Banana 2</option>'
        .. '<option value="imagen4">Imagen 4</option>'
        .. '<option value="imagen4_fast">Imagen 4 Fast</option>'
        .. '<option value="imagen4_ultra">Imagen 4 Ultra</option>'
        .. '</select></div>'
        .. '<div><label class="block font-medium mb-1" for="resolution">Resolution</label>'
        .. '<select id="resolution" name="resolution" class="w-full border rounded p-2">'
        .. '<option value="0.5K">0.5K</option>'
        .. '<option value="1K" selected>1K</option>'
        .. '<option value="2K">2K</option>'
        .. '<option value="4K">4K</option>'
        .. '</select></div>'
        .. '<div><label class="block font-medium mb-1" for="aspect_ratio">Aspect Ratio</label>'
        .. '<select id="aspect_ratio" name="aspect_ratio" class="w-full border rounded p-2">'
        .. '<option value="1:1" selected>1:1</option>'
        .. '<option value="16:9">16:9</option>'
        .. '<option value="9:16">9:16</option>'
        .. '<option value="4:3">4:3</option>'
        .. '<option value="3:4">3:4</option>'
        .. '<option value="3:2">3:2</option>'
        .. '<option value="2:3">2:3</option>'
        .. '</select></div>'
        .. '<button type="submit" class="bg-blue-600 text-white px-6 py-2 rounded hover:bg-blue-700">Generate</button>'
        .. '</form>'
end

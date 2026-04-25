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
    seedvr = "fal-ai/seedvr/upscale/image",
    bria_creative = "bria/upscale/creative",
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

-- HTML-escape user input to prevent XSS
local function html_escape(s)
    return s:gsub("&", "&amp;"):gsub("<", "&lt;"):gsub(">", "&gt;"):gsub('"', "&quot;"):gsub("'", "&#39;")
end

-- Supported raster image content types
local SUPPORTED_TYPES = {
    ["image/png"] = true,
    ["image/jpeg"] = true,
    ["image/webp"] = true,
    ["image/gif"] = true,
    ["image/tiff"] = true,
    ["image/bmp"] = true,
}

-- fal.ai retention controls — minimize how long fal.ai stores our I/O.
-- See https://fal.ai/docs/documentation/model-apis/media-expiration
--   X-Fal-Store-IO: 0  -> never store the JSON payload (default is 30 days)
--   X-Fal-Object-Lifecycle-Preference -> TTL for the generated output file
--     1 hour gives comfortable margin over RemoteResourceOverallTimeout (30m)
--     while keeping the output far shorter than the default (no expiration).
local function fal_request_headers(api_key)
    return {
        Authorization = "Key " .. api_key,
        ["Content-Type"] = "application/json",
        ["X-Fal-Store-IO"] = "0",
        ["X-Fal-Object-Lifecycle-Preference"] =
            '{"expiration_duration_seconds": 3600}',
    }
end

-- Build API request payload based on action and options
local function build_request(action_id, data_uri, params)
    if action_id == "colorize" then
        mah.log("info", "[fal.ai] build_request: action=colorize, endpoint=" .. FAL_ENDPOINTS.colorize)
        return FAL_ENDPOINTS.colorize, {image_url = data_uri}

    elseif action_id == "upscale" then
        local model = params.model or "clarity"
        mah.log("info", "[fal.ai] build_request: action=upscale, model=" .. model)
        if model == "esrgan" then
            mah.log("info", "[fal.ai] build_request: using ESRGAN with scale=4, model=RealESRGAN_x4plus")
            return FAL_ENDPOINTS.esrgan, {
                image_url = data_uri,
                scale = 4,
                model = "RealESRGAN_x4plus",
            }
        elseif model == "creative" then
            mah.log("info", "[fal.ai] build_request: using Creative Upscaler")
            return FAL_ENDPOINTS.creative, {image_url = data_uri}
        elseif model == "seedvr" then
            mah.log("info", "[fal.ai] build_request: using SeedVR Upscaler")
            return FAL_ENDPOINTS.seedvr, {image_url = data_uri}
        elseif model == "bria_creative" then
            mah.log("info", "[fal.ai] build_request: using Bria Creative Upscaler")
            return FAL_ENDPOINTS.bria_creative, {image_url = data_uri}
        else
            mah.log("info", "[fal.ai] build_request: using Clarity Upscaler")
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
        mah.log("info", "[fal.ai] build_request: action=restore, fix_colors=" .. tostring(fix_colors) .. ", remove_scratches=" .. tostring(remove_scratches))
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
        mah.log("info", "[fal.ai] build_request: action=edit, model=" .. model .. ", prompt=" .. prompt:sub(1, 100))
        if model == "flux1dev" then
            local strength = tonumber(params.strength) or 0.75
            mah.log("info", "[fal.ai] build_request: flux1dev strength=" .. tostring(strength) .. ", steps=40, guidance=3.5")
            return FAL_ENDPOINTS.flux1dev, {
                image_url = data_uri,
                prompt = prompt,
                strength = strength,
                num_inference_steps = 40,
                guidance_scale = 3.5,
                safety_tolerance = 5,
            }
        elseif model == "nanobanana2" then
            mah.log("info", "[fal.ai] build_request: nanobanana2 edit mode")
            return FAL_ENDPOINTS.nanobanana2, {
                image_urls = {data_uri},
                prompt = prompt,
                safety_tolerance = 6,
            }
        else
            local endpoint = FAL_ENDPOINTS[model] or FAL_ENDPOINTS.flux2
            mah.log("info", "[fal.ai] build_request: using endpoint=" .. endpoint .. ", guidance=2.5")
            return endpoint, {
                image_urls = {data_uri},
                prompt = prompt,
                guidance_scale = 2.5,
                safety_tolerance = 5,
            }
        end

    elseif action_id == "vectorize" then
        mah.log("info", "[fal.ai] build_request: action=vectorize, endpoint=" .. FAL_ENDPOINTS.vectorize)
        return FAL_ENDPOINTS.vectorize, {image_url = data_uri}

    else
        mah.log("error", "[fal.ai] build_request: unknown action_id=" .. tostring(action_id))
        return nil, nil
    end
end

-- Extract result image URL from API response
local function get_result_url(result)
    if result.image and result.image.url then
        mah.log("info", "[fal.ai] get_result_url: found single image URL: " .. result.image.url:sub(1, 120))
        return result.image.url
    end
    if result.images and type(result.images) == "table" then
        mah.log("info", "[fal.ai] get_result_url: found images array with " .. #result.images .. " entries")
        if result.images[1] and result.images[1].url then
            mah.log("info", "[fal.ai] get_result_url: using first image URL: " .. result.images[1].url:sub(1, 120))
            return result.images[1].url
        end
    end
    mah.log("error", "[fal.ai] get_result_url: no image URL found in response")
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
    mah.log("info", "[fal.ai] process_image: resource_id=" .. tostring(resource_id) .. ", action=" .. action_id)

    -- Get resource data
    mah.log("info", "[fal.ai] process_image: loading resource data for resource #" .. tostring(resource_id))
    local base64_data, mime_type = mah.db.get_resource_data(resource_id)
    if not base64_data then
        mah.log("error", "[fal.ai] process_image: failed to read resource file data for resource #" .. tostring(resource_id))
        error("Failed to read resource file data")
    end
    mah.log("info", "[fal.ai] process_image: resource data loaded, mime_type=" .. tostring(mime_type) .. ", base64_length=" .. #base64_data)

    -- Validate supported format
    if not SUPPORTED_TYPES[mime_type] then
        mah.log("error", "[fal.ai] process_image: unsupported format " .. tostring(mime_type) .. " for resource #" .. tostring(resource_id))
        error("Unsupported image format: " .. mime_type .. ". Only raster images (PNG, JPEG, WebP) are supported.")
    end

    if job_id then
        mah.job_progress(job_id, 10, "Preparing image...")
    end

    -- Build data URI
    local data_uri = "data:" .. mime_type .. ";base64," .. base64_data
    mah.log("info", "[fal.ai] process_image: data URI built, total size=" .. #data_uri .. " bytes")

    -- Build API request
    local endpoint, payload = build_request(action_id, data_uri, params)
    if not endpoint then
        mah.log("error", "[fal.ai] process_image: unknown action " .. action_id)
        error("Unknown action: " .. action_id)
    end

    if job_id then
        mah.job_progress(job_id, 20, "Calling fal.ai API...")
    end

    -- Call fal.ai API (sync — needed because async callbacks can't fire during action execution)
    local api_url = "https://fal.run/" .. endpoint
    mah.log("info", "[fal.ai] process_image: POST " .. api_url)
    local payload_json = mah.json.encode(payload)
    mah.log("info", "[fal.ai] process_image: payload size=" .. #payload_json .. " bytes, timeout=120s")

    local resp = mah.http.post_sync(
        api_url,
        payload_json,
        {
            headers = fal_request_headers(api_key),
            timeout = 120,
        }
    )

    if resp.error then
        mah.log("error", "[fal.ai] process_image: HTTP request failed: " .. resp.error)
        error("HTTP request failed: " .. resp.error)
    end

    mah.log("info", "[fal.ai] process_image: response status=" .. tostring(resp.status_code) .. ", body_length=" .. tostring(resp.body and #resp.body or 0))

    if resp.status_code ~= 200 then
        mah.log("error", "[fal.ai] process_image: API error: status=" .. tostring(resp.status_code) .. ", body=" .. (resp.body or ""):sub(1, 500))
        error("API error (status " .. tostring(resp.status_code) .. "): " .. (resp.body or ""):sub(1, 500))
    end

    if job_id then
        mah.job_progress(job_id, 70, "Processing result...")
    end

    -- Parse response
    local result = mah.json.decode(resp.body)
    if not result then
        mah.log("error", "[fal.ai] process_image: failed to parse API response")
        error("Failed to parse API response")
    end
    if result.msg and result.msg ~= "" then
        mah.log("error", "[fal.ai] process_image: API returned message: " .. result.msg)
        error(result.msg)
    end

    -- Get result URL
    local result_url = get_result_url(result)
    if not result_url then
        mah.log("error", "[fal.ai] process_image: no image URL in API response")
        error("No image URL in API response")
    end

    if job_id then
        mah.job_progress(job_id, 85, "Saving result...")
    end

    -- Vectorize creates a new resource (different format: SVG).
    -- All other actions add a version to the original resource.
    if action_id == "vectorize" then
        local resource_info = mah.db.get_resource(resource_id)
        local original_name = (resource_info and resource_info.name) or ("resource_" .. tostring(resource_id) .. ".png")
        local new_name = generate_name(original_name, action_id)
        mah.log("info", "[fal.ai] process_image: saving vectorize result as new resource " .. new_name)

        -- Copy relations from the source resource
        local create_opts = {
            name = new_name,
            description = (resource_info and resource_info.description) or "",
        }
        if resource_info then
            if resource_info.owner_id then
                create_opts.owner_id = resource_info.owner_id
            end
            if resource_info.meta and resource_info.meta ~= "" and resource_info.meta ~= "{}" then
                create_opts.meta = resource_info.meta
            end
            if resource_info.tags then
                local tag_ids = {}
                for _, t in ipairs(resource_info.tags) do
                    tag_ids[#tag_ids + 1] = t.id
                end
                if #tag_ids > 0 then
                    create_opts.tags = tag_ids
                end
            end
            if resource_info.groups then
                local group_ids = {}
                for _, g in ipairs(resource_info.groups) do
                    group_ids[#group_ids + 1] = g.id
                end
                if #group_ids > 0 then
                    create_opts.groups = group_ids
                end
            end
        end

        local new_resource, create_err = mah.db.create_resource_from_url(result_url, create_opts)

        if not new_resource then
            mah.log("error", "[fal.ai] process_image: failed to save result: " .. (create_err or "unknown error"))
            error("Failed to save result: " .. (create_err or "unknown error"))
        end

        -- Add new resource to the same notes as the source
        if resource_info and resource_info.notes then
            for _, n in ipairs(resource_info.notes) do
                mah.db.add_resources_to_note(n.id, {new_resource.id})
            end
        end

        mah.log("info", "[fal.ai] process_image: created resource #" .. tostring(new_resource.id) .. " from " .. action_id .. " of resource #" .. tostring(resource_id))
        return {id = new_resource.id, resource_id = new_resource.id, is_new_resource = true}
    end

    -- Add as new version of the original resource
    local comment = "fal.ai " .. action_id
    if action_id == "edit" and (params.prompt or "") ~= "" then
        comment = comment .. ": " .. params.prompt:sub(1, 100)
    end
    mah.log("info", "[fal.ai] process_image: adding version to resource #" .. tostring(resource_id) .. " (" .. comment .. ")")

    local version, ver_err = mah.db.add_resource_version_from_url(resource_id, result_url, comment)

    if not version then
        mah.log("error", "[fal.ai] process_image: failed to add version: " .. (ver_err or "unknown error"))
        error("Failed to add version: " .. (ver_err or "unknown error"))
    end

    mah.log("info", "[fal.ai] process_image: added version #" .. tostring(version.version_number) .. " to resource #" .. tostring(resource_id))
    return {id = version.id, resource_id = resource_id, version_number = version.version_number}
end

-- Common action handler for image processing actions
local function make_handler(action_id)
    return function(ctx)
        mah.log("info", "[fal.ai] handler invoked: action=" .. action_id .. ", entity_id=" .. tostring(ctx.entity_id) .. ", job_id=" .. tostring(ctx.job_id))

        local api_key = mah.get_setting("api_key")
        if not api_key or api_key == "" then
            mah.log("error", "[fal.ai] handler: API key not configured")
            return {success = false, message = "FAL.AI API key not configured. Set it in plugin settings."}
        end
        mah.log("info", "[fal.ai] handler: API key loaded (length=" .. #api_key .. ")")

        local resource_id = ctx.entity_id
        local params = ctx.params or {}
        local job_id = ctx.job_id

        -- Log params
        local param_parts = {}
        for k, v in pairs(params) do
            param_parts[#param_parts + 1] = k .. "=" .. tostring(v)
        end
        if #param_parts > 0 then
            mah.log("info", "[fal.ai] handler: params: " .. table.concat(param_parts, ", "))
        end

        local ok, result = pcall(process_image, resource_id, action_id, params, api_key, job_id)

        if ok then
            local resource_id = result.resource_id or ctx.entity_id
            if result.is_new_resource then
                mah.log("info", "[fal.ai] handler: " .. action_id .. " completed, created resource #" .. tostring(result.id))
                if job_id then
                    mah.job_complete(job_id, {message = "Done! Created resource #" .. tostring(result.id)})
                end
                return {
                    success = true,
                    message = "Created resource #" .. tostring(result.id),
                    redirect = "/v1/resource?id=" .. tostring(result.id),
                }
            else
                mah.log("info", "[fal.ai] handler: " .. action_id .. " completed, added version to resource #" .. tostring(resource_id))
                if job_id then
                    mah.job_complete(job_id, {message = "Done! Added version to resource #" .. tostring(resource_id)})
                end
                return {
                    success = true,
                    message = "Added version to resource #" .. tostring(resource_id),
                    redirect = "/v1/resource?id=" .. tostring(resource_id),
                }
            end
        else
            local err_msg = tostring(result)
            mah.log("error", "[fal.ai] handler: " .. action_id .. " FAILED for resource #" .. tostring(resource_id) .. ": " .. err_msg)
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

local function generate_form()
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

function init()
    mah.log("info", "[fal.ai] init: registering actions and pages")

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
                options = {"clarity", "esrgan", "creative", "seedvr", "bria_creative"}},
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

    mah.log("info", "[fal.ai] init: registered 5 actions (colorize, upscale, restore, edit, vectorize)")

    -- Generate Image page
    mah.page("generate", function(ctx)
        mah.log("info", "[fal.ai] generate page: accessed")

        local api_key = mah.get_setting("api_key")
        if not api_key or api_key == "" then
            mah.log("error", "[fal.ai] generate page: API key not configured")
            return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                .. '<p class="text-red-600">FAL.AI API key not configured. Please set it in plugin settings.</p></div>'
        end

        -- Check if this is a form submission
        local params = ctx.params or {}
        local prompt = params.prompt

        if prompt and prompt ~= "" then
            local model = params.model or "nanobanana2"
            local aspect_ratio = params.aspect_ratio or "1:1"
            local resolution = params.resolution or "1K"

            mah.log("info", "[fal.ai] generate page: starting async job, model=" .. model .. ", prompt=" .. prompt:sub(1, 100))

            -- Start async job and return immediately
            local job_id = mah.start_job("Generate: " .. prompt:sub(1, 40), function(jid)
                mah.job_progress(jid, 10, "Preparing request...")

                local endpoint = FAL_ENDPOINTS[model] or FAL_ENDPOINTS.nanobanana2_generate
                if model == "nanobanana2" then
                    endpoint = FAL_ENDPOINTS.nanobanana2_generate
                end

                local payload = {
                    prompt = prompt,
                    aspect_ratio = aspect_ratio,
                    output_format = "jpeg",
                    safety_tolerance = 6,
                }

                if model == "nanobanana2" then
                    payload.resolution = resolution
                elseif model ~= "imagen4_fast" then
                    if resolution == "1K" or resolution == "2K" then
                        payload.resolution = resolution
                    else
                        payload.resolution = "1K"
                    end
                end

                mah.job_progress(jid, 20, "Calling fal.ai API...")

                local api_url = "https://fal.run/" .. endpoint
                mah.log("info", "[fal.ai] generate job: POST " .. api_url)
                local payload_json = mah.json.encode(payload)

                local resp = mah.http.post_sync(
                    api_url,
                    payload_json,
                    {
                        headers = fal_request_headers(api_key),
                        timeout = 120,
                    }
                )

                if resp.error then
                    mah.job_fail(jid, "HTTP error: " .. resp.error)
                    return
                end

                if resp.status_code ~= 200 then
                    mah.job_fail(jid, "API error (status " .. tostring(resp.status_code) .. "): " .. (resp.body or ""):sub(1, 200))
                    return
                end

                mah.job_progress(jid, 70, "Processing result...")

                local result = mah.json.decode(resp.body)
                if not result then
                    mah.job_fail(jid, "Failed to parse API response")
                    return
                end

                local result_url = get_result_url(result)
                if not result_url then
                    mah.job_fail(jid, "No image URL in API response")
                    return
                end

                mah.job_progress(jid, 85, "Saving result...")

                local safe_prompt = prompt:gsub("[^%w%s_-]", ""):gsub("%s+", "_"):sub(1, 40)
                local filename = "generated_" .. safe_prompt .. ".jpg"

                local new_resource, create_err = mah.db.create_resource_from_url(result_url, {
                    name = filename,
                    description = "Generated by fal.ai: " .. prompt,
                })

                if not new_resource then
                    mah.job_fail(jid, "Failed to save: " .. (create_err or "unknown"))
                    return
                end

                mah.log("info", "[fal.ai] generate job: created resource #" .. tostring(new_resource.id))
                mah.job_complete(jid, {
                    message = "Created resource #" .. tostring(new_resource.id),
                    redirect = "/v1/resource?id=" .. tostring(new_resource.id),
                })
            end)

            return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
                .. '<p class="text-green-600 mb-4">Generation started! Track progress in the Jobs panel '
                .. '(<kbd>Ctrl+Shift+D</kbd>).</p>'
                .. '<p class="text-gray-500 text-sm mb-6">Prompt: ' .. html_escape(prompt) .. '</p>'
                .. '<hr class="my-6" /><h3 class="text-lg font-bold mb-4">Generate Another</h3>'
                .. generate_form()
                .. '</div>'
        end

        mah.log("info", "[fal.ai] generate page: displaying form")
        return '<div class="p-8"><h2 class="text-xl font-bold mb-4">Generate Image</h2>'
            .. generate_form()
            .. '</div>'
    end)

    mah.menu("Generate Image", "generate")

    -- Documentation
    mah.doc({
        name = "getting-started",
        label = "Getting Started",
        description = "Set up the fal.ai plugin for AI-powered image processing.",
        examples = {
            { title = "Configure API key", code = "Go to Plugin Settings and enter your FAL.AI API key." },
        },
        notes = {
            "Requires a fal.ai API key (get one at fal.ai).",
            "Supported input formats: PNG, JPEG, WebP, GIF, TIFF, BMP.",
            "All image actions (except Vectorize) add a new version to the original resource.",
            "Vectorize creates a new SVG resource.",
        },
    })

    mah.doc({
        name = "colorize",
        label = "Colorize",
        description = "Automatically colorize black and white images using the DDColor AI model.",
        category = "Action",
        notes = {
            "Best results with grayscale photographs.",
            "Result is added as a new version of the original resource.",
            "Available from both detail view and card view.",
        },
    })

    mah.doc({
        name = "upscale",
        label = "Upscale",
        description = "Increase image resolution using AI upscaling models.",
        category = "Action",
        attrs = {
            { name = "model", type = "select", default = "clarity", description = "Upscaling model to use" },
        },
        examples = {
            { title = "Clarity Upscaler (default)", code = "Uses prompt-guided upscaling with quality-focused defaults.", notes = "Model: fal-ai/clarity-upscaler" },
            { title = "ESRGAN", code = "4x upscaling with RealESRGAN_x4plus model.", notes = "Model: fal-ai/esrgan" },
            { title = "Creative Upscaler", code = "AI-enhanced upscaling with creative interpretation.", notes = "Model: fal-ai/creative-upscaler" },
            { title = "SeedVR", code = "High-quality upscaling with SeedVR model.", notes = "Model: fal-ai/seedvr/upscale/image" },
            { title = "Bria Creative", code = "Creative upscaling by Bria AI.", notes = "Model: bria/upscale/creative" },
        },
        notes = {
            "Result is added as a new version of the original resource.",
            "Available from both detail view and card view.",
        },
    })

    mah.doc({
        name = "restore",
        label = "Restore",
        description = "Restore and enhance old or damaged photographs using AI.",
        category = "Action",
        attrs = {
            { name = "fix_colors", type = "boolean", default = "true", description = "Fix color issues in the photo" },
            { name = "remove_scratches", type = "boolean", default = "true", description = "Remove scratches and damage" },
        },
        notes = {
            "Also enhances resolution as part of the restoration process.",
            "Result is added as a new version of the original resource.",
            "Available from both detail view and card view.",
        },
    })

    mah.doc({
        name = "edit",
        label = "AI Edit",
        description = "Edit an image using a text prompt and AI models.",
        category = "Action",
        attrs = {
            { name = "prompt", type = "text", required = true, description = "Text description of the desired edit" },
            { name = "model", type = "select", default = "flux2", description = "AI model: flux2, flux2pro, nanobanana2, flux1dev" },
            { name = "strength", type = "number", default = "0.75", description = "Edit strength (0.1-1.0, only used by flux1dev)" },
        },
        examples = {
            { title = "Change background", code = 'Prompt: "change the background to a sunset beach"' },
            { title = "Style transfer", code = 'Prompt: "make it look like a watercolor painting"' },
        },
        notes = {
            "Result is added as a new version of the original resource.",
            "Available from detail view only.",
            "Flux 2 and Flux 2 Pro accept multiple input images.",
            "Flux 1 Dev supports a strength parameter for controlling edit intensity.",
        },
    })

    mah.doc({
        name = "vectorize",
        label = "Vectorize",
        description = "Convert a raster image to SVG vector format using AI.",
        category = "Action",
        notes = {
            "Creates a new SVG resource (does not add a version).",
            "Available from both detail view and card view.",
            "Uses the Recraft vectorize model.",
        },
    })

    mah.doc({
        name = "generate",
        label = "Generate Image",
        description = "Generate images from text prompts using AI models.",
        category = "Page",
        attrs = {
            { name = "prompt", type = "text", required = true, description = "Text description of the image to generate" },
            { name = "model", type = "select", default = "nanobanana2", description = "Model: nanobanana2, imagen4, imagen4_fast, imagen4_ultra" },
            { name = "resolution", type = "select", default = "1K", description = "Output resolution: 0.5K, 1K, 2K, 4K" },
            { name = "aspect_ratio", type = "select", default = "1:1", description = "Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4, 3:2, 2:3" },
        },
        examples = {
            { title = "Basic generation", code = 'Prompt: "a serene mountain landscape at golden hour"' },
        },
        notes = {
            "Accessible via the Generate Image menu item.",
            "Uses asynchronous job processing; track progress with Ctrl+Shift+D.",
            "Generated images are saved as new resources.",
        },
    })

    mah.log("info", "[fal.ai] init: plugin fully initialized")
end

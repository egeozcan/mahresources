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
    topaz = "fal-ai/topaz/upscale/image",
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

-- Apply a string param to payload only when present and non-empty.
local function apply_str(payload, key, val)
    if val ~= nil and val ~= "" then payload[key] = val end
end

-- Apply a numeric param to payload only when it parses as a number.
local function apply_num(payload, key, val)
    local n = tonumber(val)
    if n then payload[key] = n end
end

-- Apply a boolean param to payload, accepting both bools and "true"/"false" strings.
local function apply_bool(payload, key, val)
    if val ~= nil then payload[key] = (val == "true" or val == true) end
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
            -- ESRGAN: scale and model variant (current default scale=4 preserves prior behavior)
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.esrgan_model)
            apply_num(payload, "scale", params.esrgan_scale)
            apply_bool(payload, "face", params.esrgan_face)
            apply_str(payload, "output_format", params.esrgan_output_format)
            mah.log("info", "[fal.ai] build_request: using ESRGAN, scale=" .. tostring(payload.scale) .. ", model=" .. tostring(payload.model))
            return FAL_ENDPOINTS.esrgan, payload

        elseif model == "creative" then
            local payload = {image_url = data_uri}
            apply_str(payload, "prompt", params.creative_prompt)
            apply_num(payload, "scale", params.creative_scale)
            apply_num(payload, "creativity", params.creative_creativity)
            apply_num(payload, "detail", params.creative_detail)
            apply_num(payload, "shape_preservation", params.creative_shape_preservation)
            mah.log("info", "[fal.ai] build_request: using Creative Upscaler, scale=" .. tostring(payload.scale))
            return FAL_ENDPOINTS.creative, payload

        elseif model == "seedvr" then
            local payload = {image_url = data_uri}
            apply_str(payload, "upscale_mode", params.seedvr_upscale_mode)
            apply_num(payload, "upscale_factor", params.seedvr_upscale_factor)
            apply_str(payload, "target_resolution", params.seedvr_target_resolution)
            apply_num(payload, "noise_scale", params.seedvr_noise_scale)
            apply_str(payload, "output_format", params.seedvr_output_format)
            mah.log("info", "[fal.ai] build_request: using SeedVR Upscaler, mode=" .. tostring(payload.upscale_mode))
            return FAL_ENDPOINTS.seedvr, payload

        elseif model == "bria_creative" then
            local payload = {image_url = data_uri}
            apply_bool(payload, "preserve_alpha", params.bria_preserve_alpha)
            mah.log("info", "[fal.ai] build_request: using Bria Creative Upscaler")
            return FAL_ENDPOINTS.bria_creative, payload

        elseif model == "topaz" then
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_model)
            apply_num(payload, "upscale_factor", params.topaz_upscale_factor)
            apply_str(payload, "subject_detection", params.topaz_subject_detection)
            apply_bool(payload, "face_enhancement", params.topaz_face_enhancement)
            apply_str(payload, "output_format", params.topaz_output_format)
            mah.log("info", "[fal.ai] build_request: using Topaz Upscaler, model=" .. tostring(payload.model) .. ", factor=" .. tostring(payload.upscale_factor))
            return FAL_ENDPOINTS.topaz, payload

        else
            -- Clarity (default). Safety checker stays off; existing prompt defaults preserved via param defaults.
            local payload = {image_url = data_uri, enable_safety_checker = false}
            apply_str(payload, "prompt", params.clarity_prompt)
            apply_str(payload, "negative_prompt", params.clarity_negative_prompt)
            apply_num(payload, "upscale_factor", params.clarity_upscale_factor)
            apply_num(payload, "creativity", params.clarity_creativity)
            apply_num(payload, "resemblance", params.clarity_resemblance)
            apply_num(payload, "guidance_scale", params.clarity_guidance_scale)
            apply_num(payload, "num_inference_steps", params.clarity_num_inference_steps)
            mah.log("info", "[fal.ai] build_request: using Clarity Upscaler")
            return FAL_ENDPOINTS.clarity, payload
        end

    elseif action_id == "restore" then
        local payload = {
            image_url = data_uri,
            enable_safety_checker = false,
        }
        apply_bool(payload, "fix_colors", params.fix_colors)
        apply_bool(payload, "remove_scratches", params.remove_scratches)
        apply_bool(payload, "enhance_resolution", params.enhance_resolution)
        -- image-apps-v2 expects AspectRatio as an object {ratio = "4:3"}, not a bare string.
        if params.aspect_ratio ~= nil and params.aspect_ratio ~= "" then
            payload.aspect_ratio = { ratio = params.aspect_ratio }
        end
        mah.log("info", "[fal.ai] build_request: action=restore, fix_colors=" .. tostring(payload.fix_colors) .. ", remove_scratches=" .. tostring(payload.remove_scratches) .. ", enhance_resolution=" .. tostring(payload.enhance_resolution) .. ", aspect_ratio=" .. tostring(params.aspect_ratio))
        return FAL_ENDPOINTS.restore, payload

    elseif action_id == "edit" then
        local model = params.model or "flux2"
        local prompt = params.prompt or ""
        mah.log("info", "[fal.ai] build_request: action=edit, model=" .. model .. ", prompt=" .. prompt:sub(1, 100))

        if model == "flux1dev" then
            -- flux1dev takes a single image_url, supports strength / steps / guidance / acceleration.
            -- BaseImageToInput has no safety_tolerance field — the schema-side switch is enable_safety_checker.
            local payload = {
                image_url = data_uri,
                prompt = prompt,
                strength = tonumber(params.strength) or 0.75,
                num_inference_steps = 40,
                guidance_scale = 3.5,
            }
            apply_num(payload, "num_inference_steps", params.flux1dev_num_inference_steps)
            apply_num(payload, "guidance_scale", params.flux1dev_guidance_scale)
            apply_str(payload, "acceleration", params.flux1dev_acceleration)
            mah.log("info", "[fal.ai] build_request: flux1dev strength=" .. tostring(payload.strength) .. ", steps=" .. tostring(payload.num_inference_steps) .. ", guidance=" .. tostring(payload.guidance_scale) .. ", accel=" .. tostring(payload.acceleration))
            return FAL_ENDPOINTS.flux1dev, payload

        elseif model == "nanobanana2" then
            -- NanoBanana2ImageToImageInput.safety_tolerance is a string enum '1'..'6', not a number.
            local payload = {
                image_urls = {data_uri},
                prompt = prompt,
            }
            apply_str(payload, "aspect_ratio", params.nanobanana2_aspect_ratio)
            apply_str(payload, "resolution", params.nanobanana2_resolution)
            apply_str(payload, "output_format", params.nanobanana2_output_format)
            apply_str(payload, "safety_tolerance", params.nanobanana2_safety_tolerance)
            mah.log("info", "[fal.ai] build_request: nanobanana2 edit mode, aspect=" .. tostring(payload.aspect_ratio) .. ", res=" .. tostring(payload.resolution) .. ", safety=" .. tostring(payload.safety_tolerance))
            return FAL_ENDPOINTS.nanobanana2, payload

        else
            -- flux2 turbo / flux2pro: image_urls + prompt. Schemas diverge:
            --   Flux2TurboEditImageInput  has guidance_scale (number) but NO safety_tolerance.
            --   Flux2ProImageEditInput    has safety_tolerance (string enum '1'..'5') but NO guidance_scale.
            local endpoint = FAL_ENDPOINTS[model] or FAL_ENDPOINTS.flux2
            local payload = {
                image_urls = {data_uri},
                prompt = prompt,
            }
            if model == "flux2pro" then
                apply_str(payload, "image_size", params.flux2pro_image_size)
                apply_str(payload, "output_format", params.flux2pro_output_format)
                apply_str(payload, "safety_tolerance", params.flux2pro_safety_tolerance)
            else
                payload.guidance_scale = tonumber(params.flux2_guidance_scale) or 2.5
                apply_str(payload, "image_size", params.flux2_image_size)
                apply_str(payload, "output_format", params.flux2_output_format)
            end
            mah.log("info", "[fal.ai] build_request: using endpoint=" .. endpoint .. ", image_size=" .. tostring(payload.image_size) .. ", output_format=" .. tostring(payload.output_format) .. ", safety=" .. tostring(payload.safety_tolerance) .. ", guidance=" .. tostring(payload.guidance_scale))
            return endpoint, payload
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

-- Create a new resource from a remote URL, copying name (with action suffix),
-- description, owner, meta, tags, groups, and notes from the source resource.
-- Used by vectorize (always) and by other actions when output_mode="clone".
local function create_clone_from_url(resource_id, result_url, action_id)
    local resource_info = mah.db.get_resource(resource_id)
    local original_name = (resource_info and resource_info.name) or ("resource_" .. tostring(resource_id) .. ".png")
    local new_name = generate_name(original_name, action_id)
    mah.log("info", "[fal.ai] create_clone: " .. action_id .. " -> new resource " .. new_name)

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
        mah.log("error", "[fal.ai] create_clone: failed to save: " .. (create_err or "unknown error"))
        error("Failed to save result: " .. (create_err or "unknown error"))
    end

    -- Mirror notes from the source resource
    if resource_info and resource_info.notes then
        for _, n in ipairs(resource_info.notes) do
            mah.db.add_resources_to_note(n.id, {new_resource.id})
        end
    end

    mah.log("info", "[fal.ai] create_clone: created resource #" .. tostring(new_resource.id) .. " from " .. action_id .. " of resource #" .. tostring(resource_id))
    return new_resource
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

    -- Vectorize is forced to clone (output is SVG, can't be a version of a raster).
    -- For everything else honor the user's `output_mode` choice; default is "version".
    local output_mode = params.output_mode or "version"
    if action_id == "vectorize" then output_mode = "clone" end

    if output_mode == "clone" then
        local new_resource = create_clone_from_url(resource_id, result_url, action_id)
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

-- Shared "Save Result As" toggle: version (default) vs clone (new resource).
-- Vectorize doesn't expose this since it always clones (output is SVG).
local OUTPUT_MODE_PARAM = {
    name = "output_mode", type = "select", label = "Save Result As",
    default = "version", options = {"version", "clone"},
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
        -- safety_tolerance is a string enum on every model wired to this page
        -- (Imagen4* and NanoBanana2 generate). Options "1"–"6" cover the union.
        .. '<div><label class="block font-medium mb-1" for="safety_tolerance">Safety Tolerance</label>'
        .. '<select id="safety_tolerance" name="safety_tolerance" class="w-full border rounded p-2">'
        .. '<option value="1">1 (strictest)</option>'
        .. '<option value="2">2</option>'
        .. '<option value="3">3</option>'
        .. '<option value="4">4</option>'
        .. '<option value="5">5</option>'
        .. '<option value="6" selected>6 (most permissive)</option>'
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
        params = { OUTPUT_MODE_PARAM },
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
                options = {"clarity", "esrgan", "creative", "seedvr", "bria_creative", "topaz"}},

            -- Clarity
            {name = "clarity_prompt", type = "text", label = "Prompt",
                default = "masterpiece, best quality, highres",
                show_when = {model = "clarity"}},
            {name = "clarity_negative_prompt", type = "text", label = "Negative Prompt",
                default = "(worst quality, low quality, normal quality:2)",
                show_when = {model = "clarity"}},
            {name = "clarity_upscale_factor", type = "number", label = "Upscale Factor", default = 2,
                min = 1, max = 4, step = 0.25,
                show_when = {model = "clarity"}},
            {name = "clarity_creativity", type = "number", label = "Creativity (denoise strength)",
                default = 0.35, min = 0, max = 1, step = 0.05,
                show_when = {model = "clarity"}},
            {name = "clarity_resemblance", type = "number", label = "Resemblance to Original",
                default = 0.6, min = 0, max = 1, step = 0.05,
                show_when = {model = "clarity"}},
            {name = "clarity_guidance_scale", type = "number", label = "Guidance Scale (CFG)",
                default = 4, min = 0, max = 20, step = 0.5,
                show_when = {model = "clarity"}},
            {name = "clarity_num_inference_steps", type = "number", label = "Inference Steps",
                default = 18, min = 1, max = 60, step = 1,
                show_when = {model = "clarity"}},

            -- ESRGAN
            {name = "esrgan_model", type = "select", label = "ESRGAN Model",
                default = "RealESRGAN_x4plus",
                options = {"RealESRGAN_x4plus", "RealESRGAN_x2plus",
                           "RealESRGAN_x4plus_anime_6B", "RealESRGAN_x4_v3",
                           "RealESRGAN_x4_wdn_v3", "RealESRGAN_x4_anime_v3"},
                show_when = {model = "esrgan"}},
            {name = "esrgan_scale", type = "number", label = "Scale",
                default = 4, min = 1, max = 4, step = 1,
                show_when = {model = "esrgan"}},
            {name = "esrgan_face", type = "boolean", label = "Face Mode (portraits)",
                default = false,
                show_when = {model = "esrgan"}},
            {name = "esrgan_output_format", type = "select", label = "Output Format",
                default = "png", options = {"png", "jpeg"},
                show_when = {model = "esrgan"}},

            -- Creative Upscaler
            {name = "creative_prompt", type = "text", label = "Prompt (optional, guides creativity)",
                show_when = {model = "creative"}},
            {name = "creative_scale", type = "number", label = "Scale",
                default = 2, min = 1, max = 4, step = 0.25,
                show_when = {model = "creative"}},
            {name = "creative_creativity", type = "number", label = "Creativity",
                default = 0.5, min = 0, max = 1, step = 0.05,
                show_when = {model = "creative"}},
            {name = "creative_detail", type = "number", label = "Detail",
                default = 1, min = 0, max = 2, step = 0.1,
                show_when = {model = "creative"}},
            {name = "creative_shape_preservation", type = "number", label = "Shape Preservation",
                default = 0.25, min = 0, max = 1, step = 0.05,
                show_when = {model = "creative"}},

            -- SeedVR
            {name = "seedvr_upscale_mode", type = "select", label = "Upscale Mode",
                default = "factor", options = {"factor", "target"},
                show_when = {model = "seedvr"}},
            {name = "seedvr_upscale_factor", type = "number", label = "Upscale Factor",
                default = 2, min = 1, max = 4, step = 0.25,
                show_when = {model = "seedvr", seedvr_upscale_mode = "factor"}},
            {name = "seedvr_target_resolution", type = "select", label = "Target Resolution",
                default = "1080p", options = {"720p", "1080p", "1440p", "2160p"},
                show_when = {model = "seedvr", seedvr_upscale_mode = "target"}},
            {name = "seedvr_noise_scale", type = "number", label = "Noise Scale",
                default = 0.1, min = 0, max = 1, step = 0.05,
                show_when = {model = "seedvr"}},
            {name = "seedvr_output_format", type = "select", label = "Output Format",
                default = "jpg", options = {"jpg", "png", "webp"},
                show_when = {model = "seedvr"}},

            -- Bria Creative
            {name = "bria_preserve_alpha", type = "boolean", label = "Preserve Alpha Channel",
                default = true,
                show_when = {model = "bria_creative"}},

            -- Topaz
            {name = "topaz_model", type = "select", label = "Topaz Model", default = "Standard V2",
                options = {"Standard V2", "Low Resolution V2", "CGI", "High Fidelity V2",
                           "Text Refine", "Recovery", "Redefine", "Recovery V2",
                           "Standard MAX", "Wonder"},
                show_when = {model = "topaz"}},
            {name = "topaz_upscale_factor", type = "number", label = "Upscale Factor", default = 2,
                min = 1, max = 4, step = 0.25,
                show_when = {model = "topaz"}},
            {name = "topaz_subject_detection", type = "select", label = "Subject Detection", default = "All",
                options = {"All", "Foreground", "Background"},
                show_when = {model = "topaz"}},
            {name = "topaz_face_enhancement", type = "boolean", label = "Face Enhancement", default = true,
                show_when = {model = "topaz"}},
            {name = "topaz_output_format", type = "select", label = "Output Format", default = "jpeg",
                options = {"jpeg", "png"},
                show_when = {model = "topaz"}},

            OUTPUT_MODE_PARAM,
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
            {name = "enhance_resolution", type = "boolean", label = "Enhance Resolution", default = true},
            {name = "aspect_ratio", type = "select", label = "4K Output Aspect Ratio",
                default = "4:3", options = {"1:1", "16:9", "9:16", "4:3", "3:4"}},
            OUTPUT_MODE_PARAM,
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

            -- Flux 2 Turbo
            {name = "flux2_image_size", type = "select", label = "Image Size",
                default = "square_hd",
                options = {"square_hd", "square", "portrait_4_3", "portrait_16_9",
                           "landscape_4_3", "landscape_16_9"},
                show_when = {model = "flux2"}},
            {name = "flux2_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                show_when = {model = "flux2"}},
            {name = "flux2_guidance_scale", type = "number", label = "Guidance Scale (CFG)",
                default = 2.5, min = 0, max = 20, step = 0.5,
                show_when = {model = "flux2"}},

            -- Flux 2 Pro
            {name = "flux2pro_image_size", type = "select", label = "Image Size",
                default = "auto",
                options = {"auto", "square_hd", "square", "portrait_4_3", "portrait_16_9",
                           "landscape_4_3", "landscape_16_9"},
                show_when = {model = "flux2pro"}},
            {name = "flux2pro_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                show_when = {model = "flux2pro"}},
            {name = "flux2pro_safety_tolerance", type = "select", label = "Safety Tolerance",
                default = "5", options = {"1", "2", "3", "4", "5"},
                show_when = {model = "flux2pro"}},

            -- Nano Banana 2
            {name = "nanobanana2_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "1:1",
                options = {"21:9", "16:9", "3:2", "4:3", "5:4", "1:1",
                           "4:5", "3:4", "2:3", "9:16", "4:1", "1:4", "8:1", "1:8"},
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_resolution", type = "select", label = "Resolution",
                default = "1K", options = {"0.5K", "1K", "2K", "4K"},
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_safety_tolerance", type = "select", label = "Safety Tolerance",
                default = "6", options = {"1", "2", "3", "4", "5", "6"},
                show_when = {model = "nanobanana2"}},

            -- Flux 1 Dev (image-to-image)
            {name = "strength", type = "number", label = "Strength", default = 0.75,
                min = 0.1, max = 1.0, step = 0.05,
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_num_inference_steps", type = "number", label = "Inference Steps",
                default = 40, min = 1, max = 60, step = 1,
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_guidance_scale", type = "number", label = "Guidance Scale (CFG)",
                default = 3.5, min = 0, max = 20, step = 0.5,
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_acceleration", type = "select", label = "Acceleration",
                default = "none", options = {"none", "regular", "high"},
                show_when = {model = "flux1dev"}},

            OUTPUT_MODE_PARAM,
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
            -- safety_tolerance arrives as a form string ("1".."6"); fal.ai expects a string enum here.
            local safety_tolerance = params.safety_tolerance
            if safety_tolerance == nil or safety_tolerance == "" then
                safety_tolerance = "6"
            end

            mah.log("info", "[fal.ai] generate page: starting async job, model=" .. model .. ", prompt=" .. prompt:sub(1, 100) .. ", safety=" .. safety_tolerance)

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
                    safety_tolerance = safety_tolerance,
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
            "Each action has a 'Save Result As' toggle: 'version' adds a new version to the source resource, 'clone' creates a new resource (with name, description, owner, meta, tags, groups, and notes copied from the source).",
            "Vectorize always clones — its SVG output cannot be a version of a raster source.",
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
            { name = "model", type = "select", default = "clarity", description = "Upscaling backend: clarity, esrgan, creative, seedvr, bria_creative, topaz" },
            { name = "clarity_*", type = "various", description = "Clarity controls: prompt, negative_prompt, upscale_factor, creativity, resemblance, guidance_scale, num_inference_steps (shown when model=clarity)" },
            { name = "esrgan_*", type = "various", description = "ESRGAN controls: esrgan_model variant, scale, face mode, output_format (shown when model=esrgan)" },
            { name = "creative_*", type = "various", description = "Creative Upscaler controls: prompt, scale, creativity, detail, shape_preservation (shown when model=creative)" },
            { name = "seedvr_*", type = "various", description = "SeedVR controls: upscale_mode (factor|target), upscale_factor or target_resolution, noise_scale, output_format (shown when model=seedvr)" },
            { name = "bria_preserve_alpha", type = "boolean", default = "true", description = "Preserve alpha channel (shown when model=bria_creative)" },
            { name = "topaz_*", type = "various", description = "Topaz controls: topaz_model preset, upscale_factor, subject_detection, face_enhancement, output_format (shown when model=topaz)" },
        },
        examples = {
            { title = "Clarity Upscaler (default)", code = "Uses prompt-guided upscaling with quality-focused defaults.", notes = "Model: fal-ai/clarity-upscaler" },
            { title = "ESRGAN", code = "4x upscaling with RealESRGAN_x4plus model.", notes = "Model: fal-ai/esrgan" },
            { title = "Creative Upscaler", code = "AI-enhanced upscaling with creative interpretation.", notes = "Model: fal-ai/creative-upscaler" },
            { title = "SeedVR", code = "High-quality upscaling with SeedVR model.", notes = "Model: fal-ai/seedvr/upscale/image" },
            { title = "Bria Creative", code = "Creative upscaling by Bria AI.", notes = "Model: bria/upscale/creative" },
            { title = "Topaz", code = "Detail-preserving upscaling by Topaz Labs.", notes = "Model: fal-ai/topaz/upscale/image" },
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
            { name = "enhance_resolution", type = "boolean", default = "true", description = "Improve image clarity and detail" },
            { name = "aspect_ratio", type = "select", default = "4:3", description = "4K output aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4" },
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
            { name = "flux2_image_size / flux2_output_format / flux2_guidance_scale", type = "various", description = "Flux 2 Turbo controls (shown when model=flux2). Schema has guidance_scale; safety_tolerance is not supported." },
            { name = "flux2pro_image_size / flux2pro_output_format / flux2pro_safety_tolerance", type = "various", description = "Flux 2 Pro controls (shown when model=flux2pro). safety_tolerance is a string '1'..'5'; guidance_scale is not supported." },
            { name = "nanobanana2_aspect_ratio / nanobanana2_resolution / nanobanana2_output_format / nanobanana2_safety_tolerance", type = "various", description = "Nano Banana 2 controls (shown when model=nanobanana2). safety_tolerance is a string '1'..'6'." },
            { name = "strength", type = "number", default = "0.75", description = "Edit strength 0.1-1.0 (shown when model=flux1dev)" },
            { name = "flux1dev_num_inference_steps / flux1dev_guidance_scale / flux1dev_acceleration", type = "various", description = "Flux 1 Dev controls (shown when model=flux1dev). safety_tolerance is not in the schema for this endpoint." },
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

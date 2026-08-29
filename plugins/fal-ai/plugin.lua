-- FAL.AI Image Processing Plugin for mahresources
-- AI-powered image processing using fal.ai

plugin = {
    api_version = 1,
    capabilities = { "db:read", "db:write", "http", "image", "actions", "jobs", "pages" },
    -- Two host families, because the API and the media it produces are not the
    -- same service. `*.fal.run` / `fal.ai` cover submitting a job and polling it;
    -- `fal.media` is where the finished file lives, and downloading it is a
    -- second, separately-addressed request that get_result_url() aims
    -- create_resource_from_url / add_resource_version_from_url at.
    --
    -- The wildcard is deliberate rather than lazy. fal's CDN host carries a
    -- rotating version prefix — the documented upload format is
    -- `https://v3b.fal.media/files/b/{prefix}/{filename}` while the same page's
    -- fallback chain still names `v3.fal.media` and the bare `fal.media`, and
    -- live output schemas today return `images[].url` on both v3 and v3b. An
    -- exact list would break on the next roll; the registrable domain is the
    -- stable unit fal documents as its CDN.
    -- See https://fal.ai/docs/documentation/model-apis/fal-cdn
    --
    -- Not listed: storage.googleapis.com. It appears only in fal's curated
    -- `example_outputs` documentation assets, never in a real result payload,
    -- and declaring it would open every Google Cloud Storage bucket there is.
    network = {
        "queue.fal.run", "fal.run", "*.fal.run", "fal.ai", "*.fal.ai",
        "fal.media", "*.fal.media",
    },
    name = "fal-ai",
    version = "1.2.0",
    description = "AI-powered image processing using current fal.ai models for generation, editing, color, restoration, upscaling, and vectorization.",
    settings = {
        { name = "api_key", type = "password", label = "FAL.AI API Key" },
    }
}

-- FAL.AI endpoints
local FAL_ENDPOINTS = {
    colorize = "fal-ai/ddcolor",
    clarity = "fal-ai/clarity-upscaler",
    crystal = "clarityai/crystal-upscaler",
    esrgan = "fal-ai/esrgan",
    creative = "fal-ai/creative-upscaler",
    seedvr = "fal-ai/seedvr/upscale/image",
    seedvr_seamless = "fal-ai/seedvr/upscale/image/seamless",
    bria_creative = "bria/upscale/creative",
    topaz_precision = "topaz/upscale/image/precision",
    topaz_generative = "topaz/upscale/image/generative",
    topaz_creative = "topaz/upscale/image/creative",
    topaz_transparent = "topaz/upscale/image/transparent",
    topaz_restore = "topaz/restore/image",
    topaz_denoise = "topaz/denoise/image",
    topaz_sharpen = "topaz/sharpen/image",
    topaz_adjust = "topaz/adjust/image",
    restore = "fal-ai/image-apps-v2/photo-restoration",
    codeformer = "fal-ai/codeformer",
    swin2sr = "fal-ai/swin2sr",
    nafnet_denoise = "fal-ai/nafnet/denoise",
    nafnet_deblur = "fal-ai/nafnet/deblur",
    drct = "fal-ai/drct-super-resolution",
    aura_sr = "fal-ai/aura-sr",
    post_processing = "fal-ai/post-processing",
    flux2 = "fal-ai/flux-2/turbo/edit",
    flux2pro = "fal-ai/flux-2-pro/edit",
    flux1dev = "fal-ai/flux/dev/image-to-image",
    nanobanana2 = "fal-ai/nano-banana-2/edit",
    nanobananapro = "fal-ai/nano-banana-pro/edit",
    gptimage2 = "openai/gpt-image-2/edit",
    seedream5 = "bytedance/seedream/v5/pro/edit",
    grok2 = "xai/grok-imagine-image/v2.0/edit",
    muse = "meta/muse-image/edit",
    fibo15 = "bria/fibo-edit-1.5/edit",
    nanobanana_lite = "google/nano-banana-lite/edit",
    vectorize = "fal-ai/recraft/vectorize",
    nanobanana2_generate = "fal-ai/nano-banana-2",
    nanobananapro_generate = "fal-ai/nano-banana-pro",
    gptimage2_generate = "openai/gpt-image-2",
    seedream5_generate = "bytedance/seedream/v5/pro/text-to-image",
    grok2_generate = "xai/grok-imagine-image/v2.0/text-to-image",
    muse_generate = "meta/muse-image/text-to-image",
    fibo15_generate = "bria/fibo-gen-1.5/text-to-image",
    nanobanana_lite_generate = "google/nano-banana-2-lite",
}

-- Escape a value for output: HTML metacharacters, and the shortcode brackets
-- with them, because this plugin's output goes back through the shortcode
-- processor. See mah.html_escape in plugin_system/manager.go.
local function html_escape(s)
    return s:gsub("&", "&amp;"):gsub("<", "&lt;"):gsub(">", "&gt;"):gsub('"', "&quot;"):gsub("'", "&#39;")
        :gsub("%[", "&#91;"):gsub("%]", "&#93;")
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

-- Aspect ratio enum supported by image-apps-v2/photo-restoration. The model
-- always reshapes its output to one of these — `enhance_resolution=false`
-- does NOT preserve the source ratio (verified empirically: a 512×512 source
-- came back as 4096×3072 even with enhance_resolution=false). To keep the
-- restoration from changing aspect ratio, we always send the closest enum.
local ASPECT_ENUMS = {
    {ratio = "1:1",  value = 1.0},
    {ratio = "16:9", value = 16 / 9},
    {ratio = "9:16", value = 9 / 16},
    {ratio = "4:3",  value = 4 / 3},
    {ratio = "3:4",  value = 3 / 4},
}

-- Pick the aspect_ratio enum whose decimal ratio is closest to width/height.
-- Returns nil if dimensions are missing/invalid (caller should then omit the
-- aspect_ratio param and let the model use its own default).
local function pick_aspect_ratio(width, height)
    local w = tonumber(width)
    local h = tonumber(height)
    if not w or not h or w <= 0 or h <= 0 then return nil end
    local source = w / h
    local best, best_diff = nil, math.huge
    for _, e in ipairs(ASPECT_ENUMS) do
        local d = math.abs(source - e.value)
        if d < best_diff then
            best_diff = d
            best = e.ratio
        end
    end
    return best
end

-- Look up a resource's dimensions and return the closest aspect_ratio enum.
local function auto_aspect_ratio_for(resource_id)
    local info = mah.db.get_resource(resource_id)
    if not info then return nil end
    return pick_aspect_ratio(info.width, info.height)
end

-- Build a base64 data URI for a resource. Returns (data_uri, mime_type) or
-- raises an error via `error()` if the resource can't be loaded or is in an
-- unsupported format.
local function build_data_uri(resource_id)
    -- The reason is the third return value. It matters here: "file too large
    -- (max 52428800 bytes)" and "storage not available" are very different
    -- problems for whoever is looking at the failed job, and both used to
    -- surface as the same "Failed to read resource file data".
    local base64_data, mime_type, err = mah.db.get_resource_data(resource_id)
    if not base64_data then
        error("Failed to read resource file data for #" .. tostring(resource_id)
            .. (err and (": " .. err) or ""))
    end
    if not SUPPORTED_TYPES[mime_type] then
        error("Unsupported image format: " .. mime_type .. " for resource #" .. tostring(resource_id))
    end
    return "data:" .. mime_type .. ";base64," .. base64_data, mime_type
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

-- The image_urls list every multi-image edit model takes. The extra_images picker
-- defaults to the trigger resource, so it normally already contains the source;
-- when it was cleared, fall back to the source alone.
local function edit_image_urls(data_uri, extra_data_uris)
    local urls = extra_data_uris or {}
    if #urls == 0 then
        return {data_uri}
    end
    return urls
end

-- Build API request payload based on action and options.
-- resource_id is used by actions that need to look up source-image properties
-- (e.g. restore auto-detects the aspect_ratio from the source's dimensions).
local function build_request(action_id, data_uri, params, resource_id, extra_data_uris)
    if action_id == "colorize" then
        local model = params.model or "ddcolor"
        if model == "topaz_colorize" then
            local payload = {image_url = data_uri, model = "Colorize"}
            apply_str(payload, "output_format", params.topaz_colorize_output_format)
            mah.log("info", "[fal.ai] build_request: action=colorize, endpoint=" .. FAL_ENDPOINTS.topaz_adjust)
            return FAL_ENDPOINTS.topaz_adjust, payload
        end
        mah.log("info", "[fal.ai] build_request: action=colorize, endpoint=" .. FAL_ENDPOINTS.colorize)
        return FAL_ENDPOINTS.colorize, {image_url = data_uri}

    elseif action_id == "adjust" then
        local model = params.model or "adjust_v2"
        local payload = {
            image_url = data_uri,
            model = model == "white_balance" and "White Balance" or "Adjust V2",
        }
        apply_str(payload, "output_format", params.output_format)
        mah.log("info", "[fal.ai] build_request: action=adjust, model=" .. payload.model)
        return FAL_ENDPOINTS.topaz_adjust, payload

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

        elseif model == "crystal" then
            -- Crystal Upscaler: Clarity AI's successor to clarity-upscaler, tuned
            -- for facial detail and portrait photography. Preserves aspect ratio
            -- (uniform scale_factor). `creativity` 0 is a faithful upscale; higher
            -- values let the model invent detail.
            local payload = {image_url = data_uri}
            apply_num(payload, "creativity", params.crystal_creativity)
            apply_num(payload, "scale_factor", params.crystal_scale_factor)
            apply_str(payload, "output_format", params.crystal_output_format)
            mah.log("info", "[fal.ai] build_request: using Crystal Upscaler, scale=" .. tostring(payload.scale_factor) .. ", creativity=" .. tostring(payload.creativity))
            return FAL_ENDPOINTS.crystal, payload

        elseif model == "seedvr" then
            -- The `seamless` sibling endpoint tiles without visible seams and takes
            -- the same input schema with one exception: its output_format enum
            -- spells JPEG "jpeg", where the plain endpoint — and this action's
            -- default — says "jpg". Sending "jpg" there is a 422.
            local seamless = (params.seedvr_seamless == "true" or params.seedvr_seamless == true)
            local endpoint = seamless and FAL_ENDPOINTS.seedvr_seamless or FAL_ENDPOINTS.seedvr
            local payload = {image_url = data_uri}
            apply_str(payload, "upscale_mode", params.seedvr_upscale_mode)
            apply_num(payload, "upscale_factor", params.seedvr_upscale_factor)
            apply_str(payload, "target_resolution", params.seedvr_target_resolution)
            apply_num(payload, "noise_scale", params.seedvr_noise_scale)
            apply_str(payload, "output_format", params.seedvr_output_format)
            if seamless and payload.output_format == "jpg" then
                payload.output_format = "jpeg"
            end
            mah.log("info", "[fal.ai] build_request: using SeedVR Upscaler, mode=" .. tostring(payload.upscale_mode) .. ", seamless=" .. tostring(seamless))
            return endpoint, payload

        elseif model == "bria_creative" then
            local payload = {image_url = data_uri}
            apply_bool(payload, "preserve_alpha", params.bria_preserve_alpha)
            mah.log("info", "[fal.ai] build_request: using Bria Creative Upscaler")
            return FAL_ENDPOINTS.bria_creative, payload

        elseif model == "topaz" then
            -- Topaz split its image upscaler into task-specific endpoints in
            -- August 2026. `topaz` now means the faithful Precision family.
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_model)
            apply_num(payload, "upscale_factor", params.topaz_upscale_factor)
            apply_str(payload, "subject_detection", params.topaz_subject_detection)
            apply_bool(payload, "face_enhancement", params.topaz_face_enhancement)
            apply_num(payload, "face_enhancement_strength", params.topaz_face_enhancement_strength)
            apply_num(payload, "face_enhancement_creativity", params.topaz_face_enhancement_creativity)
            apply_num(payload, "fix_compression", params.topaz_fix_compression)
            apply_num(payload, "denoise", params.topaz_denoise)
            apply_num(payload, "sharpen", params.topaz_sharpen)
            apply_num(payload, "strength", params.topaz_text_refine_strength)
            apply_str(payload, "output_format", params.topaz_output_format)
            mah.log("info", "[fal.ai] build_request: using Topaz Precision, model=" .. tostring(payload.model) .. ", factor=" .. tostring(payload.upscale_factor))
            return FAL_ENDPOINTS.topaz_precision, payload

        elseif model == "topaz_generative" then
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_generative_model)
            apply_num(payload, "upscale_factor", params.topaz_generative_upscale_factor)
            apply_str(payload, "subject_detection", params.topaz_generative_subject_detection)
            apply_bool(payload, "face_enhancement", params.topaz_generative_face_enhancement)
            apply_num(payload, "face_enhancement_strength", params.topaz_generative_face_strength)
            apply_num(payload, "face_enhancement_creativity", params.topaz_generative_face_creativity)
            apply_str(payload, "prompt", params.topaz_generative_prompt)
            apply_bool(payload, "autoprompt", params.topaz_generative_autoprompt)
            apply_num(payload, "creativity", params.topaz_generative_creativity)
            apply_num(payload, "texture", params.topaz_generative_texture)
            apply_num(payload, "detail", params.topaz_generative_detail)
            if params.topaz_generative_enhancement_strength ~= "auto" then
                apply_str(payload, "enhancement_strength", params.topaz_generative_enhancement_strength)
            end
            apply_num(payload, "denoise", params.topaz_generative_denoise)
            apply_num(payload, "sharpen", params.topaz_generative_sharpen)
            apply_str(payload, "output_format", params.topaz_generative_output_format)
            mah.log("info", "[fal.ai] build_request: using Topaz Generative, model=" .. tostring(payload.model))
            return FAL_ENDPOINTS.topaz_generative, payload

        elseif model == "topaz_creative" then
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_creative_model)
            apply_num(payload, "upscale_factor", params.topaz_creative_upscale_factor)
            apply_bool(payload, "autoprompt", params.topaz_creative_autoprompt)
            apply_bool(payload, "color_preservation", params.topaz_creative_color_preservation)
            apply_num(payload, "creativity", params.topaz_creative_creativity)
            apply_str(payload, "output_format", params.topaz_creative_output_format)
            mah.log("info", "[fal.ai] build_request: using Topaz Creative, model=" .. tostring(payload.model))
            return FAL_ENDPOINTS.topaz_creative, payload

        elseif model == "topaz_transparent" then
            mah.log("info", "[fal.ai] build_request: using Topaz Transparent (fixed 4x PNG)")
            return FAL_ENDPOINTS.topaz_transparent, {image_url = data_uri, output_format = "png"}

        elseif model == "drct" then
            -- DRCT super-resolution: degradation-aware (trained on Real-ESRGAN-style
            -- degradation pipeline), so it handles JPEG-compressed inputs better than
            -- pure-SR models. Preserves aspect ratio (uniform upscale).
            local payload = {image_url = data_uri}
            apply_num(payload, "upscale_factor", params.drct_upscale_factor)
            mah.log("info", "[fal.ai] build_request: using DRCT, factor=" .. tostring(payload.upscale_factor))
            return FAL_ENDPOINTS.drct, payload

        elseif model == "aura_sr" then
            -- Aura SR: tile-based 4x GAN. The v2 checkpoint handles JPEG-degraded
            -- inputs noticeably better than v1.
            local payload = {image_url = data_uri}
            apply_num(payload, "upscale_factor", params.aura_sr_upscale_factor)
            apply_bool(payload, "overlapping_tiles", params.aura_sr_overlapping_tiles)
            apply_str(payload, "checkpoint", params.aura_sr_checkpoint)
            mah.log("info", "[fal.ai] build_request: using Aura SR, checkpoint=" .. tostring(payload.checkpoint))
            return FAL_ENDPOINTS.aura_sr, payload

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
        local model = params.model or "photo_restoration"
        mah.log("info", "[fal.ai] build_request: action=restore, model=" .. model)

        if model == "codeformer" then
            -- Face-focused restoration. Output dims = input × upscale_factor;
            -- aspect ratio is preserved by the model (no width/height/aspect
            -- params in the schema).
            local payload = {image_url = data_uri}
            apply_num(payload, "fidelity", params.codeformer_fidelity)
            apply_num(payload, "upscale_factor", params.codeformer_upscale_factor)
            apply_bool(payload, "face_upscale", params.codeformer_face_upscale)
            apply_bool(payload, "aligned", params.codeformer_aligned)
            apply_bool(payload, "only_center_face", params.codeformer_only_center_face)
            mah.log("info", "[fal.ai] build_request: codeformer fidelity=" .. tostring(payload.fidelity) .. ", upscale_factor=" .. tostring(payload.upscale_factor) .. ", face_upscale=" .. tostring(payload.face_upscale))
            return FAL_ENDPOINTS.codeformer, payload

        elseif model == "swin2sr" then
            -- Generic super-resolution; preserves aspect ratio. The `real_sr`
            -- task is trained on real-world degraded photos and is the closest
            -- equivalent to "restore" for non-portrait sources.
            local payload = {image_url = data_uri}
            apply_str(payload, "task", params.swin2sr_task)
            mah.log("info", "[fal.ai] build_request: swin2sr task=" .. tostring(payload.task))
            return FAL_ENDPOINTS.swin2sr, payload

        elseif model == "nafnet_denoise" then
            -- NAFNet denoise: fal explicitly markets this for ISO noise and
            -- compression artifacts (JPEG blockiness, ringing, banding). Pure
            -- restoration — no upscale, preserves resolution + aspect ratio.
            local payload = {image_url = data_uri}
            apply_num(payload, "seed", params.nafnet_seed)
            mah.log("info", "[fal.ai] build_request: nafnet/denoise")
            return FAL_ENDPOINTS.nafnet_denoise, payload

        elseif model == "nafnet_deblur" then
            -- NAFNet deblur: companion to denoise — fixes camera shake / motion
            -- blur. No upscale, preserves resolution + aspect ratio.
            local payload = {image_url = data_uri}
            apply_num(payload, "seed", params.nafnet_seed)
            mah.log("info", "[fal.ai] build_request: nafnet/deblur")
            return FAL_ENDPOINTS.nafnet_deblur, payload

        elseif model == "topaz_restore" then
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_restore_model)
            apply_str(payload, "output_format", params.topaz_restore_output_format)
            mah.log("info", "[fal.ai] build_request: Topaz Restore model=" .. tostring(payload.model))
            return FAL_ENDPOINTS.topaz_restore, payload

        elseif model == "topaz_denoise" then
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_denoise_model)
            apply_str(payload, "output_format", params.topaz_denoise_output_format)
            mah.log("info", "[fal.ai] build_request: Topaz Denoise model=" .. tostring(payload.model))
            return FAL_ENDPOINTS.topaz_denoise, payload
        end

        -- photo_restoration (image-apps-v2): default. Always 4K-reshapes; we
        -- compensate by auto-picking the aspect_ratio enum closest to the
        -- source's actual dimensions when the user hasn't overridden.
        local payload = {
            image_url = data_uri,
            enable_safety_checker = false,
        }
        apply_bool(payload, "fix_colors", params.fix_colors)
        apply_bool(payload, "remove_scratches", params.remove_scratches)
        apply_bool(payload, "enhance_resolution", params.enhance_resolution)
        local ratio = params.aspect_ratio
        local resolved_via = "user"
        if ratio == nil or ratio == "" or ratio == "auto" then
            ratio = resource_id and auto_aspect_ratio_for(resource_id) or nil
            resolved_via = "auto"
        end
        if ratio ~= nil and ratio ~= "" then
            payload.aspect_ratio = { ratio = ratio }
        end
        mah.log("info", "[fal.ai] build_request: photo_restoration fix_colors=" .. tostring(payload.fix_colors) .. ", remove_scratches=" .. tostring(payload.remove_scratches) .. ", enhance_resolution=" .. tostring(payload.enhance_resolution) .. ", aspect_ratio=" .. tostring(ratio) .. " (" .. resolved_via .. ")")
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
                strength = tonumber(params.strength) or 0.95,
                num_inference_steps = 40,
                guidance_scale = 3.5,
            }
            apply_num(payload, "num_inference_steps", params.flux1dev_num_inference_steps)
            apply_num(payload, "guidance_scale", params.flux1dev_guidance_scale)
            apply_str(payload, "acceleration", params.flux1dev_acceleration)
            apply_num(payload, "seed", params.flux1dev_seed)
            apply_bool(payload, "enable_safety_checker", params.flux1dev_enable_safety_checker)
            apply_str(payload, "output_format", params.flux1dev_output_format)
            mah.log("info", "[fal.ai] build_request: flux1dev strength=" .. tostring(payload.strength) .. ", steps=" .. tostring(payload.num_inference_steps) .. ", guidance=" .. tostring(payload.guidance_scale) .. ", accel=" .. tostring(payload.acceleration))
            return FAL_ENDPOINTS.flux1dev, payload

        elseif model == "nanobanana2" then
            -- NanoBanana2ImageToImageInput.safety_tolerance is a string enum '1'..'6', not a number.
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {
                image_urls = image_urls,
                prompt = prompt,
            }
            apply_str(payload, "aspect_ratio", params.nanobanana2_aspect_ratio)
            apply_str(payload, "resolution", params.nanobanana2_resolution)
            apply_str(payload, "output_format", params.nanobanana2_output_format)
            apply_str(payload, "safety_tolerance", params.nanobanana2_safety_tolerance)
            apply_num(payload, "seed", params.nanobanana2_seed)
            if params.nanobanana2_thinking_level ~= "off" then
                apply_str(payload, "thinking_level", params.nanobanana2_thinking_level)
            end
            apply_str(payload, "system_prompt", params.nanobanana2_system_prompt)
            apply_bool(payload, "enable_web_search", params.nanobanana2_enable_web_search)
            apply_bool(payload, "limit_generations", params.nanobanana2_limit_generations)
            mah.log("info", "[fal.ai] build_request: nanobanana2 edit mode, image_count=" .. #image_urls .. ", aspect=" .. tostring(payload.aspect_ratio) .. ", res=" .. tostring(payload.resolution) .. ", safety=" .. tostring(payload.safety_tolerance))
            return FAL_ENDPOINTS.nanobanana2, payload

        elseif model == "nanobananapro" then
            -- NanoBananaProEditInput: same family as nanobanana2, but its
            -- aspect_ratio enum drops the extreme 4:1/1:4/8:1/1:8 panoramas and
            -- its resolution enum has no 0.5K.
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {
                image_urls = image_urls,
                prompt = prompt,
            }
            apply_str(payload, "aspect_ratio", params.nanobananapro_aspect_ratio)
            apply_str(payload, "resolution", params.nanobananapro_resolution)
            apply_str(payload, "output_format", params.nanobananapro_output_format)
            apply_str(payload, "safety_tolerance", params.nanobananapro_safety_tolerance)
            apply_num(payload, "seed", params.nanobananapro_seed)
            apply_str(payload, "system_prompt", params.nanobananapro_system_prompt)
            apply_bool(payload, "enable_web_search", params.nanobananapro_enable_web_search)
            apply_bool(payload, "limit_generations", params.nanobananapro_limit_generations)
            mah.log("info", "[fal.ai] build_request: nanobananapro edit mode, image_count=" .. #image_urls .. ", aspect=" .. tostring(payload.aspect_ratio) .. ", res=" .. tostring(payload.resolution) .. ", safety=" .. tostring(payload.safety_tolerance))
            return FAL_ENDPOINTS.nanobananapro, payload

        elseif model == "gptimage2" then
            -- GptImage2EditInput sizes the output with an image_size enum (no
            -- aspect_ratio / resolution) and has no safety_tolerance; `quality`
            -- is the cost/detail dial. Accepts up to 16 reference images.
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {
                image_urls = image_urls,
                prompt = prompt,
            }
            apply_str(payload, "image_size", params.gptimage2_image_size)
            apply_str(payload, "quality", params.gptimage2_quality)
            apply_str(payload, "output_format", params.gptimage2_output_format)
            apply_str(payload, "background", params.gptimage2_background)
            if payload.background == "transparent" and payload.output_format == "jpeg" then
                payload.output_format = "png"
            end
            mah.log("info", "[fal.ai] build_request: gptimage2 edit mode, image_count=" .. #image_urls .. ", size=" .. tostring(payload.image_size) .. ", quality=" .. tostring(payload.quality))
            return FAL_ENDPOINTS.gptimage2, payload

        elseif model == "seedream5" then
            -- SeedreamV5ProEditInput: image_size enum (its auto_1K / auto_2K keep
            -- the source's ratio and only set the target area), output_format is
            -- jpeg|png only, and the safety switch is a boolean, not a tolerance.
            -- Uses the last 10 images if more are sent.
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {
                image_urls = image_urls,
                prompt = prompt,
            }
            apply_str(payload, "image_size", params.seedream5_image_size)
            apply_str(payload, "output_format", params.seedream5_output_format)
            apply_bool(payload, "enable_safety_checker", params.seedream5_enable_safety_checker)
            mah.log("info", "[fal.ai] build_request: seedream5 edit mode, image_count=" .. #image_urls .. ", size=" .. tostring(payload.image_size) .. ", safety_checker=" .. tostring(payload.enable_safety_checker))
            return FAL_ENDPOINTS.seedream5, payload

        elseif model == "grok2" then
            -- GrokImagineImageV20EditInput: its own aspect_ratio enum (2:1, 20:9,
            -- 19.5:9 … 1:2 — no 21:9 / 5:4 / 4:5), lowercase '1k'/'2k' resolution,
            -- and a low|medium quality dial in place of a safety tolerance.
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            -- The picker's max is a single number shared by every model, so it
            -- cannot hold this model to its own ceiling of 3. Say so plainly
            -- rather than letting fal.ai reject the request opaquely.
            if #image_urls > 3 then
                error("Grok Imagine 2.0 accepts at most 3 input images, but "
                    .. #image_urls .. " were sent. Remove some from Additional Images.")
            end
            local payload = {
                image_urls = image_urls,
                prompt = prompt,
            }
            apply_str(payload, "aspect_ratio", params.grok2_aspect_ratio)
            apply_str(payload, "resolution", params.grok2_resolution)
            apply_str(payload, "quality", params.grok2_quality)
            apply_str(payload, "output_format", params.grok2_output_format)
            mah.log("info", "[fal.ai] build_request: grok2 edit mode, image_count=" .. #image_urls .. ", aspect=" .. tostring(payload.aspect_ratio) .. ", res=" .. tostring(payload.resolution) .. ", quality=" .. tostring(payload.quality))
            return FAL_ENDPOINTS.grok2, payload

        elseif model == "nanobanana_lite" then
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {image_urls = image_urls, prompt = prompt}
            apply_str(payload, "aspect_ratio", params.nanobanana_lite_aspect_ratio)
            apply_str(payload, "output_format", params.nanobanana_lite_output_format)
            apply_str(payload, "safety_tolerance", params.nanobanana_lite_safety_tolerance)
            apply_num(payload, "seed", params.nanobanana_lite_seed)
            if params.nanobanana_lite_thinking_level ~= "off" then
                apply_str(payload, "thinking_level", params.nanobanana_lite_thinking_level)
            end
            apply_str(payload, "system_prompt", params.nanobanana_lite_system_prompt)
            apply_bool(payload, "limit_generations", params.nanobanana_lite_limit_generations)
            mah.log("info", "[fal.ai] build_request: Nano Banana Lite, image_count=" .. #image_urls)
            return FAL_ENDPOINTS.nanobanana_lite, payload

        elseif model == "muse" then
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {image_urls = image_urls, prompt = prompt}
            if params.muse_aspect_ratio ~= "auto" then
                apply_str(payload, "aspect_ratio", params.muse_aspect_ratio)
            end
            apply_str(payload, "output_format", params.muse_output_format)
            mah.log("info", "[fal.ai] build_request: Meta Muse, image_count=" .. #image_urls)
            return FAL_ENDPOINTS.muse, payload

        elseif model == "fibo15" then
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            if #image_urls > 4 then
                error("Bria Fibo Edit 1.5 accepts at most 4 input images, but "
                    .. #image_urls .. " were sent. Remove some from Additional Images.")
            end
            local payload = {image_urls = image_urls, instruction = prompt}
            if params.fibo15_aspect_ratio ~= "auto" then
                apply_str(payload, "aspect_ratio", params.fibo15_aspect_ratio)
            end
            apply_num(payload, "seed", params.fibo15_seed)
            mah.log("info", "[fal.ai] build_request: Bria Fibo Edit 1.5, image_count=" .. #image_urls)
            return FAL_ENDPOINTS.fibo15, payload

        else
            -- flux2 turbo / flux2pro: image_urls + prompt. Schemas diverge:
            --   Flux2TurboEditImageInput  has guidance_scale (number) but NO safety_tolerance.
            --   Flux2ProImageEditInput    has safety_tolerance (string enum '1'..'5') but NO guidance_scale.
            local endpoint = FAL_ENDPOINTS[model] or FAL_ENDPOINTS.flux2
            local image_urls = edit_image_urls(data_uri, extra_data_uris)
            local payload = {
                image_urls = image_urls,
                prompt = prompt,
            }
            if model == "flux2pro" then
                apply_str(payload, "image_size", params.flux2pro_image_size)
                apply_str(payload, "output_format", params.flux2pro_output_format)
                apply_str(payload, "safety_tolerance", params.flux2pro_safety_tolerance)
                apply_num(payload, "seed", params.flux2pro_seed)
                apply_bool(payload, "enable_safety_checker", params.flux2pro_enable_safety_checker)
            else
                if #image_urls > 4 then
                    error("FLUX.2 Turbo accepts at most 4 input images, but "
                        .. #image_urls .. " were sent. Remove some from Additional Images.")
                end
                payload.guidance_scale = tonumber(params.flux2_guidance_scale) or 2.5
                apply_str(payload, "image_size", params.flux2_image_size)
                apply_str(payload, "output_format", params.flux2_output_format)
                apply_num(payload, "seed", params.flux2_seed)
                apply_bool(payload, "enable_prompt_expansion", params.flux2_enable_prompt_expansion)
                apply_bool(payload, "enable_safety_checker", params.flux2_enable_safety_checker)
            end
            mah.log("info", "[fal.ai] build_request: using endpoint=" .. endpoint .. ", image_count=" .. #image_urls .. ", image_size=" .. tostring(payload.image_size) .. ", output_format=" .. tostring(payload.output_format) .. ", safety=" .. tostring(payload.safety_tolerance) .. ", guidance=" .. tostring(payload.guidance_scale))
            return endpoint, payload
        end

    elseif action_id == "polish" then
        local model = params.model or "post_processing"
        if model == "topaz_sharpen" then
            local payload = {image_url = data_uri}
            apply_str(payload, "model", params.topaz_sharpen_model)
            apply_str(payload, "output_format", params.topaz_sharpen_output_format)
            mah.log("info", "[fal.ai] build_request: Topaz Sharpen model=" .. tostring(payload.model))
            return FAL_ENDPOINTS.topaz_sharpen, payload
        end
        -- post-processing: sharpen / grain / etc. We expose only the sharpen
        -- group since that's the useful follow-up after a denoise pass; the
        -- model has ~50 other params left at their defaults (all gated by
        -- enable_* flags, all default off).
        local payload = {image_url = data_uri, enable_sharpen = true}
        apply_str(payload, "sharpen_mode", params.sharpen_mode)
        apply_num(payload, "sharpen_radius", params.sharpen_radius)
        apply_num(payload, "sharpen_alpha", params.sharpen_alpha)
        apply_num(payload, "noise_radius", params.noise_radius)
        apply_num(payload, "preserve_edges", params.preserve_edges)
        apply_num(payload, "smart_sharpen_strength", params.smart_sharpen_strength)
        apply_num(payload, "smart_sharpen_ratio", params.smart_sharpen_ratio)
        apply_num(payload, "cas_amount", params.cas_amount)
        mah.log("info", "[fal.ai] build_request: post-processing sharpen mode=" .. tostring(payload.sharpen_mode))
        return FAL_ENDPOINTS.post_processing, payload

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

-- Submit a request to the fal.ai queue API and poll until COMPLETED.
-- Avoids the 120s sync timeout on `fal.run/` (cold starts on less-popular models
-- like nafnet/denoise can exceed it). Returns the parsed result table on success;
-- on failure raises via error() with a descriptive message.
--
-- Polling cadence: starts at 1s, grows linearly to a max of 5s, capped at 15min total.
-- The progress range [progress_start, progress_end] is reported via mah.job_progress
-- (if job_id is provided) as the request moves IN_QUEUE -> IN_PROGRESS -> COMPLETED.
local function fal_submit_and_wait(endpoint, payload, api_key, job_id, progress_start, progress_end)
    local headers = fal_request_headers(api_key)
    local payload_json = mah.json.encode(payload)
    local submit_url = "https://queue.fal.run/" .. endpoint
    mah.log("info", "[fal.ai] queue submit: POST " .. submit_url .. " (payload=" .. #payload_json .. " bytes)")

    local resp = mah.http.post_sync(submit_url, payload_json, {headers = headers, timeout = 60})
    if resp.error then
        error("HTTP submit failed: " .. resp.error)
    end
    if resp.status_code ~= 200 and resp.status_code ~= 201 then
        error("API submit error (status " .. tostring(resp.status_code) .. "): " .. (resp.body or ""):sub(1, 500))
    end

    local submit_result = mah.json.decode(resp.body)
    if not submit_result or not submit_result.status_url or not submit_result.response_url then
        error("Submit response missing status_url/response_url: " .. (resp.body or ""):sub(1, 300))
    end

    local request_id = submit_result.request_id or "?"
    mah.log("info", "[fal.ai] queue submit: request_id=" .. request_id .. ", status_url=" .. submit_result.status_url)

    local span = (progress_end or 70) - (progress_start or 20)
    if span < 0 then span = 0 end

    -- Poll until terminal state. 15 min cap = plenty for any current fal.ai model.
    local max_wait_s = 15 * 60
    local elapsed = 0
    local delay = 1
    local last_status = ""
    while elapsed < max_wait_s do
        mah.sleep(delay)
        elapsed = elapsed + delay
        if delay < 5 then delay = delay + 1 end

        local status_resp = mah.http.get_sync(submit_result.status_url, {headers = headers, timeout = 30})
        if status_resp.error then
            mah.log("warn", "[fal.ai] queue poll: transient error, will retry: " .. status_resp.error)
        elseif status_resp.status_code == 200 then
            local s = mah.json.decode(status_resp.body) or {}
            if s.status ~= last_status then
                mah.log("info", "[fal.ai] queue poll: request_id=" .. request_id .. " status=" .. tostring(s.status) .. " (elapsed=" .. elapsed .. "s)")
                last_status = s.status or ""
            end
            if s.status == "COMPLETED" then
                if job_id then
                    mah.job_progress(job_id, (progress_start or 20) + span, "Fetching result...")
                end
                local result_resp = mah.http.get_sync(submit_result.response_url, {headers = headers, timeout = 60})
                if result_resp.error then
                    error("HTTP result fetch failed: " .. result_resp.error)
                end
                if result_resp.status_code ~= 200 then
                    error("API result error (status " .. tostring(result_resp.status_code) .. "): " .. (result_resp.body or ""):sub(1, 500))
                end
                local result = mah.json.decode(result_resp.body)
                if not result then
                    error("Failed to parse result body")
                end
                if result.error then
                    error("fal.ai reported error: " .. tostring(result.error))
                end
                return result
            elseif s.status == "IN_QUEUE" then
                if job_id and span > 0 then
                    mah.job_progress(job_id, (progress_start or 20), "Queued (position " .. tostring(s.queue_position or 0) .. ")...")
                end
            elseif s.status == "IN_PROGRESS" then
                if job_id and span > 0 then
                    -- Crude linear ramp toward progress_end - 5; we don't know real %.
                    local pct = (progress_start or 20) + math.floor(span * 0.6)
                    mah.job_progress(job_id, pct, "Processing on fal.ai...")
                end
            elseif s.status == "FAILED" or s.status == "CANCELLED" then
                error("fal.ai request " .. tostring(s.status) .. ": " .. (status_resp.body or ""):sub(1, 500))
            end
        else
            mah.log("warn", "[fal.ai] queue poll: status=" .. tostring(status_resp.status_code) .. ", body=" .. (status_resp.body or ""):sub(1, 200))
        end
    end

    error("fal.ai request timed out after " .. max_wait_s .. "s (request_id=" .. request_id .. ")")
end

-- Call fal.ai API and create a new resource from the result
local function process_image(resource_id, action_id, params, api_key, job_id)
    mah.log("info", "[fal.ai] process_image: resource_id=" .. tostring(resource_id) .. ", action=" .. action_id)

    -- Build data URI (validates format; raises on failure so pcall in make_handler catches it)
    mah.log("info", "[fal.ai] process_image: loading resource data for resource #" .. tostring(resource_id))
    local data_uri, mime_type = build_data_uri(resource_id)
    mah.log("info", "[fal.ai] process_image: data URI built, total size=" .. #data_uri .. " bytes, mime=" .. mime_type)

    if job_id then
        mah.job_progress(job_id, 10, "Preparing image...")
    end

    -- Pre-pad image for photo_restoration to prevent aspect ratio warping.
    -- The model always reshapes output to the selected fixed ratio, so
    -- adding white borders to the input ensures the content stays proportional.
    if action_id == "restore" and (params.model or "photo_restoration") == "photo_restoration" then
        local ratio = params.aspect_ratio
        if ratio == nil or ratio == "" or ratio == "auto" then
            ratio = auto_aspect_ratio_for(resource_id)
        end
        if ratio then
            -- The Go binding returns (padded_uri, w, h) on success or (nil, err)
            -- on failure -- it does NOT raise, so pcall reports ok=true even when
            -- padding failed. Guard on `padded` being non-nil, otherwise we'd
            -- silently null out data_uri and send the API a payload with no image.
            local ok, padded, w_or_err, h = pcall(mah.image.pad_to_aspect_ratio, data_uri, ratio)
            if ok and padded then
                mah.log("info", "[fal.ai] process_image: padded to " .. ratio .. " (" .. tostring(w_or_err) .. "x" .. tostring(h) .. ")")
                data_uri = padded
            else
                mah.log("warn", "[fal.ai] process_image: padding failed (" .. tostring(padded or w_or_err) .. "), sending original")
            end
        end
    end

    -- Build the full image-URI list for multi-image actions (every edit model except flux1dev).
    -- extra_images uses default="trigger" so the frontend prefills the source resource; the handler
    -- iterates that list directly (no re-prepending). When show_when hides the param (e.g. flux1dev
    -- or non-edit actions), params.extra_images is nil and we fall back to source-only.
    local all_image_uris = {}
    local extras = params.extra_images
    if extras and #extras > 0 then
        mah.log("info", "[fal.ai] process_image: loading " .. #extras .. " extra image(s)")
        for _, eid in ipairs(extras) do
            local du, _ = build_data_uri(eid)
            all_image_uris[#all_image_uris + 1] = du
        end
    else
        -- show_when hid the param for this model (e.g. flux1dev), or user cleared it.
        all_image_uris[1] = data_uri
    end

    -- Build API request
    local endpoint, payload = build_request(action_id, data_uri, params, resource_id, all_image_uris)
    if not endpoint then
        mah.log("error", "[fal.ai] process_image: unknown action " .. action_id)
        error("Unknown action: " .. action_id)
    end

    if job_id then
        mah.job_progress(job_id, 20, "Submitting to fal.ai...")
    end

    -- Use queue API + polling so cold-starts >120s (e.g. nafnet/denoise) don't time out.
    local result = fal_submit_and_wait(endpoint, payload, api_key, job_id, 20, 70)

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
    "image/tiff", "image/bmp",
}

-- Shared "Save Result As" toggle: version (default) vs clone (new resource).
-- Vectorize doesn't expose this since it always clones (output is SVG).
local OUTPUT_MODE_PARAM = {
    name = "output_mode", type = "select", label = "Save Result As",
    default = "version", options = {"version", "clone"},
    description = "version adds the result to this resource; clone creates a separate resource and copies its metadata and relationships.",
}

-- The Generate form's aspect-ratio control mapped onto the `image_size` enum used
-- by the text-to-image models that have no aspect_ratio field at all. 3:2 and 2:3
-- have no exact member, so they take the closest landscape / portrait size.
local GENERATE_IMAGE_SIZE = {
    ["1:1"]  = "square_hd",
    ["16:9"] = "landscape_16_9",
    ["4:3"]  = "landscape_4_3",
    ["3:2"]  = "landscape_4_3",
    ["9:16"] = "portrait_16_9",
    ["3:4"]  = "portrait_4_3",
    ["2:3"]  = "portrait_4_3",
}

local RESOLUTION_RANK = {["0.5K"] = 1, ["1K"] = 2, ["2K"] = 3, ["4K"] = 4}

local function optional_choice(value, omitted)
    if value == omitted or value == "" then return nil end
    return value
end

local function jpeg_or_png(value)
    if value == "png" then return "png" end
    return "jpeg"
end

-- Snap the form's resolution to the nearest member of the enum a given model
-- accepts, so asking a 2K-max model for 4K yields 2K rather than its floor.
local function nearest_resolution(resolution, allowed)
    local want = RESOLUTION_RANK[resolution] or RESOLUTION_RANK["1K"]
    local best, best_diff = allowed[1], math.huge
    for _, r in ipairs(allowed) do
        local d = math.abs(RESOLUTION_RANK[r] - want)
        if d < best_diff then
            best, best_diff = r, d
        end
    end
    return best
end

-- Text-to-image models offered by the Generate page. The page renders one shared
-- form (prompt / resolution / aspect ratio / safety tolerance), but the schemas
-- behind it diverge: some models have no aspect_ratio and size their output with
-- an image_size enum, some have no resolution or a differently-cased one, and the
-- safety switch is a string tolerance on one model and a boolean on another. Each
-- entry therefore owns its endpoint and maps the shared controls onto its own
-- schema — a field the schema does not declare is never sent.
--
-- Order matters: the first entry is the form's default selection.
local GENERATE_MODELS = {
    {
        id = "nanobanana2", label = "Nano Banana 2",
        info = "Google's quality-focused all-rounder: strong prompt reasoning, typography, optional web grounding, and 0.5K-4K output.",
        endpoint = FAL_ENDPOINTS.nanobanana2_generate,
        build = function(o)
            return {
                prompt = o.prompt,
                aspect_ratio = o.aspect_ratio,
                resolution = o.resolution,
                safety_tolerance = o.safety_tolerance,
                output_format = o.output_format,
                seed = o.seed,
                thinking_level = optional_choice(o.thinking_level, "off"),
                system_prompt = optional_choice(o.system_prompt, ""),
                enable_web_search = o.enable_web_search,
            }
        end,
    },
    {
        id = "nanobananapro", label = "Nano Banana Pro",
        info = "Google's high-fidelity model for complex composition and typography. Supports 1K-4K, but not the newer thinking-level control.",
        endpoint = FAL_ENDPOINTS.nanobananapro_generate,
        build = function(o)
            -- Same family as Nano Banana 2, but its resolution enum has no 0.5K.
            return {
                prompt = o.prompt,
                aspect_ratio = o.aspect_ratio,
                resolution = nearest_resolution(o.resolution, {"1K", "2K", "4K"}),
                safety_tolerance = o.safety_tolerance,
                output_format = o.output_format,
                seed = o.seed,
                system_prompt = optional_choice(o.system_prompt, ""),
                enable_web_search = o.enable_web_search,
            }
        end,
    },
    {
        id = "nanobanana_lite", label = "Nano Banana 2 Lite",
        info = "Google's speed/cost-focused model. Sub-2-second target latency, optional thinking, and no explicit resolution control.",
        endpoint = FAL_ENDPOINTS.nanobanana_lite_generate,
        build = function(o)
            return {
                prompt = o.prompt,
                aspect_ratio = o.aspect_ratio,
                safety_tolerance = o.safety_tolerance,
                output_format = o.output_format,
                seed = o.seed,
                thinking_level = optional_choice(o.thinking_level, "off"),
                system_prompt = optional_choice(o.system_prompt, ""),
            }
        end,
    },
    {
        id = "gptimage2", label = "GPT Image 2",
        info = "OpenAI's detail and typography specialist. Quality changes both detail and cost; transparent background works with PNG or WebP.",
        endpoint = FAL_ENDPOINTS.gptimage2_generate,
        build = function(o)
            -- No aspect_ratio, resolution or safety_tolerance in this schema;
            -- `quality` (default high) is the detail/cost dial.
            local format = o.output_format
            if o.background == "transparent" and format == "jpeg" then format = "png" end
            return {
                prompt = o.prompt,
                image_size = GENERATE_IMAGE_SIZE[o.aspect_ratio] or "auto",
                quality = o.quality,
                background = o.background,
                output_format = format,
            }
        end,
    },
    {
        id = "seedream5", label = "Seedream 5.0 Pro",
        info = "ByteDance's layout- and typography-focused flagship, with strong prompt understanding across dense compositions and 14 languages.",
        endpoint = FAL_ENDPOINTS.seedream5_generate,
        build = function(o)
            -- Safety here is a boolean, not a tolerance: treat only the form's two
            -- strictest settings as "leave the checker on".
            return {
                prompt = o.prompt,
                image_size = GENERATE_IMAGE_SIZE[o.aspect_ratio] or "auto_2K",
                enable_safety_checker = (tonumber(o.safety_tolerance) or 6) <= 2,
                output_format = jpeg_or_png(o.output_format),
            }
        end,
    },
    {
        id = "grok2", label = "Grok Imagine Image 2.0",
        info = "xAI's aesthetic image model. Offers 1K/2K and low/medium quality; it has no seed or safety-tolerance field.",
        endpoint = FAL_ENDPOINTS.grok2_generate,
        build = function(o)
            -- Lowercase resolution enum capped at 2k, and no safety_tolerance.
            return {
                prompt = o.prompt,
                aspect_ratio = o.aspect_ratio,
                resolution = nearest_resolution(o.resolution, {"1K", "2K"}):lower(),
                quality = o.quality == "low" and "low" or "medium",
                output_format = o.output_format,
            }
        end,
    },
    {
        id = "muse", label = "Meta Muse Image",
        info = "Meta's newest image model, tuned for faithful instructions and fine detail such as text, plots, and QR codes. Resolution is automatic.",
        endpoint = FAL_ENDPOINTS.muse_generate,
        build = function(o)
            return {
                prompt = o.prompt,
                aspect_ratio = o.aspect_ratio,
                output_format = o.output_format,
            }
        end,
    },
    {
        id = "fibo15", label = "Bria Fibo Gen 1.5",
        info = "Commercially safe generation trained on licensed data, with accurate typography and optional style presets. Choose 1MP or 4MP via Resolution. A blank seed uses Fibo's deterministic default, 5555, rather than a random seed.",
        endpoint = FAL_ENDPOINTS.fibo15_generate,
        -- Fibo exposes no output-format input. Avoid inventing an extension for
        -- the endpoint's native response format when naming the resource.
        extension = "",
        build = function(o)
            return {
                prompt = o.prompt,
                aspect_ratio = o.aspect_ratio,
                resolution = RESOLUTION_RANK[o.resolution] >= RESOLUTION_RANK["2K"] and "4MP" or "1MP",
                style_preset = o.style_preset,
                seed = o.seed or 5555,
            }
        end,
    },
}

-- The values the Generate form can actually offer. Unlike action params, a page
-- POST is not validated by the plugin host, so a hand-crafted request could put
-- anything here — and the builders below hand several of these straight to fal.ai.
-- Anything unrecognised falls back to the form's own default rather than starting
-- a job that can only fail.
local GENERATE_ASPECT_RATIOS = {
    ["1:1"] = true, ["16:9"] = true, ["9:16"] = true, ["4:3"] = true,
    ["3:4"] = true, ["3:2"] = true, ["2:3"] = true,
}
local GENERATE_RESOLUTIONS = {["0.5K"] = true, ["1K"] = true, ["2K"] = true, ["4K"] = true}
local GENERATE_SAFETY = {
    ["1"] = true, ["2"] = true, ["3"] = true,
    ["4"] = true, ["5"] = true, ["6"] = true,
}
local GENERATE_OUTPUT_FORMATS = {jpeg = true, png = true, webp = true}
local GENERATE_QUALITY = {auto = true, low = true, medium = true, high = true}
local GENERATE_BACKGROUND = {auto = true, transparent = true, opaque = true}
local GENERATE_THINKING = {off = true, minimal = true, high = true}
local GENERATE_STYLE_PRESETS = {["No Style"] = true, Photoreal = true}

local function one_of(value, allowed, fallback)
    if value ~= nil and allowed[value] then return value end
    return fallback
end

-- Resolve a submitted model id, falling back to the form's default.
local function generate_model_by_id(id)
    for _, m in ipairs(GENERATE_MODELS) do
        if m.id == id then return m end
    end
    return GENERATE_MODELS[1]
end

local function generate_model_options()
    local html = ""
    for _, m in ipairs(GENERATE_MODELS) do
        html = html .. '<option value="' .. m.id .. '">' .. html_escape(m.label) .. '</option>'
    end
    return html
end

local function generate_model_guide()
    local html = '<details class="text-sm text-gray-600"><summary class="cursor-pointer font-medium">Model guide</summary><ul class="mt-2 space-y-2">'
    for _, m in ipairs(GENERATE_MODELS) do
        html = html .. '<li><strong>' .. html_escape(m.label) .. ':</strong> '
            .. html_escape(m.info) .. '</li>'
    end
    return html .. '</ul></details>'
end

local function generate_form()
    return '<form method="POST" class="space-y-4 max-w-lg">'
        .. '<div><label class="block font-medium mb-1" for="prompt">Prompt</label>'
        .. '<textarea id="prompt" name="prompt" required class="w-full border rounded p-2" rows="3" '
        .. 'placeholder="Describe the image you want to generate..."></textarea></div>'
        .. '<div><label class="block font-medium mb-1" for="model">Model</label>'
        .. '<select id="model" name="model" class="w-full border rounded p-2">'
        .. generate_model_options()
        .. '</select><p class="text-xs text-gray-500 mt-1">Each model receives only the controls supported by its live fal.ai schema.</p></div>'
        .. generate_model_guide()
        .. '<div><label class="block font-medium mb-1" for="resolution">Resolution</label>'
        .. '<select id="resolution" name="resolution" class="w-full border rounded p-2">'
        .. '<option value="0.5K">0.5K</option>'
        .. '<option value="1K" selected>1K</option>'
        .. '<option value="2K">2K</option>'
        .. '<option value="4K">4K</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">Unsupported sizes are mapped to the nearest model-native size; models with automatic sizing ignore this.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="aspect_ratio">Aspect Ratio</label>'
        .. '<select id="aspect_ratio" name="aspect_ratio" class="w-full border rounded p-2">'
        .. '<option value="1:1" selected>1:1</option>'
        .. '<option value="16:9">16:9</option>'
        .. '<option value="9:16">9:16</option>'
        .. '<option value="4:3">4:3</option>'
        .. '<option value="3:4">3:4</option>'
        .. '<option value="3:2">3:2</option>'
        .. '<option value="2:3">2:3</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">Controls output shape. Models that use image-size presets receive the closest matching preset.</p></div>'
        -- Options "1"–"6" cover the Nano Banana string safety_tolerance enums.
        -- Models without that field either ignore
        -- this control (GPT Image 2, Grok Imagine 2.0) or derive their boolean
        -- safety switch from it (Seedream 5.0 Pro) — see GENERATE_MODELS.
        .. '<div><label class="block font-medium mb-1" for="safety_tolerance">Safety Tolerance</label>'
        .. '<select id="safety_tolerance" name="safety_tolerance" class="w-full border rounded p-2">'
        .. '<option value="1">1 (strictest)</option>'
        .. '<option value="2">2</option>'
        .. '<option value="3">3</option>'
        .. '<option value="4">4</option>'
        .. '<option value="5">5</option>'
        .. '<option value="6" selected>6 (most permissive)</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">1 blocks the most content; 6 is most permissive. Ignored by models without a tolerance field.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="output_format">Output Format</label>'
        .. '<select id="output_format" name="output_format" class="w-full border rounded p-2">'
        .. '<option value="jpeg" selected>JPEG (small, no transparency)</option>'
        .. '<option value="png">PNG (lossless / transparency)</option>'
        .. '<option value="webp">WebP (small / transparency)</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">Seedream and Fibo do not support WebP; Seedream falls back to JPEG and Fibo chooses its native output.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="seed">Seed (optional)</label>'
        .. '<input id="seed" name="seed" type="number" step="1" class="w-full border rounded p-2" '
        .. 'placeholder="Model default"><p class="text-xs text-gray-500 mt-1">Reuses the model\'s random starting point where supported. Fibo defaults to deterministic seed 5555; other models may choose randomly.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="quality">Quality</label>'
        .. '<select id="quality" name="quality" class="w-full border rounded p-2">'
        .. '<option value="auto">Auto</option><option value="low">Low</option>'
        .. '<option value="medium">Medium</option><option value="high" selected>High</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">Used by GPT Image 2 and Grok. Higher GPT quality increases detail and cost; Grok maps auto/high to its maximum, medium.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="background">Background</label>'
        .. '<select id="background" name="background" class="w-full border rounded p-2">'
        .. '<option value="auto" selected>Auto</option><option value="opaque">Opaque</option>'
        .. '<option value="transparent">Transparent</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">GPT Image 2 only. Choose PNG or WebP when requesting transparency.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="thinking_level">Thinking Level</label>'
        .. '<select id="thinking_level" name="thinking_level" class="w-full border rounded p-2">'
        .. '<option value="off" selected>Off</option><option value="minimal">Minimal</option>'
        .. '<option value="high">High</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">Nano Banana 2 and Lite only. More thinking can improve complex instructions, with extra latency/cost.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="system_prompt">System Prompt (optional)</label>'
        .. '<textarea id="system_prompt" name="system_prompt" class="w-full border rounded p-2" rows="2" '
        .. 'placeholder="Persistent style or behavior instruction"></textarea><p class="text-xs text-gray-500 mt-1">Nano Banana family only; steers the model separately from the image prompt.</p></div>'
        .. '<div><label class="inline-flex items-center gap-2"><input type="checkbox" name="enable_web_search" value="true">'
        .. '<span class="font-medium">Enable Web Search</span></label><p class="text-xs text-gray-500 mt-1">Nano Banana 2 / Pro only. Grounds time-sensitive prompts in current web information and may add cost.</p></div>'
        .. '<div><label class="block font-medium mb-1" for="style_preset">Style Preset</label>'
        .. '<select id="style_preset" name="style_preset" class="w-full border rounded p-2">'
        .. '<option value="No Style" selected>No Style</option><option value="Photoreal">Photoreal</option>'
        .. '</select><p class="text-xs text-gray-500 mt-1">Bria Fibo Gen 1.5 only.</p></div>'
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
        params = {
            {name = "model", type = "select", label = "Model", default = "ddcolor",
                options = {"ddcolor", "topaz_colorize"},
                description = "DDColor is the fast photographic default; Topaz Colorize is a newer professional colorization pass."},
            {name = "model_info_ddcolor", type = "info", label = "DDColor — fast photo colorization",
                description = "Designed for black-and-white photographs. It has no tuning controls and returns a colorized raster image.",
                show_when = {model = "ddcolor"}},
            {name = "model_info_topaz_colorize", type = "info", label = "Topaz Colorize — professional archival color",
                description = "Topaz Labs' current one-click colorization model. It preserves source resolution and lets you choose JPEG or PNG output.",
                show_when = {model = "topaz_colorize"}},
            {name = "topaz_colorize_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "topaz_colorize"}},
            OUTPUT_MODE_PARAM,
        },
        handler = make_handler("colorize"),
    })

    -- Adjust: detail + card
    mah.action({
        id = "adjust",
        label = "Adjust Color & Lighting",
        description = "Correct exposure, lighting, or white balance with Topaz Labs",
        icon = "sun",
        entity = "resource",
        placement = {"detail", "card"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        params = {
            {name = "model", type = "select", label = "Adjustment", default = "adjust_v2",
                options = {"adjust_v2", "white_balance"},
                description = "Adjust V2 corrects overall exposure and lighting; White Balance removes unwanted color casts."},
            {name = "model_info_adjust_v2", type = "info", label = "Adjust V2 — exposure and lighting",
                description = "One-click professional correction for flat, underexposed, overexposed, or unevenly lit photos. Output keeps source resolution.",
                show_when = {model = "adjust_v2"}},
            {name = "model_info_white_balance", type = "info", label = "White Balance — neutralize color casts",
                description = "Corrects warm/cool or green/magenta casts so neutral areas look neutral. Output keeps source resolution.",
                show_when = {model = "white_balance"}},
            {name = "output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless."},
            OUTPUT_MODE_PARAM,
        },
        handler = make_handler("adjust"),
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
                options = {"clarity", "crystal", "esrgan", "creative", "seedvr",
                           "bria_creative", "topaz", "topaz_generative",
                           "topaz_creative", "topaz_transparent", "drct", "aura_sr"},
                description = "Choose a faithful, restoration-aware, creative, portrait, or transparency-preserving upscaler."},

            {name = "model_info_clarity", type = "info", label = "Clarity — prompt-guided general upscaling",
                description = "Flexible diffusion upscaler with prompt, creativity, resemblance, CFG, and step controls. Good when you want to steer reconstructed detail; higher creativity can change the source.",
                show_when = {model = "clarity"}},
            {name = "model_info_crystal", type = "info", label = "Crystal — faces and portraits",
                description = "Clarity AI's newer portrait-focused upscaler. Creativity 0 is faithful; raising it invents facial and photographic detail.",
                show_when = {model = "crystal"}},
            {name = "model_info_esrgan", type = "info", label = "ESRGAN — fast deterministic super-resolution",
                description = "Classic Real-ESRGAN presets for photos, anime, and faces. Fast and predictable, but less capable at reconstructing severely degraded detail.",
                show_when = {model = "esrgan"}},
            {name = "model_info_creative", type = "info", label = "Creative Upscaler — controlled reinterpretation",
                description = "Adds prompt-guided detail with explicit creativity, detail, and shape-preservation controls. Best when some visual reinterpretation is welcome.",
                show_when = {model = "creative"}},
            {name = "model_info_seedvr", type = "info", label = "SeedVR2 — restoration-aware quality",
                description = "Strong general upscaler for degraded inputs. Factor mode scales uniformly; target mode aims at a resolution. Seamless mode is useful for repeating textures.",
                show_when = {model = "seedvr"}},
            {name = "model_info_bria", type = "info", label = "Bria Creative — licensed-data creative upscale",
                description = "Commercially oriented creative upscaling from Bria, with optional alpha preservation and minimal tuning.",
                show_when = {model = "bria_creative"}},
            {name = "model_info_topaz", type = "info", label = "Topaz Precision — faithful professional upscale",
                description = "Current Topaz precision family. Standard V2 fits most photos; High Fidelity preserves professional detail; Low Resolution recovers compressed sources; CGI targets rendered art; Text Refine keeps text crisp.",
                show_when = {model = "topaz"}},
            {name = "model_info_topaz_generative", type = "info", label = "Topaz Generative — rebuild missing detail",
                description = "Wonder 3.5/3 add realistic detail; Recover/Recovery rebuild tiny sources; Redefine enables prompt-guided reconstruction. More generative than Precision and usually more expensive.",
                show_when = {model = "topaz_generative"}},
            {name = "model_info_topaz_creative", type = "info", label = "Topaz Creative — transform AI artwork",
                description = "Bloom-family upscaling for AI-generated images. Creativity can substantially alter detail; color preservation reins in palette drift.",
                show_when = {model = "topaz_creative"}},
            {name = "model_info_topaz_transparent", type = "info", label = "Topaz Transparent — fixed 4x PNG",
                description = "Preserves the alpha channel end to end for logos, stickers, and cutouts. Output is always PNG and exactly 4x in each dimension.",
                show_when = {model = "topaz_transparent"}},
            {name = "model_info_drct", type = "info", label = "DRCT — compressed-photo super-resolution",
                description = "Degradation-aware super-resolution trained for real-world artifacts. A strong faithful choice for JPEG-compressed photos.",
                show_when = {model = "drct"}},
            {name = "model_info_aura", type = "info", label = "Aura SR — tile-based 4x GAN",
                description = "Efficient tile-based upscaling. Checkpoint v2 handles JPEG degradation better; overlapping tiles reduce seams.",
                show_when = {model = "aura_sr"}},

            -- Clarity
            {name = "clarity_prompt", type = "text", label = "Prompt",
                default = "masterpiece, best quality, highres",
                description = "Positive guidance for the detail the model should reconstruct.",
                show_when = {model = "clarity"}},
            {name = "clarity_negative_prompt", type = "text", label = "Negative Prompt",
                default = "(worst quality, low quality, normal quality:2)",
                description = "Features and artifacts to suppress.",
                show_when = {model = "clarity"}},
            {name = "clarity_upscale_factor", type = "number", label = "Upscale Factor", default = 2,
                min = 1, max = 4, step = 0.25,
                description = "Uniform width/height multiplier; 2 doubles each dimension.",
                show_when = {model = "clarity"}},
            {name = "clarity_creativity", type = "number", label = "Creativity (denoise strength)",
                default = 0.35, min = 0, max = 1, step = 0.05,
                description = "How much new detail may be invented. Lower values stay closer to the source.",
                show_when = {model = "clarity"}},
            {name = "clarity_resemblance", type = "number", label = "Resemblance to Original",
                default = 0.6, min = 0, max = 1, step = 0.05,
                description = "How strongly the output should preserve the original image.",
                show_when = {model = "clarity"}},
            {name = "clarity_guidance_scale", type = "number", label = "Guidance Scale (CFG)",
                default = 4, min = 0, max = 20, step = 0.5,
                description = "Prompt adherence. Too high can introduce harsh or artificial detail.",
                show_when = {model = "clarity"}},
            {name = "clarity_num_inference_steps", type = "number", label = "Inference Steps",
                default = 18, min = 1, max = 60, step = 1,
                description = "More steps may refine detail but take longer; returns diminish at higher values.",
                show_when = {model = "clarity"}},

            -- Crystal (Clarity AI's successor to clarity-upscaler, portrait-focused)
            {name = "crystal_scale_factor", type = "number", label = "Scale Factor",
                default = 2, min = 1, max = 4, step = 0.25,
                description = "Uniform width/height multiplier.",
                show_when = {model = "crystal"}},
            {name = "crystal_creativity", type = "number", label = "Creativity (0 = faithful)",
                default = 0, min = 0, max = 10, step = 0.5,
                description = "0 is a faithful upscale; higher values reconstruct more aggressively.",
                show_when = {model = "crystal"}},
            {name = "crystal_output_format", type = "select", label = "Output Format",
                default = "png", options = {"png", "jpg"},
                description = "PNG is lossless; JPG is smaller.",
                show_when = {model = "crystal"}},

            -- ESRGAN
            {name = "esrgan_model", type = "select", label = "ESRGAN Model",
                default = "RealESRGAN_x4plus",
                options = {"RealESRGAN_x4plus", "RealESRGAN_x2plus",
                           "RealESRGAN_x4plus_anime_6B", "RealESRGAN_x4_v3",
                           "RealESRGAN_x4_wdn_v3", "RealESRGAN_x4_anime_v3"},
                description = "x4plus is the photo default; x2plus is gentler; anime variants favor illustration and line art.",
                show_when = {model = "esrgan"}},
            {name = "esrgan_scale", type = "number", label = "Scale",
                default = 4, min = 1, max = 4, step = 1,
                description = "Requested scale multiplier; the selected checkpoint may be optimized for 2x or 4x.",
                show_when = {model = "esrgan"}},
            {name = "esrgan_face", type = "boolean", label = "Face Mode (portraits)",
                default = false,
                description = "Runs face enhancement for portrait inputs.",
                show_when = {model = "esrgan"}},
            {name = "esrgan_output_format", type = "select", label = "Output Format",
                default = "png", options = {"png", "jpeg"},
                description = "PNG is lossless; JPEG is smaller.",
                show_when = {model = "esrgan"}},

            -- Creative Upscaler
            {name = "creative_prompt", type = "text", label = "Prompt (optional, guides creativity)",
                description = "Describe the detail or style the upscaler should add; leave blank for automatic interpretation.",
                show_when = {model = "creative"}},
            {name = "creative_scale", type = "number", label = "Scale",
                default = 2, min = 1, max = 4, step = 0.25,
                description = "Uniform width/height multiplier.",
                show_when = {model = "creative"}},
            {name = "creative_creativity", type = "number", label = "Creativity",
                default = 0.5, min = 0, max = 1, step = 0.05,
                description = "Higher values invent more detail and can depart from the source.",
                show_when = {model = "creative"}},
            {name = "creative_detail", type = "number", label = "Detail",
                default = 1, min = 0, max = 2, step = 0.1,
                description = "Amount of fine texture and micro-detail to add.",
                show_when = {model = "creative"}},
            {name = "creative_shape_preservation", type = "number", label = "Shape Preservation",
                default = 0.25, min = 0, max = 1, step = 0.05,
                description = "Higher values retain the source geometry more strongly.",
                show_when = {model = "creative"}},

            -- SeedVR
            {name = "seedvr_upscale_mode", type = "select", label = "Upscale Mode",
                default = "factor", options = {"factor", "target"},
                description = "factor multiplies source dimensions; target aims for a named output resolution.",
                show_when = {model = "seedvr"}},
            {name = "seedvr_upscale_factor", type = "number", label = "Upscale Factor",
                default = 2, min = 1, max = 4, step = 0.25,
                description = "Uniform multiplier used in factor mode.",
                show_when = {model = "seedvr", seedvr_upscale_mode = "factor"}},
            {name = "seedvr_target_resolution", type = "select", label = "Target Resolution",
                default = "1080p", options = {"720p", "1080p", "1440p", "2160p"},
                description = "Target resolution used in target mode while preserving aspect ratio.",
                show_when = {model = "seedvr", seedvr_upscale_mode = "target"}},
            {name = "seedvr_noise_scale", type = "number", label = "Noise Scale",
                default = 0.1, min = 0, max = 1, step = 0.05,
                description = "Controls variation introduced during restoration; lower values are more conservative.",
                show_when = {model = "seedvr"}},
            {name = "seedvr_output_format", type = "select", label = "Output Format",
                default = "jpg", options = {"jpg", "png", "webp"},
                description = "JPG is smallest; PNG is lossless; WebP balances size and quality.",
                show_when = {model = "seedvr"}},
            {name = "seedvr_seamless", type = "boolean", label = "Seamless Tiling (slower)",
                default = false,
                description = "Runs the seamless SeedVR endpoint, which tiles without visible seams. Use for textures and patterns.",
                show_when = {model = "seedvr"}},

            -- Bria Creative
            {name = "bria_preserve_alpha", type = "boolean", label = "Preserve Alpha Channel",
                default = true,
                description = "Keep transparency from PNG/WebP inputs where possible.",
                show_when = {model = "bria_creative"}},

            -- Topaz Precision
            {name = "topaz_model", type = "select", label = "Topaz Model", default = "Standard V2",
                options = {"Standard V2", "High Fidelity V3", "High Fidelity V2",
                           "Low Resolution V2", "CGI", "Text Refine"},
                description = "Choose by source: Standard for most photos, High Fidelity for clean detail, Low Resolution for compressed inputs, CGI for graphics, Text Refine for lettering.",
                show_when = {model = "topaz"}},
            {name = "topaz_upscale_factor", type = "number", label = "Upscale Factor", default = 2,
                min = 1, max = 4, step = 0.25,
                description = "Uniform width/height multiplier from 1x to 4x.",
                show_when = {model = "topaz"}},
            {name = "topaz_subject_detection", type = "select", label = "Subject Detection", default = "All",
                options = {"All", "Foreground", "Background"},
                description = "Where Topaz should concentrate enhancement.",
                show_when = {model = "topaz"}},
            {name = "topaz_face_enhancement", type = "boolean", label = "Face Enhancement", default = true,
                description = "Detect and restore faces separately.",
                show_when = {model = "topaz"}},
            {name = "topaz_face_enhancement_strength", type = "number", label = "Face Strength",
                default = 0.8, min = 0, max = 1, step = 0.05,
                description = "Amount of face restoration; ignored when Face Enhancement is off.",
                show_when = {model = "topaz", topaz_face_enhancement = true}},
            {name = "topaz_face_enhancement_creativity", type = "number", label = "Face Creativity",
                default = 0, min = 0, max = 1, step = 0.05,
                description = "0 preserves identity most strongly; higher values may invent facial detail.",
                show_when = {model = "topaz", topaz_face_enhancement = true}},
            {name = "topaz_fix_compression", type = "number", label = "Fix Compression (optional)",
                min = 0, max = 1, step = 0.05,
                description = "Removes JPEG/block artifacts; leave blank for model defaults. Not supported by CGI.",
                show_when = {model = "topaz"}},
            {name = "topaz_denoise", type = "number", label = "Denoise (optional)",
                min = 0, max = 1, step = 0.05,
                description = "Noise reduction level; leave blank for the selected model's default.",
                show_when = {model = "topaz"}},
            {name = "topaz_sharpen", type = "number", label = "Sharpen (optional)",
                min = 0, max = 1, step = 0.05,
                description = "Sharpening level; leave blank for the selected model's default.",
                show_when = {model = "topaz"}},
            {name = "topaz_text_refine_strength", type = "number", label = "Enhancement Strength",
                default = 0.5, min = 0.01, max = 1, step = 0.01,
                description = "Text/shape enhancement strength; applies only to Text Refine.",
                show_when = {model = "topaz", topaz_model = "Text Refine"}},
            {name = "topaz_output_format", type = "select", label = "Output Format", default = "jpeg",
                options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "topaz"}},

            -- Topaz Generative
            {name = "topaz_generative_model", type = "select", label = "Topaz Model", default = "Wonder 3.5",
                options = {"Wonder 3.5", "Wonder 3", "Wonder 2", "Wonder", "Recover 3",
                           "Standard MAX", "Redefine", "Recovery V2", "Recovery"},
                description = "Wonder 3.5 is the latest realism model; Recover/Recovery rebuild tiny sources; Redefine enables prompt controls.",
                show_when = {model = "topaz_generative"}},
            {name = "topaz_generative_upscale_factor", type = "number", label = "Upscale Factor",
                default = 2, min = 1, max = 4, step = 0.25,
                description = "Uniform width/height multiplier.", show_when = {model = "topaz_generative"}},
            {name = "topaz_generative_subject_detection", type = "select", label = "Subject Detection",
                default = "All", options = {"All", "Foreground", "Background"},
                description = "Used by Wonder 3, Recovery, and Recovery V2; other presets ignore it.",
                show_when = {model = "topaz_generative"}},
            {name = "topaz_generative_face_enhancement", type = "boolean", label = "Face Enhancement",
                default = true, description = "Detect and restore faces separately.",
                show_when = {model = "topaz_generative"}},
            {name = "topaz_generative_face_strength", type = "number", label = "Face Strength",
                default = 0.8, min = 0, max = 1, step = 0.05,
                description = "Face restoration strength; ignored when Face Enhancement is off.",
                show_when = {model = "topaz_generative", topaz_generative_face_enhancement = true}},
            {name = "topaz_generative_face_creativity", type = "number", label = "Face Creativity",
                default = 0, min = 0, max = 1, step = 0.05,
                description = "Higher values invent more facial detail and may reduce identity fidelity.",
                show_when = {model = "topaz_generative", topaz_generative_face_enhancement = true}},
            {name = "topaz_generative_prompt", type = "text", label = "Prompt (Redefine)",
                description = "Optional detail/style guidance; applies only to the Redefine preset.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Redefine"}},
            {name = "topaz_generative_autoprompt", type = "boolean", label = "Automatic Prompt (Redefine)",
                default = true, description = "Let Topaz derive a prompt from the image; applies only to Redefine.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Redefine"}},
            {name = "topaz_generative_creativity", type = "number", label = "Creativity (Redefine)",
                default = 3, min = 1, max = 6, step = 1,
                description = "Higher values hallucinate more new detail; applies only to Redefine.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Redefine"}},
            {name = "topaz_generative_texture", type = "number", label = "Texture (Redefine)",
                default = 3, min = 1, max = 5, step = 1,
                description = "Texture detail level; applies only to Redefine.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Redefine"}},
            {name = "topaz_generative_detail", type = "number", label = "Detail (Recovery V2)",
                default = 0.5, min = 0, max = 1, step = 0.05,
                description = "Detail recovery strength; applies only to Recovery V2.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Recovery V2"}},
            {name = "topaz_generative_enhancement_strength", type = "select", label = "Enhancement Strength",
                default = "auto", options = {"auto", "low", "medium", "high"},
                description = "Overall generative enhancement for Wonder 3/3.5. auto lets Topaz configure it from the image.",
                show_when = {model = "topaz_generative", topaz_generative_model = {"Wonder 3", "Wonder 3.5"}}},
            {name = "topaz_generative_denoise", type = "number", label = "Denoise (Redefine)",
                min = 0, max = 1, step = 0.05, description = "Optional denoise override for Redefine.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Redefine"}},
            {name = "topaz_generative_sharpen", type = "number", label = "Sharpen (Redefine)",
                min = 0, max = 1, step = 0.05, description = "Optional sharpen override for Redefine.",
                show_when = {model = "topaz_generative", topaz_generative_model = "Redefine"}},
            {name = "topaz_generative_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"}, description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "topaz_generative"}},

            -- Topaz Creative
            {name = "topaz_creative_model", type = "select", label = "Bloom Model", default = "Bloom 2",
                options = {"Bloom 2", "Bloom", "Bloom Realism"},
                description = "Bloom 2 is newest and most controllable; Bloom Realism biases invented detail toward photography.",
                show_when = {model = "topaz_creative"}},
            {name = "topaz_creative_upscale_factor", type = "number", label = "Upscale Factor",
                default = 2, min = 1, max = 4, step = 0.25, description = "Uniform width/height multiplier.",
                show_when = {model = "topaz_creative"}},
            {name = "topaz_creative_autoprompt", type = "boolean", label = "Automatic Prompt",
                default = true, description = "Let Topaz describe the source before enhancing it. Accepted by every Bloom preset; documented primarily for Bloom 2.",
                show_when = {model = "topaz_creative"}},
            {name = "topaz_creative_color_preservation", type = "boolean", label = "Preserve Colors",
                default = true, description = "Reduce palette drift. Accepted by every Bloom preset; documented primarily for Bloom 2.",
                show_when = {model = "topaz_creative"}},
            {name = "topaz_creative_creativity", type = "number", label = "Creativity",
                default = 4, min = 1, max = 9, step = 1,
                description = "Higher values reinterpret more of the source. Accepted by every Bloom preset; documented primarily for Bloom 2.",
                show_when = {model = "topaz_creative"}},
            {name = "topaz_creative_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"}, description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "topaz_creative"}},

            -- DRCT super-resolution: degradation-aware (handles JPEG-compressed inputs).
            {name = "drct_upscale_factor", type = "number", label = "Upscale Factor",
                default = 4, min = 1, max = 4, step = 1,
                description = "Uniform width/height multiplier; degradation-aware processing is strongest at higher factors.",
                show_when = {model = "drct"}},

            -- Aura SR: tile-based GAN. v2 checkpoint handles JPEG-degraded inputs better.
            {name = "aura_sr_checkpoint", type = "select", label = "Checkpoint",
                default = "v2", options = {"v1", "v2"},
                description = "v2 is the recommended checkpoint for JPEG-degraded sources; v1 preserves legacy behavior.",
                show_when = {model = "aura_sr"}},
            {name = "aura_sr_upscale_factor", type = "number", label = "Upscale Factor",
                default = 4, min = 1, max = 4, step = 1,
                description = "Uniform width/height multiplier.",
                show_when = {model = "aura_sr"}},
            {name = "aura_sr_overlapping_tiles", type = "boolean", label = "Overlapping Tiles (smoother seams)",
                default = true,
                description = "Blend neighboring tiles to reduce visible boundaries, at some extra work.",
                show_when = {model = "aura_sr"}},

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
            {name = "model", type = "select", label = "Model",
                default = "photo_restoration",
                options = {"photo_restoration", "codeformer", "swin2sr",
                           "nafnet_denoise", "nafnet_deblur", "topaz_restore", "topaz_denoise"},
                description = "Choose by defect: combined aging, faces, generic scenes, compression/noise, blur, or Topaz professional restoration."},

            -- Per-model strengths/weaknesses, gated by show_when so only the
            -- selected model's note appears.
            {name = "model_info_photo_restoration", type = "info",
                label = "Photo Restoration — best for combined fixes",
                description = "Strengths: combines scratch removal, color repair, and resolution enhancement in one pass. Best for a generic 'old damaged photo' input; Topaz Restore is the more specialized alternative for detail recovery or film dust/scratches.\n\nWeaknesses: always upscales to 4K and reshapes to one of {1:1, 16:9, 9:16, 4:3, 3:4}. Sources whose ratio doesn't fall on those five (e.g. 3:2, 21:9) snap to the closest one. Aspect ratio mode 'auto' below picks the closest enum to your source's actual dimensions.",
                show_when = {model = "photo_restoration"}},
            {name = "model_info_codeformer", type = "info",
                label = "CodeFormer — best for old portraits and family photos",
                description = "Strengths: face-focused transformer; preserves aspect ratio exactly. Output dimensions = source × upscale_factor.\n\nWeaknesses: only restores faces well — backgrounds get less attention. No explicit scratch removal or color repair (pair with the Colorize action for color-faded sources). For scenes without faces, prefer SWIN2SR.",
                show_when = {model = "codeformer"}},
            {name = "model_info_swin2sr", type = "info",
                label = "SWIN2SR — best for degraded scenes and non-portrait sources",
                description = "Strengths: generic super-resolution; preserves aspect ratio exactly. The 'real_sr' task is trained on real-world degraded photos — closest fit to 'restore' for landscapes, documents, street photos, etc.\n\nWeaknesses: no face-specific enhancement (use CodeFormer for portraits). No explicit scratch removal or color repair.",
                show_when = {model = "swin2sr"}},
            {name = "model_info_nafnet_denoise", type = "info",
                label = "NAFNet Denoise — best for JPEG / compression artifacts",
                description = "Strengths: targets ISO noise and compression artifacts (JPEG blockiness, ringing, color banding from heavy recompression — e.g. WhatsApp / Instagram screenshots). Pure restoration: no upscale, preserves resolution and aspect ratio exactly.\n\nWeaknesses: doesn't increase resolution — pair with an Upscale action (DRCT or Aura SR v2 are both degradation-aware) if you also need to scale up. Doesn't restore faces specifically (use CodeFormer for that).",
                show_when = {model = "nafnet_denoise"}},
            {name = "model_info_nafnet_deblur", type = "info",
                label = "NAFNet Deblur — best for camera shake and motion blur",
                description = "Strengths: companion to NAFNet Denoise; targets motion blur and out-of-focus softness. Preserves resolution and aspect ratio.\n\nWeaknesses: doesn't address compression artifacts (use NAFNet Denoise for that) or upscale. Run denoise first, then deblur, when both problems are present.",
                show_when = {model = "nafnet_deblur"}},
            {name = "model_info_topaz_restore", type = "info",
                label = "Topaz Restore — old, damaged, or severely degraded photos",
                description = "Recover 3 generatively rebuilds natural detail; Dust-Scratch V2 removes film dust and scratches while staying closer to the source. Output keeps source resolution.",
                show_when = {model = "topaz_restore"}},
            {name = "model_info_topaz_denoise", type = "info",
                label = "Topaz Denoise — high-ISO and night photography",
                description = "Normal, Strong, and Extreme remove progressively more noise; Denoise Max adds generative detail recovery. Output keeps source resolution.",
                show_when = {model = "topaz_denoise"}},

            -- photo_restoration (image-apps-v2): full restoration in one pass.
            -- Always reshapes the output to a 4K image with one of 5 aspect
            -- ratio enums; the "auto" option matches source dims to the closest
            -- enum so the aspect ratio is preserved.
            {name = "fix_colors", type = "boolean", label = "Fix Colors", default = true,
                description = "Correct faded or shifted color in the old photograph.",
                show_when = {model = "photo_restoration"}},
            {name = "remove_scratches", type = "boolean", label = "Remove Scratches", default = true,
                description = "Detect and repair visible scratches and surface damage.",
                show_when = {model = "photo_restoration"}},
            {name = "enhance_resolution", type = "boolean", label = "Enhance Resolution", default = true,
                description = "Enable the model's resolution enhancement; this endpoint still returns a 4K-shaped result when disabled.",
                show_when = {model = "photo_restoration"}},
            {name = "aspect_ratio", type = "select", label = "Output Aspect Ratio",
                default = "auto",
                options = {"auto", "1:1", "16:9", "9:16", "4:3", "3:4"},
                description = "auto chooses the closest supported ratio to the source. Other ratios are not accepted by this endpoint.",
                show_when = {model = "photo_restoration"}},

            -- CodeFormer: face-focused restoration. Preserves aspect ratio
            -- exactly (output dims = input × upscale_factor).
            {name = "codeformer_fidelity", type = "number", label = "Fidelity (identity vs. quality)",
                default = 0.5, min = 0, max = 1, step = 0.05,
                description = "Higher preserves facial identity; lower allows stronger cleanup and reconstruction.",
                show_when = {model = "codeformer"}},
            {name = "codeformer_upscale_factor", type = "number", label = "Upscale Factor",
                default = 2, min = 1, max = 4, step = 1,
                description = "Uniform output scale; aspect ratio is preserved.",
                show_when = {model = "codeformer"}},
            {name = "codeformer_face_upscale", type = "boolean", label = "Upscale Faces", default = true,
                description = "Apply the face-specific upsampler after restoration.",
                show_when = {model = "codeformer"}},
            {name = "codeformer_aligned", type = "boolean", label = "Faces Pre-Aligned", default = false,
                description = "Enable only when the input is already a cropped, aligned face; otherwise detection is skipped incorrectly.",
                show_when = {model = "codeformer"}},
            {name = "codeformer_only_center_face", type = "boolean", label = "Only Center Face", default = false,
                description = "Restore only the central face instead of every detected face.",
                show_when = {model = "codeformer"}},

            -- SWIN2SR: generic super-resolution. Preserves aspect ratio.
            -- "real_sr" is trained on real-world degraded photos and is the
            -- closest fit to a "restore" use case for non-portrait sources.
            {name = "swin2sr_task", type = "select", label = "Task",
                default = "real_sr",
                options = {"real_sr", "classical_sr", "compressed_sr"},
                description = "real_sr fits degraded photos; classical_sr fits clean synthetic downscales; compressed_sr targets JPEG artifacts.",
                show_when = {model = "swin2sr"}},

            -- NAFNet (denoise + deblur share the same `seed` param). Both are
            -- pure restoration: no upscale, aspect ratio preserved exactly.
            {name = "nafnet_seed", type = "number", label = "Seed (optional, for reproducibility)",
                description = "Reuse a random seed to make repeated runs more comparable.",
                show_when = {model = {"nafnet_denoise", "nafnet_deblur"}}},

            {name = "topaz_restore_model", type = "select", label = "Restore Model",
                default = "Recover 3", options = {"Recover 3", "Dust-Scratch V2"},
                description = "Recover 3 rebuilds missing natural detail; Dust-Scratch V2 targets film dust and scratches with less invention.",
                show_when = {model = "topaz_restore"}},
            {name = "topaz_restore_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.", show_when = {model = "topaz_restore"}},
            {name = "topaz_denoise_model", type = "select", label = "Denoise Model",
                default = "Normal", options = {"Normal", "Strong", "Extreme", "Denoise Max"},
                description = "Normal/Strong/Extreme increase removal strength; Denoise Max generatively recovers detail after heavy denoising.",
                show_when = {model = "topaz_denoise"}},
            {name = "topaz_denoise_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.", show_when = {model = "topaz_denoise"}},

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
            {name = "prompt", type = "text", label = "Edit Prompt", required = true,
                description = "Describe the change precisely, including what must stay unchanged."},
            {name = "model", type = "select", label = "Model", default = "flux2",
                options = {"flux2", "flux2pro", "nanobanana2", "nanobananapro",
                           "nanobanana_lite", "gptimage2", "seedream5", "grok2",
                           "muse", "fibo15", "flux1dev"},
                description = "Choose by quality, speed, reference-image support, typography, commercial provenance, and available controls."},

            {name = "model_info_flux2", type = "info", label = "FLUX.2 Turbo — fast multi-reference editing",
                description = "Fast default with up to 4 reference images, prompt expansion, CFG, seed, and WebP support. Best for quick general edits.",
                show_when = {model = "flux2"}},
            {name = "model_info_flux2pro", type = "info", label = "FLUX.2 Pro — premium photorealism",
                description = "Quality-focused FLUX editor with automatic sizing, seed, and a five-level safety tolerance. It supports JPEG/PNG but not WebP.",
                show_when = {model = "flux2pro"}},
            {name = "model_info_nanobanana2", type = "info", label = "Nano Banana 2 — reasoning and current-world edits",
                description = "Google's flexible editor with 0.5K-4K output, optional thinking, web search, system instruction, and multiple references.",
                show_when = {model = "nanobanana2"}},
            {name = "model_info_nanobananapro", type = "info", label = "Nano Banana Pro — high-fidelity composition",
                description = "Strong for complex composition and typography at 1K-4K. Supports web search and a system instruction, but no thinking-level control.",
                show_when = {model = "nanobananapro"}},
            {name = "model_info_nanobanana_lite", type = "info", label = "Nano Banana Lite — speed and cost",
                description = "Sub-2-second target latency for iterative local edits. Supports optional thinking and system instruction, with automatic resolution.",
                show_when = {model = "nanobanana_lite"}},
            {name = "model_info_gptimage2", type = "info", label = "GPT Image 2 — typography and fine detail",
                description = "OpenAI's detailed editor, especially useful for rendered text. Quality drives both detail and cost; background can be transparent with PNG/WebP.",
                show_when = {model = "gptimage2"}},
            {name = "model_info_seedream5", type = "info", label = "Seedream 5.0 Pro — region-precise editing",
                description = "Changes one element while preserving the rest of the frame, with up to 10 references and source-ratio-preserving auto sizes.",
                show_when = {model = "seedream5"}},
            {name = "model_info_grok2", type = "info", label = "Grok Imagine 2.0 — aesthetic multi-image edits",
                description = "Supports up to 3 references, 1K/2K output, and low/medium quality. Auto aspect preserves the first input's ratio.",
                show_when = {model = "grok2"}},
            {name = "model_info_muse", type = "info", label = "Meta Muse — precise, coherent edits",
                description = "Meta's newest editor changes only what is requested, composes up to 10 references, and is strong at fine details such as text and plots.",
                show_when = {model = "muse"}},
            {name = "model_info_fibo15", type = "info", label = "Bria Fibo Edit 1.5 — commercially safe composites",
                description = "Trained for licensed-data workflows, complex object/character combinations, virtual try-on, backgrounds, and style transfer with up to 4 ordered references.",
                show_when = {model = "fibo15"}},
            {name = "model_info_flux1dev", type = "info", label = "FLUX.1 Dev — single-image strength control",
                description = "Older but useful when you need a direct source-strength dial. Accepts only the trigger image; no additional references.",
                show_when = {model = "flux1dev"}},

            -- Flux 2 Turbo
            {name = "flux2_image_size", type = "select", label = "Image Size",
                default = "square_hd",
                options = {"square_hd", "square", "portrait_4_3", "portrait_16_9",
                           "landscape_4_3", "landscape_16_9"},
                description = "Output shape/size preset. square_hd is the high-resolution square preset.",
                show_when = {model = "flux2"}},
            {name = "flux2_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                description = "JPEG is smallest; PNG is lossless; WebP balances size and transparency.",
                show_when = {model = "flux2"}},
            {name = "flux2_guidance_scale", type = "number", label = "Guidance Scale (CFG)",
                default = 2.5, min = 0, max = 20, step = 0.5,
                description = "How strongly the output follows the prompt. Very high values can look harsh or over-constrained.",
                show_when = {model = "flux2"}},
            {name = "flux2_seed", type = "number", label = "Seed (optional)",
                description = "Reuse a random seed for more comparable results.", show_when = {model = "flux2"}},
            {name = "flux2_enable_prompt_expansion", type = "boolean", label = "Expand Prompt",
                default = false, description = "Let FLUX elaborate the instruction for richer detail; disable for literal wording.",
                show_when = {model = "flux2"}},
            {name = "flux2_enable_safety_checker", type = "boolean", label = "Safety Checker",
                default = true, description = "Disable only if your fal.ai account is authorized; otherwise fal.ai still checks the image.",
                show_when = {model = "flux2"}},

            -- Flux 2 Pro
            {name = "flux2pro_image_size", type = "select", label = "Image Size",
                default = "auto",
                options = {"auto", "square_hd", "square", "portrait_4_3", "portrait_16_9",
                           "landscape_4_3", "landscape_16_9"},
                description = "auto lets the model infer output size from the references and instruction.",
                show_when = {model = "flux2pro"}},
            {name = "flux2pro_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "flux2pro"}},
            {name = "flux2pro_safety_tolerance", type = "select", label = "Safety Tolerance",
                default = "5", options = {"1", "2", "3", "4", "5"},
                description = "1 is strictest; 5 is most permissive.",
                show_when = {model = "flux2pro"}},
            {name = "flux2pro_seed", type = "number", label = "Seed (optional)",
                description = "Reuse a random seed for more comparable results.", show_when = {model = "flux2pro"}},
            {name = "flux2pro_enable_safety_checker", type = "boolean", label = "Safety Checker",
                default = true, description = "Disable only if your fal.ai account is authorized; tolerance still controls moderation strictness.",
                show_when = {model = "flux2pro"}},

            -- Nano Banana 2
            {name = "nanobanana2_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "1:1",
                options = {"21:9", "16:9", "3:2", "4:3", "5:4", "1:1",
                           "4:5", "3:4", "2:3", "9:16", "4:1", "1:4", "8:1", "1:8"},
                description = "Output shape, including extreme panorama/strip ratios. Use a source-like ratio to avoid reframing.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_resolution", type = "select", label = "Resolution",
                default = "1K", options = {"0.5K", "1K", "2K", "4K"},
                description = "Long-edge resolution tier. Higher tiers take longer and cost more.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                description = "JPEG is smallest; PNG is lossless; WebP balances size and transparency.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_safety_tolerance", type = "select", label = "Safety Tolerance",
                default = "6", options = {"1", "2", "3", "4", "5", "6"},
                description = "1 blocks the most content; 6 is most permissive.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_seed", type = "number", label = "Seed (optional)",
                description = "Reuse the random starting point for more comparable results.", show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_thinking_level", type = "select", label = "Thinking Level",
                default = "off", options = {"off", "minimal", "high"},
                description = "off disables model thinking; minimal/high can improve complex edits with extra latency and cost.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_system_prompt", type = "text", label = "System Prompt (optional)",
                description = "Persistent style or behavior instruction, separate from the edit prompt.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_enable_web_search", type = "boolean", label = "Enable Web Search",
                default = false, description = "Ground time-sensitive edits in current web information; may add cost.",
                show_when = {model = "nanobanana2"}},
            {name = "nanobanana2_limit_generations", type = "boolean", label = "Limit Prompt-Requested Generations",
                default = true, description = "Ignore instructions asking for multiple/intermediate images so this single-result action stays predictable.",
                show_when = {model = "nanobanana2"}},

            -- Nano Banana Pro. Narrower aspect_ratio enum than Nano Banana 2 (no
            -- 4:1/1:4/8:1/1:8) and no 0.5K resolution.
            {name = "nanobananapro_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "auto",
                options = {"auto", "21:9", "16:9", "3:2", "4:3", "5:4", "1:1",
                           "4:5", "3:4", "2:3", "9:16"},
                description = "auto lets the model decide from references; Pro does not support the extreme strip ratios of Nano Banana 2.",
                show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_resolution", type = "select", label = "Resolution",
                default = "1K", options = {"1K", "2K", "4K"},
                description = "Output tier. Pro starts at 1K; higher tiers take longer and cost more.",
                show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                description = "JPEG is smallest; PNG is lossless; WebP balances size and transparency.",
                show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_safety_tolerance", type = "select", label = "Safety Tolerance",
                default = "6", options = {"1", "2", "3", "4", "5", "6"},
                description = "1 blocks the most content; 6 is most permissive.",
                show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_seed", type = "number", label = "Seed (optional)",
                description = "Reuse the random starting point for more comparable results.", show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_system_prompt", type = "text", label = "System Prompt (optional)",
                description = "Persistent style or behavior instruction, separate from the edit prompt.",
                show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_enable_web_search", type = "boolean", label = "Enable Web Search",
                default = false, description = "Ground time-sensitive edits in current web information; may add cost.",
                show_when = {model = "nanobananapro"}},
            {name = "nanobananapro_limit_generations", type = "boolean", label = "Limit Prompt-Requested Generations",
                default = true, description = "Ignore instructions asking for multiple images so this single-result action stays predictable.",
                show_when = {model = "nanobananapro"}},

            -- Nano Banana Lite
            {name = "nanobanana_lite_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "auto", options = {"auto", "21:9", "16:9", "3:2", "4:3", "5:4", "1:1",
                           "4:5", "3:4", "2:3", "9:16", "4:1", "1:4", "8:1", "1:8"},
                description = "auto lets the model infer shape; extreme strip ratios are supported.",
                show_when = {model = "nanobanana_lite"}},
            {name = "nanobanana_lite_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                description = "JPEG is smallest; PNG is lossless; WebP balances size and transparency.",
                show_when = {model = "nanobanana_lite"}},
            {name = "nanobanana_lite_safety_tolerance", type = "select", label = "Safety Tolerance",
                default = "4", options = {"1", "2", "3", "4", "5", "6"},
                description = "1 blocks the most content; 6 is most permissive.",
                show_when = {model = "nanobanana_lite"}},
            {name = "nanobanana_lite_seed", type = "number", label = "Seed (optional)",
                description = "Reuse the random starting point for more comparable results.",
                show_when = {model = "nanobanana_lite"}},
            {name = "nanobanana_lite_thinking_level", type = "select", label = "Thinking Level",
                default = "off", options = {"off", "minimal", "high"},
                description = "off disables thinking; minimal/high can improve complex edits with extra latency and cost.",
                show_when = {model = "nanobanana_lite"}},
            {name = "nanobanana_lite_system_prompt", type = "text", label = "System Prompt (optional)",
                description = "Persistent style or behavior instruction, separate from the edit prompt.",
                show_when = {model = "nanobanana_lite"}},
            {name = "nanobanana_lite_limit_generations", type = "boolean", label = "Limit Prompt-Requested Generations",
                default = true, description = "Ignore instructions asking for multiple images so this single-result action stays predictable.",
                show_when = {model = "nanobanana_lite"}},

            -- GPT Image 2. Sized by an image_size enum rather than aspect ratio +
            -- resolution; `quality` drives both detail and cost (billed per token).
            {name = "gptimage2_image_size", type = "select", label = "Image Size",
                default = "auto",
                options = {"auto", "square_hd", "square", "portrait_4_3", "portrait_16_9",
                           "landscape_4_3", "landscape_16_9"},
                description = "auto infers size from references; presets force a specific output shape.",
                show_when = {model = "gptimage2"}},
            {name = "gptimage2_quality", type = "select", label = "Quality",
                default = "high", options = {"auto", "low", "medium", "high"},
                description = "Detail/cost tier. high is most detailed and most expensive; auto lets the model choose.",
                show_when = {model = "gptimage2"}},
            {name = "gptimage2_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                description = "Use PNG or WebP for transparency; JPEG is smaller and always opaque.",
                show_when = {model = "gptimage2"}},
            {name = "gptimage2_background", type = "select", label = "Background",
                default = "auto", options = {"auto", "transparent", "opaque"},
                description = "transparent requires PNG or WebP; opaque forces a filled background.",
                show_when = {model = "gptimage2"}},

            -- Seedream 5.0 Pro. auto_1K / auto_2K keep the source's aspect ratio and
            -- only set the target area; the fixed enums reshape to that ratio.
            {name = "seedream5_image_size", type = "select", label = "Image Size",
                default = "auto_2K",
                options = {"auto_2K", "auto_1K", "square_hd", "square", "portrait_4_3",
                           "portrait_16_9", "landscape_4_3", "landscape_16_9"},
                description = "auto_1K/auto_2K preserve the first source's ratio; fixed presets reshape to their named ratio.",
                show_when = {model = "seedream5"}},
            {name = "seedream5_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "seedream5"}},
            {name = "seedream5_enable_safety_checker", type = "boolean", label = "Safety Checker",
                default = false,
                description = "Enable fal.ai moderation. Unauthorized accounts may be checked even when this is off.",
                show_when = {model = "seedream5"}},

            -- Grok Imagine Image 2.0. Its own aspect_ratio enum, lowercase
            -- resolutions, and a low|medium quality dial.
            {name = "grok2_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "auto",
                options = {"auto", "2:1", "20:9", "19.5:9", "16:9", "4:3", "3:2", "1:1",
                           "2:3", "3:4", "9:16", "9:19.5", "9:20", "1:2"},
                description = "auto preserves the first input's aspect ratio; fixed values reshape the result.",
                show_when = {model = "grok2"}},
            {name = "grok2_resolution", type = "select", label = "Resolution",
                default = "1k", options = {"1k", "2k"},
                description = "1k is standard; 2k is the higher-resolution tier.",
                show_when = {model = "grok2"}},
            {name = "grok2_quality", type = "select", label = "Quality",
                default = "medium", options = {"low", "medium"},
                description = "medium is Grok's maximum quality; low is faster/cheaper.",
                show_when = {model = "grok2"}},
            {name = "grok2_output_format", type = "select", label = "Output Format",
                default = "png", options = {"jpeg", "png", "webp"},
                description = "JPEG is smallest; PNG is lossless; WebP balances size and transparency.",
                show_when = {model = "grok2"}},

            -- Meta Muse
            {name = "muse_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "auto", options = {"auto", "21:9", "16:9", "4:3", "3:2", "1:1", "2:3", "3:4", "9:16", "9:21"},
                description = "auto lets Muse choose dimensions from the request; otherwise force one of its nine supported ratios.",
                show_when = {model = "muse"}},
            {name = "muse_output_format", type = "select", label = "Output Format",
                default = "webp", options = {"jpeg", "png", "webp"},
                description = "WebP is Muse's compact default; PNG is lossless; JPEG is broadly compatible.",
                show_when = {model = "muse"}},

            -- Bria Fibo Edit 1.5
            {name = "fibo15_aspect_ratio", type = "select", label = "Aspect Ratio",
                default = "auto", options = {"auto", "1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9"},
                description = "auto keeps the first reference's ratio. A chosen ratio only applies when two or more references are sent.",
                show_when = {model = "fibo15"}},
            {name = "fibo15_seed", type = "number", label = "Seed",
                default = 5555, description = "Reuse the same seed and ordered references for more reproducible edits.",
                show_when = {model = "fibo15"}},

            -- Flux 1 Dev (image-to-image)
            {name = "strength", type = "number", label = "Strength", default = 0.95,
                min = 0.01, max = 1.0, step = 0.01,
                description = "How much the source may change. The live endpoint recommends high values and defaults to 0.95.",
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_num_inference_steps", type = "number", label = "Inference Steps",
                default = 40, min = 10, max = 50, step = 1,
                description = "More steps can refine detail but take longer; the endpoint accepts 10-50.",
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_guidance_scale", type = "number", label = "Guidance Scale (CFG)",
                default = 3.5, min = 1, max = 20, step = 0.5,
                description = "Prompt adherence. Very high values can over-constrain the image.",
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_acceleration", type = "select", label = "Acceleration",
                default = "none", options = {"none", "regular", "high"},
                description = "Faster modes reduce latency and may trade some quality or reproducibility.",
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_seed", type = "number", label = "Seed (optional)",
                description = "Reuse a seed for more reproducible results.", show_when = {model = "flux1dev"}},
            {name = "flux1dev_enable_safety_checker", type = "boolean", label = "Safety Checker",
                default = true, description = "Disable only if your fal.ai account is authorized.",
                show_when = {model = "flux1dev"}},
            {name = "flux1dev_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"}, description = "JPEG is smaller; PNG is lossless.",
                show_when = {model = "flux1dev"}},

            {
                name = "extra_images", type = "entity_ref", entity = "resource",
                label = "Additional Images", multi = true,
                min = 0, max = 9,
                default = "trigger",
                description = "Ordered reference images. Flux 1 Dev ignores them; Grok allows 3; FLUX.2 Turbo and Fibo allow 4; Seedream and Muse allow 10. The action reports an over-limit count where fal.ai would otherwise truncate or reject it.",
                show_when = { model = {"flux2", "flux2pro", "nanobanana2", "nanobananapro",
                                       "nanobanana_lite", "gptimage2", "seedream5", "grok2",
                                       "muse", "fibo15"} },
                filters = { content_types = IMAGE_CONTENT_TYPES },
            },

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
        params = {
            {name = "model_info_vectorize", type = "info", label = "Recraft Vectorize — raster to SVG",
                description = "Traces the source into editable vector paths. Best for logos, icons, flat graphics, and clean illustrations; photographs produce much more complex SVGs."},
        },
        handler = make_handler("vectorize"),
    })

    -- Polish: detail only (post-processing finishing pass — typically run after a denoise / restore)
    mah.action({
        id = "polish",
        label = "Polish",
        description = "Sharpen the image (post-processing finishing pass, typically after Restore)",
        icon = "sparkles",
        entity = "resource",
        placement = {"detail"},
        async = true,
        filters = { content_types = IMAGE_CONTENT_TYPES },
        params = {
            {name = "model", type = "select", label = "Model", default = "post_processing",
                options = {"post_processing", "topaz_sharpen"},
                description = "Use lightweight configurable sharpening or Topaz's blur-specific professional models."},
            {name = "model_info_post_processing", type = "info", label = "Post Processing — configurable finishing pass",
                description = "Low-cost basic, edge-aware smart, or contrast-adaptive sharpening. Best after denoise when the image is already structurally sound.",
                show_when = {model = "post_processing"}},
            {name = "model_info_topaz_sharpen", type = "info", label = "Topaz Sharpen — match the blur type",
                description = "Choose a preset for lens blur, motion blur, portraits, wildlife, or severe defocus. Super Focus V3 is generative and best reserved for badly blurred inputs.",
                show_when = {model = "topaz_sharpen"}},
            {name = "sharpen_mode", type = "select", label = "Sharpen Mode",
                default = "smart",
                options = {"basic", "smart", "cas"},
                description = "basic is direct radius/strength; smart protects edges; cas is contrast-adaptive.",
                show_when = {model = "post_processing"}},

            -- basic
            {name = "sharpen_radius", type = "number", label = "Radius",
                default = 1, min = 1, max = 10, step = 1,
                description = "Blur radius used to find detail edges; larger values affect broader features.",
                show_when = {model = "post_processing", sharpen_mode = "basic"}},
            {name = "sharpen_alpha", type = "number", label = "Strength",
                default = 1, min = 0, max = 5, step = 0.1,
                description = "Basic sharpen intensity.",
                show_when = {model = "post_processing", sharpen_mode = "basic"}},

            -- smart (good default for post-denoise)
            {name = "noise_radius", type = "number", label = "Noise Radius",
                default = 7, min = 1, max = 20, step = 1,
                description = "Neighborhood used to distinguish noise from real structure.",
                show_when = {model = "post_processing", sharpen_mode = "smart"}},
            {name = "preserve_edges", type = "number", label = "Preserve Edges",
                default = 0.75, min = 0, max = 1, step = 0.05,
                description = "Higher values protect strong edges from halos and oversharpening.",
                show_when = {model = "post_processing", sharpen_mode = "smart"}},
            {name = "smart_sharpen_strength", type = "number", label = "Strength",
                default = 5, min = 0, max = 20, step = 0.5,
                description = "Smart sharpen intensity.",
                show_when = {model = "post_processing", sharpen_mode = "smart"}},
            {name = "smart_sharpen_ratio", type = "number", label = "Ratio",
                default = 0.5, min = 0, max = 1, step = 0.05,
                description = "Balance between fine-detail sharpening and edge protection.",
                show_when = {model = "post_processing", sharpen_mode = "smart"}},

            -- cas (Contrast-Adaptive Sharpen)
            {name = "cas_amount", type = "number", label = "CAS Amount",
                default = 0.8, min = 0, max = 1, step = 0.05,
                description = "Contrast-Adaptive Sharpen intensity; high values can produce halos.",
                show_when = {model = "post_processing", sharpen_mode = "cas"}},

            {name = "topaz_sharpen_model", type = "select", label = "Sharpen Model",
                default = "Auto Sharpen",
                options = {"Standard", "Strong", "Lens Blur V2", "Motion Blur", "Natural",
                           "Refocus", "Wildlife", "Portrait", "Auto Sharpen", "Super Focus V3", "Super Focus V2"},
                description = "Auto Sharpen chooses broadly; match Lens/Motion/Portrait/Wildlife when known. Super Focus generatively reconstructs severe blur.",
                show_when = {model = "topaz_sharpen"}},
            {name = "topaz_sharpen_output_format", type = "select", label = "Output Format",
                default = "jpeg", options = {"jpeg", "png"},
                description = "JPEG is smaller; PNG is lossless.", show_when = {model = "topaz_sharpen"}},

            OUTPUT_MODE_PARAM,
        },
        handler = make_handler("polish"),
    })

    mah.log("info", "[fal.ai] init: registered 7 actions (colorize, adjust, upscale, restore, edit, vectorize, polish)")

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
            -- Every value below is clamped to what the form can offer before it
            -- reaches a builder; see GENERATE_ASPECT_RATIOS and friends.
            local model = params.model or "nanobanana2"
            local aspect_ratio = one_of(params.aspect_ratio, GENERATE_ASPECT_RATIOS, "1:1")
            local resolution = one_of(params.resolution, GENERATE_RESOLUTIONS, "1K")
            -- safety_tolerance arrives as a form string ("1".."6"); fal.ai expects a string enum here.
            local safety_tolerance = one_of(params.safety_tolerance, GENERATE_SAFETY, "6")
            local output_format = one_of(params.output_format, GENERATE_OUTPUT_FORMATS, "jpeg")
            local quality = one_of(params.quality, GENERATE_QUALITY, "high")
            local background = one_of(params.background, GENERATE_BACKGROUND, "auto")
            local thinking_level = one_of(params.thinking_level, GENERATE_THINKING, "off")
            local style_preset = one_of(params.style_preset, GENERATE_STYLE_PRESETS, "No Style")
            local seed = tonumber(params.seed)
            if seed then seed = math.floor(seed) end

            mah.log("info", "[fal.ai] generate page: starting async job, model=" .. model .. ", prompt=" .. prompt:sub(1, 100) .. ", safety=" .. safety_tolerance)

            -- Start async job and return immediately
            local job_id = mah.start_job("Generate: " .. prompt:sub(1, 40), function(jid)
                mah.job_progress(jid, 10, "Preparing request...")

                local spec = generate_model_by_id(model)
                local endpoint = spec.endpoint
                local payload = spec.build({
                    prompt = prompt,
                    aspect_ratio = aspect_ratio,
                    resolution = resolution,
                    safety_tolerance = safety_tolerance,
                    output_format = output_format,
                    quality = quality,
                    background = background,
                    seed = seed,
                    thinking_level = thinking_level,
                    system_prompt = params.system_prompt or "",
                    enable_web_search = params.enable_web_search == "true",
                    style_preset = style_preset,
                })

                mah.log("info", "[fal.ai] generate job: endpoint=" .. endpoint
                    .. ", aspect_ratio=" .. tostring(payload.aspect_ratio)
                    .. ", image_size=" .. tostring(payload.image_size)
                    .. ", resolution=" .. tostring(payload.resolution))

                mah.job_progress(jid, 20, "Submitting to fal.ai...")

                local ok, result_or_err = pcall(fal_submit_and_wait, endpoint, payload, api_key, jid, 20, 70)
                if not ok then
                    mah.job_fail(jid, tostring(result_or_err))
                    return
                end
                local result = result_or_err

                local result_url = get_result_url(result)
                if not result_url then
                    mah.job_fail(jid, "No image URL in API response")
                    return
                end

                mah.job_progress(jid, 85, "Saving result...")

                local safe_prompt = prompt:gsub("[^%w%s_-]", ""):gsub("%s+", "_"):sub(1, 40)
                local extension
                if spec.extension ~= nil then
                    extension = spec.extension
                else
                    extension = ({jpeg = "jpg", png = "png", webp = "webp"})[payload.output_format] or "jpg"
                end
                local filename = "generated_" .. safe_prompt
                    .. (extension ~= "" and ("." .. extension) or "")

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
        description = "Colorize black-and-white images with DDColor or Topaz Colorize.",
        category = "Action",
        attrs = {
            { name = "model", type = "select", default = "ddcolor", description = "ddcolor is fast and simple; topaz_colorize is Topaz Labs' current professional archival colorization model." },
            { name = "topaz_colorize_output_format", type = "select", default = "jpeg", description = "JPEG or lossless PNG (Topaz only)." },
        },
        notes = {
            "Best results with grayscale photographs.",
            "Result is added as a new version of the original resource.",
            "Available from both detail view and card view.",
        },
    })

    mah.doc({
        name = "adjust",
        label = "Adjust Color & Lighting",
        description = "Correct exposure, lighting, or white balance with Topaz Labs.",
        category = "Action",
        attrs = {
            { name = "model", type = "select", default = "adjust_v2", description = "adjust_v2 fixes exposure/lighting; white_balance removes color casts." },
            { name = "output_format", type = "select", default = "jpeg", description = "JPEG or lossless PNG." },
        },
        notes = {
            "Uses topaz/adjust/image and preserves source resolution.",
            "Available from both detail view and card view.",
        },
    })

    mah.doc({
        name = "upscale",
        label = "Upscale",
        description = "Increase image resolution using AI upscaling models.",
        category = "Action",
        attrs = {
            { name = "model", type = "select", default = "clarity", description = "Backends: clarity, crystal, esrgan, creative, seedvr, bria_creative, topaz (Precision), topaz_generative, topaz_creative, topaz_transparent, drct, aura_sr." },
            { name = "clarity_*", type = "various", description = "Clarity controls: prompt, negative_prompt, upscale_factor, creativity, resemblance, guidance_scale, num_inference_steps (shown when model=clarity)" },
            { name = "crystal_*", type = "various", description = "Crystal controls: scale_factor, creativity (0 = faithful upscale, up to 10), output_format (shown when model=crystal)" },
            { name = "esrgan_*", type = "various", description = "ESRGAN controls: esrgan_model variant, scale, face mode, output_format (shown when model=esrgan)" },
            { name = "creative_*", type = "various", description = "Creative Upscaler controls: prompt, scale, creativity, detail, shape_preservation (shown when model=creative)" },
            { name = "seedvr_*", type = "various", description = "SeedVR controls: upscale_mode (factor|target), upscale_factor or target_resolution, noise_scale, output_format, seamless (switches to the seamless-tiling endpoint) (shown when model=seedvr)" },
            { name = "bria_preserve_alpha", type = "boolean", default = "true", description = "Preserve alpha channel (shown when model=bria_creative)" },
            { name = "topaz_*", type = "various", description = "Topaz Precision exposes preset, scale, subject/face handling, compression/denoise/sharpen overrides, and format. Generative exposes Wonder/Recover/Redefine controls; Creative exposes Bloom controls; Transparent is fixed 4x PNG." },
            { name = "drct_upscale_factor", type = "number", default = "4", description = "DRCT upscale factor 1-4 (shown when model=drct)" },
            { name = "aura_sr_*", type = "various", description = "Aura SR controls: checkpoint (v1|v2, default v2), upscale_factor, overlapping_tiles (shown when model=aura_sr)" },
        },
        examples = {
            { title = "Clarity Upscaler (default)", code = "Uses prompt-guided upscaling with quality-focused defaults.", notes = "Model: fal-ai/clarity-upscaler" },
            { title = "Crystal Upscaler", code = "Clarity AI's newer upscaler, tuned for facial detail and portrait photography. Leave creativity at 0 for a faithful upscale.", notes = "Model: clarityai/crystal-upscaler" },
            { title = "ESRGAN", code = "4x upscaling with RealESRGAN_x4plus model.", notes = "Model: fal-ai/esrgan" },
            { title = "Creative Upscaler", code = "AI-enhanced upscaling with creative interpretation.", notes = "Model: fal-ai/creative-upscaler" },
            { title = "SeedVR", code = "High-quality upscaling with SeedVR model. Enable Seamless Tiling for textures and repeating patterns.", notes = "Models: fal-ai/seedvr/upscale/image, fal-ai/seedvr/upscale/image/seamless" },
            { title = "Bria Creative", code = "Creative upscaling by Bria AI.", notes = "Model: bria/upscale/creative" },
            { title = "Topaz Precision", code = "Faithful professional upscaling with source-specific presets.", notes = "Model: topaz/upscale/image/precision" },
            { title = "Topaz Generative", code = "Rebuild missing detail with Wonder, Recover, Recovery, or prompt-guided Redefine.", notes = "Model: topaz/upscale/image/generative" },
            { title = "Topaz Creative / Transparent", code = "Bloom creatively enhances AI art; Transparent preserves alpha in a fixed 4x PNG.", notes = "Models: topaz/upscale/image/creative, topaz/upscale/image/transparent" },
            { title = "DRCT", code = "Degradation-aware super-resolution; handles JPEG-compressed sources better than pure-SR models.", notes = "Model: fal-ai/drct-super-resolution" },
            { title = "Aura SR", code = "Tile-based 4x GAN. Use checkpoint=v2 for JPEG-degraded inputs.", notes = "Model: fal-ai/aura-sr" },
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
            { name = "model", type = "select", default = "photo_restoration", description = "Backends: photo_restoration (combined aging), codeformer (faces), swin2sr (scenes), NAFNet (noise/blur), Topaz Restore (damage/detail), and Topaz Denoise (high ISO/night)." },
            { name = "fix_colors / remove_scratches / enhance_resolution / aspect_ratio", type = "various", description = "photo_restoration controls. aspect_ratio defaults to 'auto', which picks the closest enum to the source's actual dimensions. Other options: 1:1, 16:9, 9:16, 4:3, 3:4. (Shown when model=photo_restoration.)" },
            { name = "codeformer_*", type = "various", description = "CodeFormer controls: fidelity (identity vs. quality, 0-1), upscale_factor (1-4), face_upscale, aligned, only_center_face. (Shown when model=codeformer.)" },
            { name = "swin2sr_task", type = "select", default = "real_sr", description = "SWIN2SR task: real_sr (degraded real-world photos), classical_sr, or compressed_sr. (Shown when model=swin2sr.)" },
            { name = "nafnet_seed", type = "number", description = "Optional seed for nafnet_denoise / nafnet_deblur reproducibility." },
            { name = "topaz_restore_* / topaz_denoise_*", type = "various", description = "Topaz preset and JPEG/PNG format. Recover 3 rebuilds detail, Dust-Scratch V2 repairs film damage; Denoise presets range from Normal to generative Denoise Max." },
        },
        examples = {
            { title = "Photo restoration (default)", code = "Combined color/scratch/quality fix in one pass.", notes = "Model: fal-ai/image-apps-v2/photo-restoration. Always reshapes to a 4K aspect_ratio enum; 'auto' matches the source's actual ratio." },
            { title = "Old portrait", code = "Use CodeFormer with fidelity 0.5-0.7 for old family photos with faces.", notes = "Model: fal-ai/codeformer. Preserves aspect ratio exactly." },
            { title = "Old scene/landscape", code = "Use SWIN2SR with task=real_sr for degraded non-portrait photos.", notes = "Model: fal-ai/swin2sr. Preserves aspect ratio exactly." },
            { title = "JPEG / compression artifacts", code = "Use NAFNet Denoise on heavily-recompressed images (WhatsApp / Instagram screenshots, social-media downloads).", notes = "Model: fal-ai/nafnet/denoise. Pure restoration — preserves resolution and aspect ratio. Pair with an Upscale step (DRCT or Aura SR v2) if you also need to scale up." },
            { title = "Motion blur / camera shake", code = "Use NAFNet Deblur. Run after Denoise if both blur and compression artifacts are present.", notes = "Model: fal-ai/nafnet/deblur. Preserves resolution and aspect ratio." },
            { title = "Topaz professional restore", code = "Use Recover 3 for missing natural detail or Dust-Scratch V2 for archival film damage.", notes = "Model: topaz/restore/image" },
            { title = "Topaz high-ISO denoise", code = "Choose Normal, Strong, Extreme, or generative Denoise Max by noise severity.", notes = "Model: topaz/denoise/image" },
        },
        notes = {
            "photo_restoration combines scratches, color fading, and resolution repair, but always 4K-reshapes to one of {1:1, 16:9, 9:16, 4:3, 3:4}. 'auto' picks the closest enum to the source. Topaz Restore instead keeps source resolution and specializes in natural-detail recovery or dust/scratch cleanup.",
            "codeformer and swin2sr both preserve aspect ratio exactly but don't do explicit color/scratch repair — they're denoise + super-resolution. Pair them with the Colorize action if the source is also color-faded.",
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
            { name = "model", type = "select", default = "flux2", description = "Models: FLUX 2 Turbo/Pro, Nano Banana 2/Pro/Lite, GPT Image 2, Seedream 5 Pro, Grok Imagine 2, Meta Muse, Bria Fibo Edit 1.5, and FLUX 1 Dev." },
            { name = "flux2_* / flux2pro_*", type = "various", description = "FLUX controls include size/format, seed, safety checker, plus Turbo prompt expansion + CFG or Pro safety tolerance." },
            { name = "nanobanana*", type = "various", description = "Nano Banana controls include aspect/format/safety, seed, system prompt, generation limiting, optional thinking (2/Lite), resolution and web search where supported." },
            { name = "gptimage2_*", type = "various", description = "GPT Image 2 exposes size, quality, format, and background. Quality drives detail/cost; transparent background needs PNG/WebP." },
            { name = "seedream5_image_size / seedream5_output_format / seedream5_enable_safety_checker", type = "various", description = "Seedream 5.0 Pro controls (shown when model=seedream5). image_size auto_1K / auto_2K keep the source's aspect ratio; safety is a boolean, not a tolerance." },
            { name = "grok2_aspect_ratio / grok2_resolution / grok2_quality / grok2_output_format", type = "various", description = "Grok Imagine Image 2.0 controls (shown when model=grok2). Own aspect_ratio enum, lowercase '1k'/'2k' resolution, quality low|medium. No safety_tolerance in the schema." },
            { name = "muse_* / fibo15_*", type = "various", description = "Muse exposes aspect and output format with up to 10 references. Fibo exposes aspect and seed with up to 4 ordered references." },
            { name = "strength", type = "number", default = "0.95", description = "Edit strength 0.01-1.0 (shown when model=flux1dev)." },
            { name = "flux1dev_num_inference_steps / flux1dev_guidance_scale / flux1dev_acceleration", type = "various", description = "Flux 1 Dev controls (shown when model=flux1dev). safety_tolerance is not in the schema for this endpoint." },
            { name = "extra_images", type = "entity_ref", description = "Additional resource IDs sent alongside the source. Every model except Flux 1 Dev uses these. Defaults to the trigger resource (the source image) — picker lets the user add more or remove the source." },
        },
        examples = {
            { title = "Change background", code = 'Prompt: "change the background to a sunset beach"' },
            { title = "Style transfer", code = 'Prompt: "make it look like a watercolor painting"' },
            { title = "Region-precise edit", code = 'Use Seedream 5.0 Pro to change one element while the rest of the frame stays intact.', notes = "Model: bytedance/seedream/v5/pro/edit" },
            { title = "Typography and fine detail", code = 'Use GPT Image 2 when the edit involves rendered text or fine-grained detail.', notes = "Model: openai/gpt-image-2/edit" },
        },
        notes = {
            "Result is added as a new version of the original resource.",
            "Available from detail view only.",
            "All models except Flux 1 Dev accept multiple input images via the 'Additional Images' picker. The trigger image is included by default.",
            "The picker's nine-image maximum is shared by every model. The request builder reports explicit over-limit errors for Grok (3), FLUX.2 Turbo and Fibo (4); Seedream and Muse accept 10, while GPT Image 2 accepts 16 (above the picker's limit).",
            "Flux 1 Dev accepts only a single input image and supports a strength parameter.",
        },
    })

    mah.doc({
        name = "polish",
        label = "Polish",
        description = "Sharpen an image as a finishing pass. Useful after a Restore (NAFNet Denoise especially) to recover detail that the denoise step softened.",
        category = "Action",
        attrs = {
            { name = "model", type = "select", default = "post_processing", description = "post_processing provides configurable basic/smart/CAS sharpening; topaz_sharpen provides blur-specific professional presets and generative Super Focus." },
            { name = "sharpen_mode", type = "select", default = "smart", description = "basic, smart, or CAS; shown only when model=post_processing." },
            { name = "sharpen_radius / sharpen_alpha", type = "various", description = "Basic-mode controls; shown when model=post_processing and sharpen_mode=basic." },
            { name = "noise_radius / preserve_edges / smart_sharpen_strength / smart_sharpen_ratio", type = "various", description = "Smart-mode controls; shown when model=post_processing and sharpen_mode=smart." },
            { name = "cas_amount", type = "number", default = "0.8", description = "CAS strength 0-1; shown when model=post_processing and sharpen_mode=cas." },
            { name = "topaz_sharpen_model", type = "select", default = "Auto Sharpen", description = "Blur-specific Topaz preset, including Lens Blur, Motion Blur, Portrait, Wildlife, and generative Super Focus; shown when model=topaz_sharpen." },
            { name = "topaz_sharpen_output_format", type = "select", default = "jpeg", description = "JPEG or lossless PNG; shown when model=topaz_sharpen." },
        },
        notes = {
            "The configurable mode uses fal-ai/post-processing's sharpen group. Topaz mode uses topaz/sharpen/image and keeps source resolution.",
            "Recommended workflow: Restore (NAFNet Denoise) → Polish (smart mode) → optional Upscale.",
            "Result is added as a new version of the original resource by default.",
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
            { name = "model", type = "select", default = "nanobanana2", description = "Models: Nano Banana 2/Pro/Lite, GPT Image 2, Seedream 5 Pro, Grok Imagine 2, Meta Muse, and Bria Fibo Gen 1.5." },
            { name = "resolution", type = "select", default = "1K", description = "0.5K-4K union. Values map to each model's nearest native tier; Lite/Muse/GPT/Seedream auto-size, and Fibo maps to 1MP/4MP." },
            { name = "aspect_ratio", type = "select", default = "1:1", description = "Shared aspect union; GPT/Seedream receive the closest image_size preset." },
            { name = "safety_tolerance", type = "select", default = "6", description = "1 strictest to 6 most permissive. Nano Banana receives it; Seedream maps 1-2 to its boolean checker; other models ignore it." },
            { name = "output_format / seed / quality / background", type = "various", description = "Format is mapped to model support; seed is sent where supported; quality/background apply to GPT Image 2 and quality also maps to Grok." },
            { name = "thinking_level / system_prompt / enable_web_search", type = "various", description = "Advanced Nano Banana controls. The inline form says exactly which family members accept each field." },
            { name = "style_preset", type = "select", default = "No Style", description = "Bria Fibo Gen 1.5 style preset: No Style or Photoreal." },
        },
        examples = {
            { title = "Basic generation", code = 'Prompt: "a serene mountain landscape at golden hour"' },
            { title = "Text in the image", code = 'Use GPT Image 2 or Seedream 5.0 Pro when the image has to contain readable text.' },
        },
        notes = {
            "Accessible via the Generate Image menu item.",
            "Uses asynchronous job processing; track progress with Ctrl+Shift+D.",
            "Generated images are saved as new resources.",
            "The form is a shared union of controls; each model receives only the fields its own schema declares, with the values mapped as described above.",
        },
    })

    mah.log("info", "[fal.ai] init: plugin fully initialized")
end

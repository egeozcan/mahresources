plugin = {
    name = "example-blocks",
    version = "1.0",
    description = "Example plugin demonstrating custom block types"
}

plugin.settings = {}

function init()
    mah.block_type({
        type = "counter",
        label = "Counter",
        icon = "🔢",
        description = "A simple click counter block",

        content_schema = {
            type = "object",
            properties = {
                label = { type = "string" }
            },
            required = { "label" }
        },

        state_schema = {
            type = "object",
            properties = {
                count = { type = "number" }
            }
        },

        default_content = { label = "My Counter" },
        default_state = { count = 0 },

        render_view = function(ctx)
            local count = ctx.block.state.count or 0
            local label = ctx.block.content.label or "Counter"
            local blockId = ctx.block.id

            return string.format([[
                <div style="text-align:center; padding:20px;">
                    <h3 style="margin:0 0 10px 0;">%s</h3>
                    <div style="font-size:2em; font-weight:bold; margin:10px 0;">%d</div>
                    <button onclick="mahBlock.updateState(%d, {count: %d})"
                            style="padding:8px 16px; background:#3b82f6; color:white; border:none; border-radius:4px; cursor:pointer;">
                        +1
                    </button>
                </div>
            ]], label, count, blockId, count + 1)
        end,

        render_edit = function(ctx)
            local label = ctx.block.content.label or "Counter"
            local blockId = ctx.block.id

            return string.format([[
                <div style="padding:10px;">
                    <label style="display:block; margin-bottom:4px; font-weight:500;">Counter Label</label>
                    <input type="text" value="%s"
                           onchange="mahBlock.saveContent(%d, {label: this.value})"
                           style="width:100%%; padding:8px; border:1px solid #d1d5db; border-radius:4px;" />
                </div>
            ]], label, blockId)
        end
    })
end

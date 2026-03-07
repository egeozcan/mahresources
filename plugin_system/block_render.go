package plugin_system

import (
	"context"
	"fmt"
	"log"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const luaBlockRenderTimeout = 5 * time.Second

// BlockRenderData holds block data for the render context.
type BlockRenderData struct {
	ID       uint           `json:"id"`
	Content  map[string]any `json:"content"`
	State    map[string]any `json:"state"`
	Position string         `json:"position"`
}

// NoteRenderData holds note data for the render context.
type NoteRenderData struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	NoteTypeID uint   `json:"note_type_id"`
}

// BlockRenderContext holds all context passed to the Lua render function.
type BlockRenderContext struct {
	Block    BlockRenderData `json:"block"`
	Note     NoteRenderData  `json:"note"`
	Settings map[string]any  `json:"settings"`
}

// RenderBlock executes the Lua render function for a plugin block type
// and returns the rendered HTML string.
func (pm *PluginManager) RenderBlock(pluginName, fullTypeName, mode string, ctx BlockRenderContext) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	pbt := pm.GetPluginBlockType(fullTypeName)
	if pbt == nil {
		return "", fmt.Errorf("block type %q not found", fullTypeName)
	}
	if pbt.PluginName != pluginName {
		return "", fmt.Errorf("block type %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	var fn *lua.LFunction
	switch mode {
	case "view":
		fn = pbt.RenderView
	case "edit":
		fn = pbt.RenderEdit
	default:
		return "", fmt.Errorf("invalid render mode %q: must be 'view' or 'edit'", mode)
	}
	if fn == nil {
		return "", fmt.Errorf("no render_%s function for block type %q", mode, fullTypeName)
	}

	L := pbt.State
	mu := pm.VMLock(L)
	if mu == nil {
		return "", fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	mu.Lock()
	defer mu.Unlock()

	// Build context table
	ctxData := map[string]any{
		"block": map[string]any{
			"id":       ctx.Block.ID,
			"content":  ctx.Block.Content,
			"state":    ctx.Block.State,
			"position": ctx.Block.Position,
		},
		"note": map[string]any{
			"id":           ctx.Note.ID,
			"name":         ctx.Note.Name,
			"note_type_id": ctx.Note.NoteTypeID,
		},
	}
	if ctx.Settings != nil {
		ctxData["settings"] = ctx.Settings
	} else {
		ctxData["settings"] = map[string]any{}
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaBlockRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: block render %q/%q returned error: %v", pluginName, fullTypeName, err)
		return "", fmt.Errorf("block render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", nil
}

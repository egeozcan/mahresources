package plugin_system

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	lua "github.com/yuin/gopher-lua"
)

// maxStreamedResourceFileSize caps mah.db.each_resource_json's file. Memory is
// bounded per piece (below), so this bounds how long a call can run.
const maxStreamedResourceFileSize = 1 << 30 // 1 GiB

// maxStreamedElementSize caps the bytes of one piece -- an array element, or a
// top-level member with its key. A piece is decoded whole into Go values, at
// up to about forty times its size for something like "[{},{},...]", so
// without this a single huge element in a small-enough file could exhaust the
// server's memory. Real export entries are kilobytes.
const maxStreamedElementSize = 4 << 20 // 4 MiB

// maxStreamedElementValues caps the values in one piece. Bytes alone do not
// bound the Lua side: every table costs on the order of a hundred bytes however
// small its JSON, so a piece of a few megabytes of "[]," would still become
// gigabytes of tables. Each scalar, array and object counts as one.
const maxStreamedElementValues = 200_000

// windowSlack lets the decoder read a little past a piece at the cap -- the
// separator and whitespace after it -- without tripping the window. The exact
// cap is still enforced on the parsed size.
const windowSlack = 4096

// maxStreamedNestingDepth matches the nesting limit encoding/json's Decode
// applies. The token path below recurses once per level and would otherwise
// overflow the goroutine stack -- a fatal error, not a panic -- on a file of a
// few million '{'.
const maxStreamedNestingDepth = 10000

var errElementTooLarge = fmt.Errorf("an element is larger than %d bytes", maxStreamedElementSize)

var errElementTooManyValues = fmt.Errorf("an element holds more than %d values", maxStreamedElementValues)

// windowReader refuses to read more than maxStreamedElementSize bytes past the
// start of the current piece, so the decoder never buffers much more than one
// piece. The decoder may already hold bytes past that start from reading
// ahead, so a piece's size is also checked once it is parsed (see piece).
type windowReader struct {
	r     io.Reader
	read  int64
	limit int64
}

// open starts a piece at the decoder's parse offset.
func (w *windowReader) open(start int64) { w.limit = start + maxStreamedElementSize + windowSlack }

func (w *windowReader) Read(p []byte) (int, error) {
	if w.read >= w.limit {
		return 0, errElementTooLarge
	}
	if room := w.limit - w.read; int64(len(p)) > room {
		p = p[:room]
	}
	n, err := w.r.Read(p)
	w.read += int64(n)
	return n, err
}

// errStopStream is how a callback returning false ends the walk early.
var errStopStream = errors.New("stopped by callback")

// eachResourceJSON is the body of
//
//	mah.db.each_resource_json(id, fn(key, index, value)) -> true | false | nil, err
//
// once db_api.go has opened the file as the caller. It streams the JSON and
// calls fn once per piece: for a root object, once per element of each
// top-level array member as (key, i, element) and once per other member as
// (key, nil, value); for a root array, once per element as (nil, i, element).
// Only one element is decoded at a time, which is the point: get_resource_data
// hands over the whole file, and decoding a large export into Lua tables in one
// go takes many times the file's size in memory.
//
// It returns true after the whole file, false when fn returned false, and
// nil, err when the file or one piece of it is over its cap, or it is not
// JSON, after any pieces already delivered. An error raised inside fn propagates as if fn had been
// called directly.
func eachResourceJSON(L *lua.LState, file io.Reader, size int64, fn *lua.LFunction) int {
	if size > maxStreamedResourceFileSize {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("file too large (max %d bytes)", maxStreamedResourceFileSize)))
		return 2
	}

	deliver := func(key lua.LValue, index lua.LValue, value any) error {
		if ctx := L.Context(); ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		budget := maxStreamedElementValues
		luaValue, err := streamedValueToLua(L, value, &budget)
		if err != nil {
			return err
		}
		if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, key, index, luaValue); err != nil {
			var apiErr *lua.ApiError
			if errors.As(err, &apiErr) {
				L.Error(apiErr.Object, 0)
			}
			// Protect:true reports every error as an *ApiError; this is only
			// here so a change to that cannot swallow one.
			L.RaiseError("%s", err.Error())
		}
		ret := L.Get(-1)
		L.Pop(1)
		if ret == lua.LFalse {
			return errStopStream
		}
		return nil
	}

	err := streamJSON(io.LimitReader(file, maxStreamedResourceFileSize), deliver)
	switch {
	case err == nil:
		L.Push(lua.LTrue)
		return 1
	case errors.Is(err, errStopStream):
		L.Push(lua.LFalse)
		return 1
	case errors.Is(err, errElementTooLarge):
		L.Push(lua.LNil)
		L.Push(lua.LString(errElementTooLarge.Error()))
		return 2
	case errors.Is(err, errElementTooManyValues):
		L.Push(lua.LNil)
		L.Push(lua.LString(errElementTooManyValues.Error()))
		return 2
	default:
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
}

// streamJSON walks a JSON document as eachResourceJSON describes.
func streamJSON(r io.Reader, deliver func(key, index lua.LValue, value any) error) error {
	w := &windowReader{r: r}
	w.open(0)
	dec := json.NewDecoder(w)
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	switch tok {
	case json.Delim('['):
		if err := streamArray(dec, w, lua.LNil, deliver); err != nil {
			return err
		}
	case json.Delim('{'):
		for dec.More() {
			start := dec.InputOffset()
			w.open(start)
			keyTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("not valid JSON: %w", err)
			}
			key := lua.LString(keyTok.(string))
			valueTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("not valid JSON: %w", err)
			}
			if valueTok == json.Delim('[') {
				if err := streamArray(dec, w, key, deliver); err != nil {
					return err
				}
				continue
			}
			value, err := valueFromToken(dec, valueTok, 1)
			if err != nil {
				return err
			}
			if dec.InputOffset()-start > maxStreamedElementSize {
				return errElementTooLarge
			}
			if err := deliver(key, lua.LNil, value); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return fmt.Errorf("not valid JSON: %w", err)
		}
	default:
		return errors.New("not a JSON object or array")
	}
	return onlyWhitespaceLeft(io.MultiReader(dec.Buffered(), w.r))
}

// onlyWhitespaceLeft reads what follows the document a buffer at a time, so
// trailing whitespace of any length (up to the file cap) is accepted and
// anything else is refused without being parsed.
func onlyWhitespaceLeft(r io.Reader) error {
	buf := make([]byte, 32<<10)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
				return errors.New("not valid JSON: data after the end of the document")
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// streamArray delivers the elements of an array whose '[' has been read.
func streamArray(dec *json.Decoder, w *windowReader, key lua.LValue, deliver func(key, index lua.LValue, value any) error) error {
	for i := 1; dec.More(); i++ {
		start := dec.InputOffset()
		w.open(start)
		var element any
		if err := dec.Decode(&element); err != nil {
			return fmt.Errorf("not valid JSON: %w", err)
		}
		if dec.InputOffset()-start > maxStreamedElementSize {
			return errElementTooLarge
		}
		if err := deliver(key, lua.LNumber(i), element); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	return nil
}

// valueFromToken builds the value that starts with tok. The decoder cannot
// peek, so a member that turned out not to be an array has already had its
// first token read, and the rest is assembled from tokens. depth is the
// nesting level tok opens.
func valueFromToken(dec *json.Decoder, tok json.Token, depth int) (any, error) {
	if depth > maxStreamedNestingDepth {
		return nil, fmt.Errorf("not valid JSON: nested more than %d levels deep", maxStreamedNestingDepth)
	}
	switch tok {
	case json.Delim('{'):
		obj := map[string]any{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("not valid JSON: %w", err)
			}
			next, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("not valid JSON: %w", err)
			}
			value, err := valueFromToken(dec, next, depth+1)
			if err != nil {
				return nil, err
			}
			obj[keyTok.(string)] = value
		}
		_, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("not valid JSON: %w", err)
		}
		return obj, nil
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			var element any
			if err := dec.Decode(&element); err != nil {
				return nil, fmt.Errorf("not valid JSON: %w", err)
			}
			arr = append(arr, element)
		}
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("not valid JSON: %w", err)
		}
		return arr, nil
	}
	return tok, nil
}

// streamedValueToLua converts one decoded piece to Lua, as mah.json.decode
// would, but with every table sized exactly and the values counted against
// budget. goToLuaValue preallocates a few kilobytes per table, which is
// harmless for the small values it is used for and not for a piece of JSON
// made of empty arrays.
func streamedValueToLua(L *lua.LState, v any, budget *int) (lua.LValue, error) {
	*budget--
	if *budget < 0 {
		return nil, errElementTooManyValues
	}
	switch val := v.(type) {
	case nil:
		return lua.LNil, nil
	case string:
		return lua.LString(val), nil
	case float64:
		return lua.LNumber(val), nil
	case bool:
		return lua.LBool(val), nil
	case []any:
		tbl := L.CreateTable(len(val), 0)
		for i, item := range val {
			lv, err := streamedValueToLua(L, item, budget)
			if err != nil {
				return nil, err
			}
			tbl.RawSetInt(i+1, lv)
		}
		return tbl, nil
	case map[string]any:
		tbl := L.CreateTable(0, len(val))
		for k, item := range val {
			lv, err := streamedValueToLua(L, item, budget)
			if err != nil {
				return nil, err
			}
			tbl.RawSetString(k, lv)
		}
		return tbl, nil
	}
	return nil, fmt.Errorf("not valid JSON: unexpected %T", v)
}

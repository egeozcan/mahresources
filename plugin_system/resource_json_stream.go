package plugin_system

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	lua "github.com/yuin/gopher-lua"
)

// maxStreamedResourceFileSize caps mah.db.each_resource_json. The decoder holds
// one element at a time, so this bounds how long a call can run, not memory.
const maxStreamedResourceFileSize = 1 << 30 // 1 GiB

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
// nil, err when the file is over the cap or is not JSON, after any pieces
// already delivered. An error raised inside fn propagates as if fn had been
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
		if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, key, index, goToLuaValue(L, value)); err != nil {
			var apiErr *lua.ApiError
			if errors.As(err, &apiErr) {
				L.Error(apiErr.Object, 0)
			}
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
	default:
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
}

// streamJSON walks a JSON document as luaEachResourceJSON describes.
func streamJSON(r io.Reader, deliver func(key, index lua.LValue, value any) error) error {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	switch tok {
	case json.Delim('['):
		if err := streamArray(dec, lua.LNil, deliver); err != nil {
			return err
		}
	case json.Delim('{'):
		for dec.More() {
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
				if err := streamArray(dec, key, deliver); err != nil {
					return err
				}
				continue
			}
			value, err := valueFromToken(dec, valueTok)
			if err != nil {
				return err
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
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("not valid JSON: data after the end of the document")
	}
	return nil
}

// streamArray delivers the elements of an array whose '[' has been read.
func streamArray(dec *json.Decoder, key lua.LValue, deliver func(key, index lua.LValue, value any) error) error {
	for i := 1; dec.More(); i++ {
		var element any
		if err := dec.Decode(&element); err != nil {
			return fmt.Errorf("not valid JSON: %w", err)
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
// first token read, and the rest is assembled from tokens.
func valueFromToken(dec *json.Decoder, tok json.Token) (any, error) {
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
			value, err := valueFromToken(dec, next)
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

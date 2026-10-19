//go:build js && wasm

// Command tuimark-wasm is the Tuimark runtime compiled to WebAssembly for
// the website's playground (ADR 0033). It is the same runtime as the CLI,
// not a second host: its frames are the dumps tuimark prints.
//
// It registers one global, __tuimark, with the methods open, render, step,
// validate, reload, and close. Every method takes JS values (objects,
// strings, numbers) and returns a JSON string; the engine, the argument
// shapes, and the result shapes are documented in internal/playground.
// A result holds an "error" member when the call failed and never
// otherwise; a panic in Go is recovered into the same shape.
//
// This entry is private to the website: no compatibility promise, not
// published, not a command. Build it with
//
//	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o tuimark.wasm ./cmd/tuimark-wasm
//
// and load it with the wasm_exec.js of the same Go toolchain
// ($(go env GOROOT)/lib/wasm/wasm_exec.js). __tuimark.ready is true once
// the methods are registered.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/abdul-hamid-achik/tuimark/internal/playground"
)

var methods = []string{"open", "render", "step", "validate", "reload", "close"}

// encode turns a JS argument into its JSON text; absent and null are
// "null".
func encode(v js.Value) string {
	if v.IsUndefined() || v.IsNull() {
		return "null"
	}
	return js.Global().Get("JSON").Call("stringify", v).String()
}

// call runs one method; a panic (including a JS exception raised while
// reading the arguments) becomes an error result.
func call(m *playground.Manager, method string, args []js.Value) (out string) {
	defer func() {
		if r := recover(); r != nil {
			b, _ := json.Marshal(map[string]string{"error": fmt.Sprint("internal error in ", method, ": ", r)})
			out = string(b)
		}
	}()
	enc := make([]string, len(args))
	for i, a := range args {
		enc[i] = encode(a)
	}
	return m.Call(method, enc...)
}

func main() {
	m := playground.NewManager()
	api := js.Global().Get("Object").New()
	for _, name := range methods {
		name := name
		api.Set(name, js.FuncOf(func(_ js.Value, args []js.Value) any {
			return call(m, name, args)
		}))
	}
	api.Set("ready", true)
	js.Global().Set("__tuimark", api)
	select {}
}

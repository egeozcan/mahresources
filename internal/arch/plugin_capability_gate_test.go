package arch

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// exprText renders an expression back to source, for matching on receiver names
// and guard conditions.
func exprText(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

// alwaysInstalledModules are the mah sub-modules every plugin gets, because
// none of them reads or writes anything outside the plugin itself. Adding a
// module here is the decision this test exists to force somebody to make.
var alwaysInstalledModules = map[string]bool{
	"registerJsonModule": true,
	"registerUtilModule": true,
	// registerDbModule takes the grant set and splits db:read from db:write
	// inside itself, so it is called unconditionally by design.
	"registerDbModule": true,
}

// moduleCapabilities pins which capability gates which sub-module. A guard is
// not enough on its own: `if grants.Has(CapKV) { pm.registerHttpModule(...) }`
// is a positive grants.Has check that grants http to anyone who asked for kv.
var moduleCapabilities = map[string]string{
	"registerHttpModule":  "CapHTTP",
	"registerKvModule":    "CapKV",
	"registerImageModule": "CapImage",
}

// ungatedFunctions are the root-level mah functions installed for every plugin,
// registered as setIf("", ...). Each reads or writes nothing outside the plugin
// itself, in the sense that matters for consent: none of them reads or changes
// another plugin's data, the user's library, or anything off the machine.
// html_escape is pure; abort is inert without a hook to raise from; doc
// contributes to the plugin's own docs page; get_setting reads only that
// plugin's settings; log writes the plugin's own lines to the application log;
// sleep spends up to 30s of the plugin's own VM. The last two are bounded
// resource use rather than nothing at all, which is the honest way to state it.
//
// The list is exact, because `setIf("", ...)` is the cheapest way to add an
// ungated function — copy the line next door — so the test names them rather
// than counting them.
var ungatedFunctions = map[string]bool{
	"log":         true,
	"abort":       true,
	"get_setting": true,
	"doc":         true,
	"html_escape": true,
	"sleep":       true,
}

// setIfRegistrations pins the capability that reaches each single-capability
// root function. The ungated set and every setIfAny list are already pinned by
// name for the same reason; this was the one registration shape a refactor
// could quietly move, because nothing checked *which* Cap constant was passed —
// setIf(CapKV, "on", ...) compiled and satisfied every other rule.
var setIfRegistrations = map[string]string{
	"on":           "CapHooks",
	"inject":       "CapInject",
	"page":         "CapPages",
	"menu":         "CapPages",
	"action":       "CapActions",
	"block_type":   "CapRender",
	"display_type": "CapRender",
	"shortcode":    "CapRender",
	"api":          "CapAPI",
	"start_job":    "CapJobs",
	"schedule":     "CapSchedule",
}

// setIfAnyRegistrations pins every multi-capability registration and the exact
// list that reaches it. setIfAny exists for "more than one capability implies
// this", so widening its list is the natural way to make a new function
// reachable — and a list long enough that every plugin holds one member is an
// ungated function that no other rule would notice.
var setIfAnyRegistrations = map[string][]string{
	"job_progress": {"CapActions", "CapJobs"},
	"job_complete": {"CapActions", "CapJobs"},
	"job_fail":     {"CapActions", "CapJobs"},
}

// subModuleKeys pins which key each sub-module registrar may write onto the mah
// table. The mahMod rule below is scoped to registerMahModule, but every
// registrar receives mahMod and could write anything into it — and two of them
// are called unconditionally, so a key added inside one would reach every
// plugin ungated.
var subModuleKeys = map[string]string{
	"registerDbModule":    "db",
	"registerHttpModule":  "http",
	"registerJsonModule":  "json",
	"registerKvModule":    "kv",
	"registerImageModule": "image",
	"registerUtilModule":  "util",
}

// TestSubModulesWriteOnlyTheirOwnKey holds the mahMod rule over the registrars
// too, not just over the function that declares the table.
func TestSubModulesWriteOnlyTheirOwnKey(t *testing.T) {
	root := moduleRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "plugin_system", "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	var offenders []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			want, pinned := subModuleKeys[fn.Name.Name]
			if !pinned {
				continue
			}
			seen[fn.Name.Name] = true

			// mahMod may be *mentioned* only in the one write below. An alias
			// (`t := mahMod; t.RawSetString(...)`) would otherwise add a second
			// root key from inside a registrar, where the rule in
			// registerMahModule cannot see it.
			var writes []ast.Node
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && exprText(fset, sel.X) == "mahMod" {
					writes = append(writes, call)
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok || id.Name != "mahMod" {
					return true
				}
				for _, w := range writes {
					if id.Pos() >= w.Pos() && id.End() <= w.End() {
						return true
					}
				}
				// The parameter declaration itself is fine.
				if fn.Type.Params != nil && id.Pos() >= fn.Type.Params.Pos() && id.End() <= fn.Type.Params.End() {
					return true
				}
				offenders = append(offenders, fmt.Sprintf("%s mentions mahMod at %s outside its single registration",
					fn.Name.Name, fset.Position(id.Pos())))
				return true
			})

			var keys []string
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 2 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || exprText(fset, sel.X) != "mahMod" {
					return true
				}
				if lit, ok := call.Args[0].(*ast.BasicLit); ok {
					keys = append(keys, strings.Trim(lit.Value, `"`))
				} else {
					keys = append(keys, "<computed>")
				}
				return true
			})
			if len(keys) != 1 || keys[0] != want {
				offenders = append(offenders, fmt.Sprintf("%s writes %v, expected exactly [%s]", fn.Name.Name, keys, want))
			}
		}
	}
	for name := range subModuleKeys {
		if !seen[name] {
			offenders = append(offenders, fmt.Sprintf("%s not found; update subModuleKeys", name))
		}
	}
	if len(offenders) > 0 {
		t.Errorf("sub-module registrar wrote something other than its own key: %s.\n"+
			"Each registrar receives the mah table and may add exactly one key — its own module. Two of\n"+
			"them are installed for every plugin, so anything else added inside one is an ungated\n"+
			"function on every plugin's surface.", strings.Join(offenders, "; "))
	}
}

// TestEveryMahRegistrationHasACapability pins the rules that make capability
// grants enforceable by construction rather than by discipline.
//
// Grants work because registerMahModule builds the mah table key by key: an
// ungranted key is never set, so there is nothing to guard at the call sites
// and nothing for a plugin to reach. That property is invisible in the code — a
// new registration looks exactly like the gated ones next to it, and silently
// hands every plugin a power no operator consented to.
//
// The rule is therefore stated over `mahMod` itself rather than over any
// particular way of writing to it: inside registerMahModule the identifier may
// appear only where it is declared, inside the setIf/setIfAny installers, and
// as an argument to a sub-module registration that carries a capability
// decision. That covers RawSetString, RawSet, L.SetField and aliasing without
// having to enumerate them.
func TestEveryMahRegistrationHasACapability(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == "registerMahModule" {
			fn = d
			break
		}
	}
	if fn == nil {
		t.Fatal("registerMahModule not found in plugin_system/manager.go; this test needs updating with whatever replaced it")
	}

	type span struct{ from, to token.Pos }
	within := func(spans []span, n ast.Node) bool {
		for _, s := range spans {
			if n.Pos() >= s.from && n.End() <= s.to {
				return true
			}
		}
		return false
	}

	// The setIf/setIfAny closures are the two places that may write mahMod, and
	// the declaration is where it comes from.
	var installers, declared []span
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		switch ident.Name {
		case "setIf", "setIfAny":
			if lit, ok := assign.Rhs[0].(*ast.FuncLit); ok {
				installers = append(installers, span{lit.Pos(), lit.End()})
			}
		case "mahMod":
			declared = append(declared, span{assign.Pos(), assign.End()})
		}
		return true
	})
	if len(installers) == 0 || len(declared) == 0 {
		t.Fatal("registerMahModule no longer declares mahMod alongside its setIf/setIfAny installers; the capability gate has been restructured")
	}

	// The installer bodies are exempt from the mahMod rule because they are the
	// ones that write it — so they are the one place where deleting a check
	// disables every gate at once, invisibly.
	//
	// The rule is about *reachability*, not about a particular spelling: every
	// write to mahMod inside an installer must be unreachable unless a grant
	// says otherwise. Two shapes satisfy that, and both are in use —
	// `if !grants.Has(c) { return }` before the write (setIf), and a write that
	// sits inside an `if grants.Has(c)` body (setIfAny). Anything else — no
	// check, an inverted check, or a check whose result is discarded — leaves
	// the write reachable for a plugin that was granted nothing.
	for _, body := range installers {
		var writes []ast.Node
		var negativeGuardEnd token.Pos
		var positiveBlocks [][2]token.Pos
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			if n.Pos() < body.from || n.End() > body.to {
				return true
			}
			switch node := n.(type) {
			case *ast.IfStmt:
				cond := exprText(fset, node.Cond)
				if strings.Contains(cond, "!grants.Has") && !strings.Contains(cond, `== ""`) {
					// `capability == "" && !grants.Has(capability)` reads as a
					// negative guard and refuses only the ungated functions —
					// i.e. exactly nothing.
					for _, stmt := range node.Body.List {
						if _, isReturn := stmt.(*ast.ReturnStmt); isReturn && node.End() > negativeGuardEnd {
							negativeGuardEnd = node.End()
						}
					}
				} else if strings.Contains(cond, "grants.Has") {
					positiveBlocks = append(positiveBlocks, [2]token.Pos{node.Body.Pos(), node.Body.End()})
				}
			case *ast.CallExpr:
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok && exprText(fset, sel.X) == "mahMod" {
					writes = append(writes, node)
				}
			}
			return true
		})

		for _, w := range writes {
			guarded := w.Pos() > negativeGuardEnd && negativeGuardEnd != token.NoPos
			for _, blk := range positiveBlocks {
				if w.Pos() >= blk[0] && w.End() <= blk[1] {
					guarded = true
				}
			}
			if !guarded {
				t.Errorf("the installer at %s writes mahMod at %s without a grant deciding whether it runs.\n"+
					"It is the only code that writes that table, so a missing, inverted or discarded check\n"+
					"there makes every setIf call an unconditional registration.",
					fset.Position(body.from), fset.Position(w.Pos()))
			}
		}
		if len(writes) == 0 {
			t.Errorf("the installer at %s no longer writes mahMod; this test is no longer reading what it thinks it is",
				fset.Position(body.from))
		}
	}

	// guardedCapabilityName returns the capability constant a grants.Has guard
	// names, or "" if the condition is not one.
	guardedCapabilityName := func(cond ast.Expr) string {
		call, ok := cond.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return ""
		}
		if id, ok := call.Args[0].(*ast.Ident); ok {
			return id.Name
		}
		return ""
	}

	// positiveGrantGuard reports whether an if-condition is exactly
	// grants.Has(X). A negated or compound condition is not a guard:
	// `if !grants.Has(CapKV) { register }` inverts the grant while reading like
	// one.
	positiveGrantGuard := func(cond ast.Expr) bool {
		call, ok := cond.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Has" {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		return ok && x.Name == "grants"
	}

	// Every call handed mahMod is a sub-module registration — found by what it
	// receives rather than by what it is named, so renaming one does not hide
	// it.
	var moduleCalls []span
	var ungatedModules []string
	var gatedModules int
	var guardStack []bool
	var capabilityStack []string
	// guardCapability reports the innermost capability currently gating the walk.
	guardCapability := func() string {
		for i := len(capabilityStack) - 1; i >= 0; i-- {
			if capabilityStack[i] != "" {
				return capabilityStack[i]
			}
		}
		return ""
	}

	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			// The capability only counts when the condition really is
			// grants.Has(X): otherwise `if somethingElse(CapHTTP) { ... }` reads
			// as gated on http while gating on nothing.
			positive := positiveGrantGuard(node.Cond)
			guardStack = append(guardStack, positive)
			named := ""
			if positive {
				named = guardedCapabilityName(node.Cond)
			}
			capabilityStack = append(capabilityStack, named)
			ast.Inspect(node.Body, walk)
			guardStack = guardStack[:len(guardStack)-1]
			capabilityStack = capabilityStack[:len(capabilityStack)-1]
			if node.Else != nil {
				// An else branch is not covered by the guard that produced it.
				guardStack = append(guardStack, false)
				capabilityStack = append(capabilityStack, "")
				ast.Inspect(node.Else, walk)
				guardStack = guardStack[:len(guardStack)-1]
				capabilityStack = capabilityStack[:len(capabilityStack)-1]
			}
			return false
		case *ast.CallExpr:
			if within(installers, node) {
				return true
			}
			passesMahMod := false
			for _, arg := range node.Args {
				if id, ok := arg.(*ast.Ident); ok && id.Name == "mahMod" {
					passesMahMod = true
				}
			}
			if !passesMahMod {
				return true
			}
			moduleCalls = append(moduleCalls, span{node.Pos(), node.End()})
			name := ""
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				name = sel.Sel.Name
			}
			// L.SetGlobal("mah", mahMod) publishes the finished table. It is
			// the one call that hands mahMod somewhere without registering
			// anything into it, and it is what makes every gated decision above
			// take effect.
			if name == "SetGlobal" && len(node.Args) == 2 {
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Value == `"mah"` {
					return true
				}
			}
			if name == "" {
				// The call is not `pm.somethingModule(...)` — it reaches its
				// target through a variable, which is exactly how a registrar
				// escapes the capability pin below: `register :=
				// pm.registerHttpModule` and then `if grants.Has(CapKV) {
				// register(L, mahMod) }` grants http to whoever asked for kv,
				// under a positive guard, with no name to check it against.
				ungatedModules = append(ungatedModules,
					fmt.Sprintf("a call at %s receives mahMod through a variable rather than naming its registrar",
						fset.Position(node.Pos())))
			} else if want, pinned := moduleCapabilities[name]; pinned {
				if guardCapability() != want {
					ungatedModules = append(ungatedModules, fmt.Sprintf("%s (gated on %s, expected %s)", name, guardCapability(), want))
				} else {
					gatedModules++
				}
			} else if alwaysInstalledModules[name] {
				gatedModules++
			} else {
				// Not "guarded by something" — pinned, by name, in this test. A
				// new registrar under any positive grants.Has guard would
				// otherwise install a whole new surface that no rule here has
				// an opinion about.
				ungatedModules = append(ungatedModules, fmt.Sprintf(
					"%s (not listed in moduleCapabilities or alwaysInstalledModules)", name))
			}
		}
		return true
	}
	ast.Inspect(fn.Body, walk)

	// Any other mention of mahMod is a write the gate cannot see.
	// Aliasing an installer takes every registration made through it out of
	// every rule in this file, all of which match the literal callee.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, rhs := range assign.Rhs {
			if id, ok := rhs.(*ast.Ident); ok && (id.Name == "setIf" || id.Name == "setIfAny") {
				t.Errorf("%s is aliased at %s. Every rule in this file matches the literal callee, so a\n"+
					"registration made through the alias is examined by nothing at all.",
					id.Name, fset.Position(assign.Pos()))
			}
		}
		return true
	})

	var escapes []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != "mahMod" {
			return true
		}
		if within(installers, id) || within(declared, id) || within(moduleCalls, id) {
			return true
		}
		escapes = append(escapes, fset.Position(id.Pos()).String())
		return true
	})
	if len(escapes) > 0 {
		t.Errorf("mahMod is used outside setIf/setIfAny and the sub-module registrations, at %s.\n"+
			"Every root-level mah function must be installed through setIf(capability, name, fn) or\n"+
			"setIfAny([]string{...}, name, fn): an ungranted key must never be set, which is what makes a\n"+
			"grant enforceable rather than advisory. Aliasing mahMod, or writing it with RawSet/SetField,\n"+
			"escapes that — which is why the rule is on the identifier and not on one method name.",
			strings.Join(escapes, ", "))
	}
	if len(ungatedModules) > 0 {
		t.Errorf("sub-module(s) %s receive mahMod without a capability decision.\n"+
			"Wrap the call in `if grants.Has(CapX)` — positive, not negated — or add it to\n"+
			"alwaysInstalledModules in this test with the reason it needs no grant.",
			strings.Join(ungatedModules, ", "))
	}

	// The always-installed set is named, not counted.
	var ungated []string
	var setIfCalls int
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || (id.Name != "setIf" && id.Name != "setIfAny") {
			return true
		}
		setIfCalls++
		if id.Name != "setIf" || len(call.Args) < 2 {
			return true
		}
		capability, ok := call.Args[0].(*ast.BasicLit)
		if !ok || capability.Value != `""` {
			return true
		}
		if name, ok := call.Args[1].(*ast.BasicLit); ok {
			ungated = append(ungated, strings.Trim(name.Value, `"`))
		}
		return true
	})

	// setIfAny's list is pinned by name, for the same reason the ungated set is:
	// widening it is how a function becomes reachable without a decision.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "setIfAny" || len(call.Args) < 2 {
			return true
		}
		name := ""
		if lit, ok := call.Args[1].(*ast.BasicLit); ok {
			name = strings.Trim(lit.Value, `"`)
		}
		want, pinned := setIfAnyRegistrations[name]
		if !pinned {
			t.Errorf("mah.%s is registered with setIfAny but is not in setIfAnyRegistrations in this test.\n"+
				"Add it with the exact capability list that may reach it.", name)
			return true
		}
		var got []string
		if lit, ok := call.Args[0].(*ast.CompositeLit); ok {
			for _, elt := range lit.Elts {
				got = append(got, exprText(fset, elt))
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("mah.%s is reachable from %v, but this test pins %v. Widening a setIfAny list is how a\n"+
				"function becomes effectively ungated; change the test deliberately if the change is intended.",
				name, got, want)
		}
		return true
	})

	// Which capability, not merely that there is one.
	seenSetIf := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "setIf" || len(call.Args) < 2 {
			return true
		}
		capArg, capOK := call.Args[0].(*ast.Ident)
		nameLit, nameOK := call.Args[1].(*ast.BasicLit)
		if !capOK || !nameOK {
			return true
		}
		name := strings.Trim(nameLit.Value, `"`)
		want, pinned := setIfRegistrations[name]
		if !pinned {
			t.Errorf("mah.%s is registered with setIf(%s, ...) but is not in setIfRegistrations in this test.\n"+
				"Add it with the capability that may reach it.", name, capArg.Name)
			return true
		}
		seenSetIf[name] = true
		if capArg.Name != want {
			t.Errorf("mah.%s is gated on %s, but this test pins %s. Moving a function to another\n"+
				"capability is a change to what an operator consented to; change the test deliberately.",
				name, capArg.Name, want)
		}
		return true
	})
	for name := range setIfRegistrations {
		if !seenSetIf[name] {
			t.Errorf("mah.%s is pinned in setIfRegistrations but is no longer registered with setIf; update the test.", name)
		}
	}

	// A capability argument that is a string literal rather than one of the Cap
	// constants would never match a grant, so the function would be installed
	// for nobody — or, spelled as "", for everybody.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "setIf" || len(call.Args) < 1 {
			return true
		}
		switch arg := call.Args[0].(type) {
		case *ast.Ident:
			if !strings.HasPrefix(arg.Name, "Cap") {
				t.Errorf("setIf at %s takes capability %q, which is not one of the Cap constants",
					fset.Position(call.Pos()), arg.Name)
			}
		case *ast.BasicLit:
			if arg.Value != `""` {
				t.Errorf("setIf at %s takes the string literal %s as its capability; use a Cap constant, "+
					"or \"\" for a function that needs no grant", fset.Position(call.Pos()), arg.Value)
			}
		}
		return true
	})
	sort.Strings(ungated)
	seen := map[string]bool{}
	for _, name := range ungated {
		seen[name] = true
		if !ungatedFunctions[name] {
			t.Errorf("mah.%s is installed for every plugin (setIf with an empty capability) but is not in\n"+
				"ungatedFunctions in this test. Either give it a capability, or add it there with the reason\n"+
				"it reads and writes nothing outside the plugin itself.", name)
		}
	}
	for name := range ungatedFunctions {
		if !seen[name] {
			t.Errorf("mah.%s is listed as always-installed in this test but is no longer registered that way; update ungatedFunctions.", name)
		}
	}

	// A sanity floor: if the shape of the function changes so much that nothing
	// matches, the rules above would pass vacuously.
	if setIfCalls < 10 || gatedModules < 3 {
		t.Errorf("found only %d setIf/setIfAny calls and %d sub-module registrations; this test is no longer reading what it thinks it is",
			setIfCalls, gatedModules)
	}
}

// TestNothingReachesTheMahTableFromOutsideItsBuilder closes the gap the other
// two guards leave: they read one function each.
//
// registerMahModule builds the mah table and publishes it with L.SetGlobal.
// Nothing stops a helper elsewhere in the package from fetching it back —
// `L.GetGlobal("mah").(*lua.LTable).RawSetString("danger", ...)` — and adding a
// function to every plugin's surface, after every capability decision has been
// made and with no registration site for the other guards to notice.
func TestNothingReachesTheMahTableFromOutsideItsBuilder(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "plugin_system")

	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	var offenders []string
	for _, path := range entries {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				// GetGlobal("mah") fetches the finished table back;
				// SetGlobal("mah", ...) replaces it wholesale. Both reach the
				// surface after every capability decision has been made.
				// L.Get(lua.GlobalsIndex) is the globals table by another name.
				if sel.Sel.Name == "Get" && len(node.Args) == 1 &&
					strings.Contains(exprText(fset, node.Args[0]), "GlobalsIndex") {
					offenders = append(offenders, fset.Position(node.Pos()).String()+" (GlobalsIndex)")
				}
				if (sel.Sel.Name == "GetGlobal" || sel.Sel.Name == "SetGlobal") && len(node.Args) >= 1 {
					lit, isLiteral := node.Args[0].(*ast.BasicLit)
					switch {
					case isLiteral && lit.Value == `"mah"`:
						if !insideMahBuilder(file, node) {
							offenders = append(offenders, fset.Position(node.Pos()).String())
						}
					case !isLiteral:
						// A computed name cannot be checked against "mah", so
						// it cannot be allowed to *install* anything:
						// L.GetGlobal("m" + "ah") reaches the table while
						// matching no literal. Setting a computed name to nil
						// is the opposite — that is how the unsafe base
						// functions are removed — and installs nothing.
						removesGlobal := sel.Sel.Name == "SetGlobal" && len(node.Args) == 2 &&
							exprText(fset, node.Args[1]) == "lua.LNil"
						if !removesGlobal {
							offenders = append(offenders, fset.Position(node.Pos()).String()+" (computed global name)")
						}
					}
				}
			case *ast.AssignStmt:
				// `g := L.G` puts the globals table one selector away, out of
				// reach of the rule below, which matches on `L.G.Global`.
				for _, rhs := range node.Rhs {
					if sel, ok := rhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "G" {
						offenders = append(offenders, fset.Position(node.Pos()).String()+" (L.G aliased)")
					}
				}
			case *ast.SelectorExpr:
				// A computed argument hides the key: L.GetGlobal("m" + "ah").
				// (handled below alongside the literal case)
				// A method value hands the same reach to a variable:
				// `get := L.GetGlobal; get("mah")`.
				if node.Sel.Name == "GetGlobal" || node.Sel.Name == "SetGlobal" {
					if !isCallee(file, node) {
						offenders = append(offenders, fset.Position(node.Pos()).String()+" (method value)")
					}
					return true
				}
				// L.G.Global is the globals table itself: RawGetString("mah")
				// on it, or writing through it, goes around the rule above.
				// Stripping its metatable is the one legitimate use.
				if node.Sel.Name != "Global" {
					return true
				}
				inner, ok := node.X.(*ast.SelectorExpr)
				if !ok || inner.Sel.Name != "G" {
					return true
				}
				if !isMetatableStrip(file, node) {
					offenders = append(offenders, fset.Position(node.Pos()).String()+" (L.G.Global)")
				}
			}
			return true
		})
	}

	if len(offenders) > 0 {
		t.Errorf("the mah table, or the globals table holding it, is reached at %s.\n"+
			"The capability gate is enforced where the table is built. Reading it back anywhere else\n"+
			"allows a function to be added to every plugin's surface after every grant decision has\n"+
			"been made, with no registration site for the other guards to see.",
			strings.Join(offenders, ", "))
	}
}

// querierBackedWrites create resources but are declared on EntityQuerier, so
// they are gated as writes while calling querierFor. Both accessors bind the
// same adapter to the same principal, so nothing escapes today — but the
// interface is lying, and moving them onto EntityWriter is the follow-up that
// removes this list.
var querierBackedWrites = map[string]bool{
	"create_resource_from_url":      true,
	"create_resource_from_data":     true,
	"add_resource_version_from_url": true,
}

// urlFetchingWrites are the mah.db functions that perform an outbound request
// by handing a URL to the application's own downloader. They are gated on
// db:write AND http, and their URL goes through the same egress layers mah.http
// does.
var urlFetchingWrites = map[string]bool{
	"create_resource_from_url":      true,
	"add_resource_version_from_url": true,
}

// querierWriteMethods are the EntityQuerier methods those three registrations
// legitimately call.
var querierWriteMethods = map[string]bool{
	"CreateResourceFromURL":     true,
	"CreateResourceFromData":    true,
	"AddResourceVersionFromURL": true,
}

// TestDbModuleRegistrationsDeclareReadOrWrite pins the db:read / db:write split.
//
// mah.db is one table with two capabilities over it, so the split cannot be a
// single `if` — it is a per-function decision. setRead and setWrite are the only
// two ways to make it, so a function added later cannot land on the wrong side
// by being copied from a neighbour.
func TestDbModuleRegistrationsDeclareReadOrWrite(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "db_api.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse db_api.go: %v", err)
	}

	// The bodies of setRead/setWrite are the only places dbMod may be written.
	var allowedRanges [][2]token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || (ident.Name != "setRead" && ident.Name != "setWrite" && ident.Name != "setFetchingWrite") {
			return true
		}
		if lit, ok := assign.Rhs[0].(*ast.FuncLit); ok {
			allowedRanges = append(allowedRanges, [2]token.Pos{lit.Pos(), lit.End()})
		}
		return true
	})
	if len(allowedRanges) != 3 {
		t.Fatalf("expected setRead, setWrite and setFetchingWrite closures in registerDbModule, found %d", len(allowedRanges))
	}

	// Where dbMod comes from, and the one call that hands it on.
	var dbModDeclaration ast.Node
	var dbModPublication ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) == 1 {
				if id, ok := node.Lhs[0].(*ast.Ident); ok && id.Name == "dbMod" {
					dbModDeclaration = node
				}
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "RawSetString" || exprText(fset, sel.X) != "mahMod" || len(node.Args) != 2 {
				return true
			}
			if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Value == `"db"` {
				dbModPublication = node
			}
		}
		return true
	})
	if dbModDeclaration == nil || dbModPublication == nil {
		t.Fatal("registerDbModule no longer declares dbMod and publishes it as mah.db; this test needs updating")
	}

	// The setRead/setWrite closures are exempt from the dbMod rule below because
	// they are what writes it — so, as with the mah installers, they are the one
	// place where deleting a condition makes every registration unconditional
	// while every other rule still passes.
	// setFetchingWrite is the third registrar: the two writers that open a
	// socket need db:write AND http, so its guard must name both. Checking only
	// for "canWrite" here would accept a guard that had quietly lost the http
	// half, which is the whole point of the third registrar existing.
	wants := [][]string{{"canRead"}, {"canWrite"}, {"canWrite", "grants.Has(CapHTTP)"}}
	for i, r := range allowedRanges {
		if i >= len(wants) {
			break
		}
		for _, want := range wants[i] {
			gated := false
			ast.Inspect(file, func(n ast.Node) bool {
				ifStmt, ok := n.(*ast.IfStmt)
				if !ok || n.Pos() < r[0] || n.End() > r[1] {
					return true
				}
				cond := exprText(fset, ifStmt.Cond)
				if cond == want || strings.Contains(cond, want) {
					gated = true
				}
				return true
			})
			if !gated {
				t.Errorf("the db registrar at %s has no `%s` in its guard, so every registration it makes is\n"+
					"unconditional in that dimension and the capability split stops existing.",
					fset.Position(r[0]), want)
			}
		}
	}

	// `register := setRead` would take the handler out of every rule below.
	// (setIf/setIfAny get the same treatment in the capability test.)
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, rhs := range assign.Rhs {
			if id, ok := rhs.(*ast.Ident); ok && (id.Name == "setRead" || id.Name == "setWrite" || id.Name == "setFetchingWrite") {
				t.Errorf("%s is aliased at %s. Every rule here matches the literal callee, so a registration\n"+
					"made through the alias is unexamined — including which adapter it reaches.",
					id.Name, fset.Position(assign.Pos()))
			}
		}
		return true
	})

	var registrars int
	registeredWith := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || (ident.Name != "setRead" && ident.Name != "setWrite" && ident.Name != "setFetchingWrite") {
			return true
		}
		registrars++
		if len(call.Args) > 0 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				registeredWith[strings.Trim(lit.Value, `"`)] = ident.Name
			}
		}
		return true
	})

	// The querier-backed exception is an exception for *writes*. Nothing tied it
	// to setWrite, so moving one of those three to setRead passed both checks —
	// the cross-check because the body genuinely does call querierFor, and the
	// mutation check because the method is allowlisted by name — and a
	// db:read-only plugin would have got a mutation.
	for name := range querierBackedWrites {
		switch registeredWith[name] {
		case "setWrite", "setFetchingWrite":
		case "":
			t.Errorf("%s is in querierBackedWrites but is not registered in db_api.go; update the list", name)
		default:
			t.Errorf("%s is registered with %s. It is in querierBackedWrites, which exempts it from the\n"+
				"querier/writer cross-check — an exemption that only makes sense for a write. Registered as a\n"+
				"read, it hands a mutation to a db:read-only plugin.", name, registeredWith[name])
		}
	}

	// The two writers that fetch a URL must go through setFetchingWrite.
	//
	// Plain setWrite compiles, passes every other rule here, and silently drops
	// the `http` requirement — handing any db:write plugin a server-side fetch
	// primitive that its declared `network` list does not describe. That is the
	// exact hole this package exists to close, so it gets its own rule rather
	// than relying on someone noticing the registrar name.
	for name := range urlFetchingWrites {
		switch registeredWith[name] {
		case "setFetchingWrite":
		case "":
			t.Errorf("%s is in urlFetchingWrites but is not registered in db_api.go; update the list", name)
		default:
			t.Errorf("%s is registered with %s, not setFetchingWrite. It opens a socket, so it needs the\n"+
				"http capability as well as db:write, and its URL must go through the egress layers.\n"+
				"Registered as a plain write, a db:write-only plugin regains an unrestricted fetch.",
				name, registeredWith[name])
		}
	}

	// The rule is on the identifier, not on one method name: an alias
	// (`alias := dbMod`), a RawSet, or an L.SetField would each escape a check
	// written against `dbMod.RawSetString`. dbMod may appear only where it is
	// declared, inside setRead/setWrite, and where the finished table is
	// published onto mah.
	var offenders []string
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != "dbMod" {
			return true
		}
		for _, r := range allowedRanges {
			if id.Pos() >= r[0] && id.End() <= r[1] {
				return true
			}
		}
		if dbModDeclaration != nil && id.Pos() >= dbModDeclaration.Pos() && id.End() <= dbModDeclaration.End() {
			return true
		}
		if dbModPublication != nil && id.Pos() >= dbModPublication.Pos() && id.End() <= dbModPublication.End() {
			return true
		}
		offenders = append(offenders, fset.Position(id.Pos()).String())
		return true
	})

	// The split is only worth anything if the two sides reach the two bound
	// adapters.
	var crossed []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || (ident.Name != "setRead" && ident.Name != "setWrite" && ident.Name != "setFetchingWrite") {
			return true
		}
		lit, ok := call.Args[1].(*ast.FuncLit)
		if !ok {
			// `setRead("x", pm.someMethod)` has no body here to inspect, so the
			// querier/writer rule below could not be applied to it at all.
			crossed = append(crossed, fmt.Sprintf("%s (handler must be written inline, not passed by name)",
				fset.Position(call.Pos())))
			return true
		}
		name := ""
		if s, ok := call.Args[0].(*ast.BasicLit); ok {
			name = strings.Trim(s.Value, `"`)
		}
		want, forbidden := "querierFor", "writerFor"
		switch ident.Name {
		case "setWrite":
			want, forbidden = "writerFor", "querierFor"
			if querierBackedWrites[name] {
				return true
			}
		case "setFetchingWrite":
			// querierForFetch, specifically. Plain querierFor binds the same
			// adapter to the same principal and would compile and pass every
			// other rule here — while silently dropping the plugin's network
			// policy, so the host-side downloader would fetch with no dial-time
			// deny and no redirect re-check. The policy only reaches the
			// fetcher because this accessor puts it on the invocation.
			want, forbidden = "querierForFetch", "writerFor"
		}
		var sawWanted, sawForbidden bool
		ast.Inspect(lit.Body, func(inner ast.Node) bool {
			sel, ok := inner.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case want:
				sawWanted = true
			case forbidden:
				sawForbidden = true
			}
			return true
		})
		if sawForbidden || !sawWanted {
			label := name
			if label == "" {
				label = fset.Position(call.Pos()).String()
			}
			crossed = append(crossed, fmt.Sprintf("%s (%s should use pm.%s)", label, ident.Name, want))
		}
		return true
	})

	if len(crossed) > 0 {
		t.Errorf("read/write registration crosses the adapter it declares: %s.\n"+
			"setRead bodies must reach the DB through pm.querierFor, setWrite bodies through\n"+
			"pm.writerFor, and setFetchingWrite bodies through pm.querierForFetch — which is the\n"+
			"only accessor that carries the plugin's network policy to the host-side downloader.",
			strings.Join(crossed, ", "))
	}
	if len(offenders) > 0 {
		t.Errorf("dbMod is used outside setRead/setWrite at %s.\n"+
			"Every mah.db function belongs to db:read (built on querierFor) or db:write (writerFor), and\n"+
			"setRead/setWrite are the only two ways to say which. Writing dbMod anywhere else — including\n"+
			"through an alias — puts a function in both, so a plugin granted read-only gets a writer.",
			strings.Join(offenders, ", "))
	}
	// Most of the 63 mah.db functions are registered through the shared
	// registrar helpers (registerGetter, registerOptsWriter, ...), each of which
	// calls setRead or setWrite once, so this floor counts call sites, not
	// functions.
	if registrars < 15 {
		t.Errorf("found only %d setRead/setWrite registrations; this test is no longer reading what it thinks it is", registrars)
	}
}

// TestReadPathsCannotWrite guards the db:read side against EntityQuerier
// growing a mutation.
//
// Be clear about what this does and does not catch today. EntityQuerier
// currently declares exactly three write-shaped methods — the resource-creation
// three — and all three are exempt, so right now the rule cannot fire: the
// compiler is what stops a setRead body calling DeleteGroup, because DeleteGroup
// is on EntityWriter and a querier does not have it.
//
// It is here for the day that stops being true. The moment a mutation is added
// to EntityQuerier — which is exactly when the read/write split would silently
// stop meaning anything — a read path reaching it fails the build. The three
// exemptions are pinned to setWrite registrations by
// TestDbModuleRegistrationsDeclareReadOrWrite, so they cannot quietly migrate to
// the read side while keeping their exemption.
func TestReadPathsCannotWrite(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "db_api.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse db_api.go: %v", err)
	}

	writeShaped := func(method string) bool {
		for _, prefix := range []string{"Create", "Update", "Patch", "Delete", "Add", "Remove"} {
			if strings.HasPrefix(method, prefix) {
				return true
			}
		}
		return false
	}

	// Names bound to a querier: `db := pm.querierFor(L)`, and any function
	// parameter declared as an EntityQuerier (the shared registrar helpers take
	// one).
	querierNames := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			call, ok := node.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "querierFor" {
				if id, ok := node.Lhs[0].(*ast.Ident); ok {
					querierNames[id.Name] = true
				}
			}
		case *ast.FuncType:
			if node.Params == nil {
				return true
			}
			for _, field := range node.Params.List {
				if exprText(fset, field.Type) != "EntityQuerier" {
					continue
				}
				for _, name := range field.Names {
					querierNames[name.Name] = true
				}
			}
		}
		return true
	})
	if len(querierNames) == 0 {
		t.Fatal("no querier-bound identifiers found in db_api.go; this test is no longer reading what it thinks it is")
	}

	var offenders []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recvName := ""
		switch recv := sel.X.(type) {
		case *ast.Ident:
			if querierNames[recv.Name] {
				recvName = recv.Name
			}
		case *ast.CallExpr:
			// The inline form: pm.querierFor(L).CreateSomething(...)
			if inner, ok := recv.Fun.(*ast.SelectorExpr); ok && inner.Sel.Name == "querierFor" {
				recvName = "pm.querierFor(...)"
			}
		}
		if recvName == "" {
			return true
		}
		if !writeShaped(sel.Sel.Name) || querierWriteMethods[sel.Sel.Name] {
			return true
		}
		offenders = append(offenders, fmt.Sprintf("%s.%s at %s", recvName, sel.Sel.Name, fset.Position(call.Pos())))
		return true
	})

	if len(offenders) > 0 {
		t.Errorf("a mutation is called through a querier: %s.\n"+
			"EntityQuerier declares every write method too, so reaching one through querierFor puts a\n"+
			"mutation on the db:read side of the split — a plugin granted only db:read would get it.\n"+
			"Use pm.writerFor and register with setWrite.", strings.Join(offenders, ", "))
	}
}

// insideMahBuilder reports whether a node sits in registerMahModule, which is
// the one function allowed to publish the mah table.
func insideMahBuilder(file *ast.File, n ast.Node) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name.Name != "registerMahModule" {
			continue
		}
		if n.Pos() >= fn.Body.Pos() && n.End() <= fn.Body.End() {
			return true
		}
	}
	return false
}

// isMetatableStrip reports whether an `L.G.Global` reference is the assignment
// that clears the globals metatable — the only reason to touch it.
func isMetatableStrip(file *ast.File, target ast.Node) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 {
			return true
		}
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Metatable" {
			return true
		}
		if target.Pos() >= assign.Pos() && target.End() <= assign.End() {
			found = true
		}
		return true
	})
	return found
}

// isCallee reports whether a selector is the thing being called, rather than a
// value being taken. `L.GetGlobal("mah")` is a call; `get := L.GetGlobal` is a
// method value that reaches the same place with no call site to match.
func isCallee(file *ast.File, sel *ast.SelectorExpr) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if call.Fun == ast.Expr(sel) {
			found = true
		}
		return true
	})
	return found
}

// nonRegisteringFunctions are the mah functions that write no per-plugin
// registry, and so need no liveness check. Everything else does.
//
// Stated as the exemption rather than the requirement on purpose: a new mah
// function must be named here to escape the check, so forgetting to think about
// it fails the build instead of passing silently. The reverse — an allowlist of
// functions that must check — makes a new registration invisible to this rule,
// which is how the same hole came back twice.
var nonRegisteringFunctions = map[string]bool{
	// jobs and their reporters write pm.actionJobs, which is not a per-plugin
	// registry: a job is keyed by its own id and is torn down with the plugin
	// through the in-flight WaitGroup.
	"start_job": true, "job_progress": true, "job_complete": true, "job_fail": true,
	// pure or plugin-local.
	"log": true, "abort": true, "get_setting": true, "html_escape": true, "sleep": true,
}

// registeringFunctions is every registration the mah table installs, minus the
// exemptions above.
func registeringFunctions() map[string]bool {
	out := map[string]bool{}
	for name := range setIfRegistrations {
		if !nonRegisteringFunctions[name] {
			out[name] = true
		}
	}
	for name := range setIfAnyRegistrations {
		if !nonRegisteringFunctions[name] {
			out[name] = true
		}
	}
	for name := range ungatedFunctions {
		if !nonRegisteringFunctions[name] {
			out[name] = true
		}
	}
	return out
}

// registryFields are the per-plugin registries on PluginManager. Teardown and
// shutdown must both account for every one of them.
var registryFields = []string{
	"hooks", "injections", "pages", "menuItems", "actions",
	"blockTypes", "displayTypes", "shortcodes", "docs", "apiEndpoints", "schedules",
}

// TestEveryRegistryIsTornDownAndReset pins the structural invariant the
// capability rules do not touch.
//
// Capability decisions have a thousand lines of guard here; the lifecycle that
// makes a grant *withdrawable* had none — and it produced the worst finding in
// four consecutive review rounds. A registry that `unregisterPluginLocked`
// forgets keeps a revoked plugin dispatchable; one that `Close` forgets keeps
// pointers to closed VMs and answers routes after shutdown (which is exactly
// how `docs` was found).
func TestEveryRegistryIsTornDownAndReset(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	bodyOf := func(name string) string {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name != name {
				continue
			}
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, fset, fn.Body); err != nil {
				t.Fatalf("print %s: %v", name, err)
			}
			return buf.String()
		}
		t.Fatalf("%s not found in manager.go; this test needs updating with whatever replaced it", name)
		return ""
	}

	unregister := bodyOf("unregisterPluginLocked")
	closeBody := bodyOf("Close")

	for _, field := range registryFields {
		// Mentioned *and* filtered on the owning state. `_ = pm.actions`
		// mentions it and removes nothing.
		mentions := strings.Contains(unregister, "pm."+field)
		filters := strings.Contains(unregister, "pm."+field+"[name]") ||
			strings.Contains(unregister, "range pm."+field)
		if !mentions || !filters {
			t.Errorf("unregisterPluginLocked does not remove entries from pm.%s by owning state.\n"+
				"Naming the field is not removing from it: a disabled plugin then keeps its entries there,\n"+
				"dispatchable, pointing at a VM that is about to close.", field)
		}
		if !strings.Contains(closeBody, "pm."+field) {
			t.Errorf("Close never resets pm.%s, so it keeps entries — and their *lua.LState pointers — after\n"+
				"every state has been closed.", field)
		}
	}

	// And the teardown must match on the state, never on the plugin name alone:
	// a name can be held by more than one VM over time, so a name-keyed delete
	// takes a replacement generation's registrations with it — which is round
	// 5's worst finding, and it was pinned for exactly one registry.
	var byName []string
	for _, decl := range file.Decls {
		fnDecl, ok := decl.(*ast.FuncDecl)
		if !ok || fnDecl.Body == nil || fnDecl.Name.Name != "unregisterPluginLocked" {
			continue
		}
		// An `if len(pm.X[name]) == 0 { delete(pm.X, name) }` is dropping an
		// emptied outer key, not deleting a plugin's registrations by name.
		emptyChecks := map[token.Pos]token.Pos{}
		ast.Inspect(fnDecl.Body, func(n ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if ok && strings.Contains(exprText(fset, ifStmt.Cond), "len(") {
				emptyChecks[ifStmt.Body.Pos()] = ifStmt.Body.End()
			}
			return true
		})
		insideEmptyCheck := func(n ast.Node) bool {
			for from, to := range emptyChecks {
				if n.Pos() >= from && n.End() <= to {
					return true
				}
			}
			return false
		}
		ast.Inspect(fnDecl.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if len(node.Args) != 2 {
					return true
				}
				if id, ok := node.Fun.(*ast.Ident); !ok || id.Name != "delete" {
					return true
				}
				if arg, ok := node.Args[1].(*ast.Ident); !ok || arg.Name != "name" {
					return true
				}
				target := exprText(fset, node.Args[0])
				for _, field := range registryFields {
					if target == "pm."+field && !insideEmptyCheck(node) {
						byName = append(byName, field)
					}
				}
			case *ast.AssignStmt:
				// `pm.actions[name] = nil` clears a plugin's registrations by
				// name just as thoroughly as delete does.
				if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
					return true
				}
				idx, ok := node.Lhs[0].(*ast.IndexExpr)
				if !ok {
					return true
				}
				if key, ok := idx.Index.(*ast.Ident); !ok || key.Name != "name" {
					return true
				}
				if exprText(fset, node.Rhs[0]) != "nil" {
					return true
				}
				target := exprText(fset, idx.X)
				for _, field := range registryFields {
					if target == "pm."+field {
						byName = append(byName, field+" (assigned nil)")
					}
				}
			}
			return true
		})
	}
	if len(byName) > 0 {
		t.Errorf("unregisterPluginLocked deletes %s by plugin name alone. A name can be held by more than\n"+
			"one VM over time, so that removes a replacement generation's registrations along with the dead\n"+
			"one's. Filter on the owning state, the way hooks and injections do.", strings.Join(byName, ", "))
	}
}

// TestRegistrationFunctionsCheckLiveness pins the other half: a state that is no
// longer live may not register.
//
// Without it a revoked plugin's worker keeps a fully-installed mah table and can
// re-register after its own teardown swept — including overwriting a
// replacement generation's entry under the same key.
func TestRegistrationFunctionsCheckLiveness(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == "registerMahModule" {
			fn = d
			break
		}
	}
	if fn == nil {
		t.Fatal("registerMahModule not found")
	}

	mustCheckLiveness := registeringFunctions()
	var checked, unchecked []string
	seen := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || (id.Name != "setIf" && id.Name != "setIfAny") || len(call.Args) < 3 {
			return true
		}
		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok {
			return true
		}
		name := strings.Trim(lit.Value, `"`)
		// Pinned by name, not detected by what the body happens to mention. A
		// registration that writes its registry through a helper does not name
		// the map, and a "does it look like it writes a registry" test then
		// skips it silently.
		if !mustCheckLiveness[name] {
			return true
		}
		seen[name] = true
		body, ok := call.Args[2].(*ast.FuncLit)
		if !ok {
			// A handler passed by name has no body here to inspect — the same
			// hole this rule's db-side twin already closes.
			unchecked = append(unchecked, name+" (handler must be written inline, not passed by name)")
			return true
		}
		// Not "mentions it" — `_ = pm.stateMayRegisterLocked(L)` mentions it and
		// gates nothing. The check has to be a branch that returns before the
		// write.
		// Polarity and position: `if pm.stateMayRegisterLocked(L) { return }`
		// refuses exactly the live states, and a check placed after the write
		// refuses nothing at all.
		gated := false
		var guardEnd token.Pos
		ast.Inspect(body.Body, func(inner ast.Node) bool {
			ifStmt, ok := inner.(*ast.IfStmt)
			if !ok {
				return true
			}
			// The condition must be exactly the negated check. Anything else
			// conjoined with it — `false &&`, a flag, a capability test — can
			// make it never fire while still reading as a guard.
			if exprText(fset, ifStmt.Cond) != "!pm.stateMayRegisterLocked(L)" {
				return true
			}
			for _, stmt := range ifStmt.Body.List {
				if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
					gated = true
					guardEnd = ifStmt.End()
				}
			}
			return true
		})
		if gated {
			// Every write to a registry must come after it.
			ast.Inspect(body.Body, func(inner ast.Node) bool {
				sel, ok := inner.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				for _, field := range registryFields {
					if sel.Sel.Name == field && inner.Pos() < guardEnd {
						gated = false
					}
				}
				return true
			})
		}
		if gated {
			checked = append(checked, name)
		} else {
			unchecked = append(unchecked, name)
		}
		return true
	})

	if len(unchecked) > 0 {
		t.Errorf("mah.%s writes a registry without calling stateMayRegisterLocked.\n"+
			"A revoked plugin's worker keeps a fully-installed mah table until it finishes, so without that\n"+
			"check it can register after its own teardown swept — and overwrite a replacement generation's\n"+
			"entry under the same key.", strings.Join(unchecked, ", mah."))
	}
	for name := range mustCheckLiveness {
		if !seen[name] {
			t.Errorf("mah.%s should write a registry but was not found; either it is gone, or it needs an\n"+
				"entry in nonRegisteringFunctions saying why it writes none.", name)
		}
	}
	if len(checked) != len(mustCheckLiveness) {
		t.Errorf("%d of %d registration functions check liveness", len(checked), len(mustCheckLiveness))
	}
}

// TestRegistrationsStampTheMainState pins the other half of round 7's fix.
//
// A registration made inside a coroutine that stores the coroutine's own
// LState is unmatched by dispatch and by teardown alike: it can never fire and
// can never be removed. Nothing else in this file would notice — the
// registration still checks liveness, and its registry is still torn down.
func TestRegistrationsStampTheMainState(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == "registerMahModule" {
			fn = d
			break
		}
	}
	if fn == nil {
		t.Fatal("registerMahModule not found")
	}

	// Locals bound from mainState(L) — a stamp may store one of these, and
	// nothing else. `owner := L` would otherwise read as a valid stamp.
	ownerBindings := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		if exprText(fset, assign.Rhs[0]) == "mainState(L)" {
			ownerBindings[id.Name] = true
		}
		return true
	})

	var bare []string
	var stamped int
	// Every place a state is stored: `state: X`, `State: X`, `x.state = X`,
	// `x.State = X`.
	record := func(pos token.Pos, value ast.Expr) {
		text := exprText(fset, value)
		if text == "mainState(L)" {
			stamped++
			return
		}
		if text == "L" {
			bare = append(bare, fset.Position(pos).String())
			return
		}
		if ownerBindings[text] {
			stamped++
			return
		}
		bare = append(bare, fset.Position(pos).String()+" (stores "+text+")")
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			if k, ok := node.Key.(*ast.Ident); ok && (k.Name == "state" || k.Name == "State") {
				record(node.Pos(), node.Value)
			}
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			if sel, ok := node.Lhs[0].(*ast.SelectorExpr); ok && (sel.Sel.Name == "state" || sel.Sel.Name == "State") {
				record(node.Pos(), node.Rhs[0])
			}
		}
		return true
	})

	if len(bare) > 0 {
		t.Errorf("a registration stores the raw L rather than mainState(L) at %s.\n"+
			"Dispatch and teardown are both keyed on the main state, so a registration made inside a\n"+
			"coroutine would be stamped with the coroutine's state — unmatched by both, so it could never\n"+
			"fire and could never be removed.", strings.Join(bare, ", "))
	}
	// Every registration in the mah table stamps a state, so the count is the
	// number of registrations — not a floor that absorbs one regression before
	// firing.
	if stamped < len(registeringFunctions()) {
		t.Errorf("found %d state stamps for %d registering functions; one of them no longer stamps at all",
			stamped, len(registeringFunctions()))
	}
}

// TestTeardownUnregistersProcessGlobalBlockTypes pins the one registry that
// outlives the manager.
//
// Block types live in a process-global map as well as in pm.blockTypes, so
// filtering the slice without unregistering the global leaves a type backed by
// a closed VM — and every existing note block of that type stops rendering, for
// every user, until a restart. Nothing else in this file would notice.
func TestTeardownUnregistersProcessGlobalBlockTypes(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	for _, fnName := range []string{"unregisterPluginLocked", "Close"} {
		found := false
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name != fnName {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "UnregisterBlockType" {
					found = true
				}
				return true
			})
		}
		if !found {
			t.Errorf("%s does not call block_types.UnregisterBlockType.\n"+
				"Block types are also in a process-global registry, so dropping only the per-plugin slice\n"+
				"leaves a type backed by a closed VM and breaks every existing note block of that type.", fnName)
		}
	}
}

// TestConsentIsEnforcedBeforeAnyHostFunctionIsInstalled pins the ordering that
// makes a consent refusal mean anything.
//
// registerMahModule is the only place a mah key is ever set, and init() is the
// first Lua that could observe one. A consent check that ran after either would
// still report the refusal, and would still be green in every behavioural test
// that only asserts EnablePlugin returns an error — while the plugin had
// already been handed the capabilities it was being refused.
//
// The rule is positional rather than "is called somewhere", because "somewhere"
// is exactly the property that does not hold.
func TestConsentIsEnforcedBeforeAnyHostFunctionIsInstalled(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == "loadPlugin" {
			fn = d
			break
		}
	}
	if fn == nil {
		t.Fatal("loadPlugin not found in plugin_system/manager.go; this test needs updating with whatever replaced it")
	}

	var consentPos, registerPos, initCallPos token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch exprText(fset, call.Fun) {
		case "pm.enforceConsent":
			if !consentPos.IsValid() {
				consentPos = call.Pos()
			}
		case "pm.registerMahModule":
			if !registerPos.IsValid() {
				registerPos = call.Pos()
			}
		case "L.CallByParam":
			// The init() invocation.
			if !initCallPos.IsValid() {
				initCallPos = call.Pos()
			}
		}
		return true
	})

	if !consentPos.IsValid() {
		t.Fatal("loadPlugin never calls pm.enforceConsent: a plugin can now widen its own grants by editing plugin.lua")
	}
	if !registerPos.IsValid() {
		t.Fatal("loadPlugin no longer calls pm.registerMahModule; this test needs updating with whatever installs the mah table now")
	}
	if consentPos > registerPos {
		t.Errorf("pm.enforceConsent (line %d) runs AFTER pm.registerMahModule (line %d): "+
			"the refused plugin has already been handed its host functions",
			fset.Position(consentPos).Line, fset.Position(registerPos).Line)
	}
	if initCallPos.IsValid() && consentPos > initCallPos {
		t.Errorf("pm.enforceConsent (line %d) runs AFTER init() is called (line %d): "+
			"the refused plugin has already run",
			fset.Position(consentPos).Line, fset.Position(initCallPos).Line)
	}

	// A refusal must abandon the VM. Returning without it leaves the state in
	// pm.vmLocks and pm.loading, holding its own lock forever.
	consentStmt := enclosingIfStmt(fn.Body, consentPos)
	if consentStmt == nil {
		t.Fatal("the pm.enforceConsent call is not inside an if-statement, so its error cannot be being checked")
	}
	body := nodeText(fset, consentStmt.Body)
	if !strings.Contains(body, "abandon()") {
		t.Errorf("the pm.enforceConsent failure branch does not call abandon(): the VM, its lock entry "+
			"and its loading marker are leaked on refusal. Branch was:\n%s", body)
	}
	if !strings.Contains(body, "return") {
		t.Errorf("the pm.enforceConsent failure branch does not return, so the load continues after a refusal. Branch was:\n%s", body)
	}
}

// enclosingIfStmt finds the innermost if-statement whose init or condition
// contains pos.
func enclosingIfStmt(body *ast.BlockStmt, pos token.Pos) *ast.IfStmt {
	var found *ast.IfStmt
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		stmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if stmt.Init != nil && pos >= stmt.Init.Pos() && pos <= stmt.Init.End() {
			found = stmt
			return false
		}
		if stmt.Cond != nil && pos >= stmt.Cond.Pos() && pos <= stmt.Cond.End() {
			found = stmt
			return false
		}
		return true
	})
	return found
}

// nodeText renders any AST node back to source, for matching on statement
// bodies rather than expressions.
func nodeText(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		return ""
	}
	return buf.String()
}

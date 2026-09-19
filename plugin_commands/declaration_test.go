package plugin_commands

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildInvocationPreservesArgvAndRedactsParameterElements(t *testing.T) {
	declaration := Declaration{
		Name:            "download",
		Argv:            []string{"tool", "--paths", "{{exchange_dir}}", "--format", "{{format}}", "--", "{{url}}"},
		Timeout:         2 * time.Hour,
		SensitiveParams: []string{"url"},
	}
	params := map[string]string{
		"format": "best video; echo not-a-shell",
		"url":    "https://example.test/watch?v=secret&x=$(whoami)",
	}

	got, err := BuildInvocation(declaration, params, "/private/exchange/run-1")
	if err != nil {
		t.Fatalf("BuildInvocation: %v", err)
	}
	wantArgv := []string{
		"tool", "--paths", "/private/exchange/run-1", "--format", "best video; echo not-a-shell",
		"--", "https://example.test/watch?v=secret&x=$(whoami)",
	}
	if !reflect.DeepEqual(got.Argv, wantArgv) {
		t.Fatalf("Argv = %#v, want %#v", got.Argv, wantArgv)
	}
	wantRedacted := append([]string(nil), wantArgv...)
	wantRedacted[len(wantRedacted)-1] = "[redacted]"
	if !reflect.DeepEqual(got.RedactedArgv, wantRedacted) {
		t.Fatalf("RedactedArgv = %#v, want %#v", got.RedactedArgv, wantRedacted)
	}
	wantView := map[string]string{
		"format": params["format"],
		"url":    "[redacted]",
	}
	if !reflect.DeepEqual(got.ParamView, wantView) {
		t.Fatalf("ParamView = %#v, want %#v", got.ParamView, wantView)
	}
	if params["url"] != "https://example.test/watch?v=secret&x=$(whoami)" {
		t.Fatal("BuildInvocation mutated the caller's params map")
	}
}

func TestBuildInvocationRejectsMissingEmptyAndReservedParams(t *testing.T) {
	declaration := Declaration{
		Name:    "download",
		Argv:    []string{"tool", "{{exchange_dir}}", "{{url}}"},
		Timeout: DefaultTimeout,
	}
	for name, params := range map[string]map[string]string{
		"missing":           {},
		"empty":             {"url": ""},
		"reserved override": {"url": "ok", "exchange_dir": "/attacker"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildInvocation(declaration, params, "/trusted"); err == nil {
				t.Fatal("expected BuildInvocation to refuse the params")
			}
		})
	}
	if _, err := BuildInvocation(declaration, map[string]string{"url": "ok"}, ""); err == nil {
		t.Fatal("expected an empty host-filled exchange_dir to be refused")
	}
}

func TestBuildInvocationParameterLimits(t *testing.T) {
	declarationFor := func(count int) Declaration {
		argv := []string{"tool"}
		for i := 0; i < count; i++ {
			argv = append(argv, "{{"+fmt.Sprintf("p%02d", i)+"}}")
		}
		return Declaration{Name: "limits", Argv: argv, Timeout: DefaultTimeout}
	}
	paramsFor := func(count, size int) map[string]string {
		params := make(map[string]string, count)
		for i := 0; i < count; i++ {
			params[fmt.Sprintf("p%02d", i)] = strings.Repeat("x", size)
		}
		return params
	}

	if _, err := BuildInvocation(declarationFor(MaxParameters), paramsFor(MaxParameters, 1), "/exchange"); err != nil {
		t.Fatalf("%d params must be accepted: %v", MaxParameters, err)
	}
	if _, err := BuildInvocation(declarationFor(MaxParameters+1), paramsFor(MaxParameters+1, 1), "/exchange"); err == nil {
		t.Fatalf("%d params must be refused", MaxParameters+1)
	}

	one := Declaration{Name: "value", Argv: []string{"tool", "{{value}}"}, Timeout: DefaultTimeout}
	if _, err := BuildInvocation(one, map[string]string{"value": strings.Repeat("x", MaxParameterBytes)}, "/exchange"); err != nil {
		t.Fatalf("a %d-byte value must be accepted: %v", MaxParameterBytes, err)
	}
	if _, err := BuildInvocation(one, map[string]string{"value": strings.Repeat("x", MaxParameterBytes+1)}, "/exchange"); err == nil {
		t.Fatalf("a %d-byte value must be refused", MaxParameterBytes+1)
	}

	aggregateCount := MaxAggregateParameterBytes / MaxParameterBytes
	if _, err := BuildInvocation(declarationFor(aggregateCount), paramsFor(aggregateCount, MaxParameterBytes), "/exchange"); err != nil {
		t.Fatalf("exact aggregate limit must be accepted: %v", err)
	}
	over := paramsFor(aggregateCount+1, MaxParameterBytes)
	over[fmt.Sprintf("p%02d", aggregateCount)] = "x"
	if _, err := BuildInvocation(declarationFor(aggregateCount+1), over, "/exchange"); err == nil {
		t.Fatal("aggregate parameter bytes above the limit must be refused")
	}
}

func TestValidateDeclarationRejectsUnsafeTemplates(t *testing.T) {
	valid := func(argv ...string) Declaration {
		return Declaration{Name: "download", Argv: argv, Timeout: DefaultTimeout}
	}
	cases := map[string]Declaration{
		"empty argv":          valid(),
		"placeholder argv0":   valid("{{tool}}"),
		"absolute argv0":      valid("/usr/bin/tool"),
		"relative path argv0": valid("bin/tool"),
		"windows path argv0":  valid(`bin\\tool.exe`),
		"dotdot argv0":        valid("tool..backup"),
		"leading dash argv0":  valid("-tool"),
		"partial prefix":      valid("tool", "prefix-{{url}}"),
		"partial suffix":      valid("tool", "{{url}}-suffix"),
		"unclosed open":       valid("tool", "{{url"),
		"unopened close":      valid("tool", "url}}"),
		"bad placeholder":     valid("tool", "{{URL}}"),
		"timeout zero":        {Name: "download", Argv: []string{"tool"}},
		"timeout over cap":    {Name: "download", Argv: []string{"tool"}, Timeout: MaxTimeout + time.Second},
		"bad command slug":    {Name: "Download Job", Argv: []string{"tool"}, Timeout: DefaultTimeout},
	}
	for name, declaration := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDeclaration(declaration); err == nil {
				t.Fatal("expected declaration to be refused")
			}
		})
	}
}

func TestValidateDeclarationChecksSensitiveParameterNames(t *testing.T) {
	base := Declaration{
		Name:            "download",
		Argv:            []string{"tool", "{{url}}", "{{format}}"},
		Timeout:         DefaultTimeout,
		SensitiveParams: []string{"url"},
	}
	if err := ValidateDeclaration(base); err != nil {
		t.Fatalf("valid declaration refused: %v", err)
	}
	for name, sensitive := range map[string][]string{
		"unknown":      {"cookie"},
		"host filled":  {"exchange_dir"},
		"invalid name": {"URL"},
		"duplicate":    {"url", "url"},
	} {
		t.Run(name, func(t *testing.T) {
			declaration := base
			declaration.SensitiveParams = sensitive
			if err := ValidateDeclaration(declaration); err == nil {
				t.Fatal("expected sensitive_params to be refused")
			}
		})
	}
}

func TestSameDeclarationsUsesCommandNamesAndPositionalArgv(t *testing.T) {
	a := []Declaration{
		{Name: "one", Argv: []string{"tool", "--", "{{url}}"}, Timeout: time.Hour, SensitiveParams: []string{"url", "token"}},
		{Name: "two", Argv: []string{"other"}, Timeout: 2 * time.Hour},
	}
	b := []Declaration{
		{Name: "two", Argv: []string{"other"}, Timeout: 2 * time.Hour},
		{Name: "one", Argv: []string{"tool", "--", "{{url}}"}, Timeout: time.Hour, SensitiveParams: []string{"token", "url"}},
	}
	if !SameDeclarations(a, b) {
		t.Fatal("declaration order and sensitive_params order must not affect identity")
	}
	changed := append([]Declaration(nil), b...)
	changed[1] = b[1]
	changed[1].Argv = []string{"tool", "{{url}}", "--"}
	if SameDeclarations(a, changed) {
		t.Fatal("argv order must affect identity")
	}
	changed = append([]Declaration(nil), b...)
	changed[1] = b[1]
	changed[1].Timeout++
	if SameDeclarations(a, changed) {
		t.Fatal("timeout must affect identity")
	}
	changed = append([]Declaration(nil), b...)
	changed[1] = b[1]
	changed[1].SensitiveParams = []string{"url"}
	if SameDeclarations(a, changed) {
		t.Fatal("sensitive_params membership must affect identity")
	}
}

func TestShellJoinIsDisplayOnlyAndQuotesUnsafeElements(t *testing.T) {
	got := ShellJoin([]string{"tool", "--", "plain/value", "two words", "it's", "", "{{url}}"})
	want := `tool -- plain/value 'two words' 'it'\''s' '' '{{url}}'`
	if got != want {
		t.Fatalf("ShellJoin = %q, want %q", got, want)
	}

	typeOfInvocation := reflect.TypeOf(Invocation{})
	wantFields := []string{"Argv", "RedactedArgv", "ParamView"}
	if typeOfInvocation.NumField() != len(wantFields) {
		t.Fatalf("Invocation has %d fields; launch-facing data must remain argv-only", typeOfInvocation.NumField())
	}
	for i, want := range wantFields {
		if got := typeOfInvocation.Field(i).Name; got != want {
			t.Fatalf("Invocation field %d = %q, want %q", i, got, want)
		}
	}
}

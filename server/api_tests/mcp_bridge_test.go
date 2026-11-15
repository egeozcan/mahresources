package api_tests

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mahresources/cmd/mr/mcpbridge"
)

// bridged connects a downstream client to an mr-mcp bridge that forwards to
// env's /mcp endpoint, the way `mr mcp` does over stdio.
func bridged(t *testing.T, env *mcpEnv, protocol string, answer *mcp.ElicitResult) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	httpClient := &http.Client{Transport: headerTransport{
		headers: map[string]string{"Authorization": env.bearer}, base: http.DefaultTransport,
	}}
	remote, err := mcpbridge.Connect(ctx, mcpbridge.Options{Endpoint: env.srv.URL + "/mcp", HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("bridge connect: %v", err)
	}
	t.Cleanup(func() { _ = remote.Close() })
	server, err := mcpbridge.NewServer(ctx, remote, "test")
	if err != nil {
		t.Fatalf("bridge server: %v", err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("bridge serve: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	copts := &mcp.ClientOptions{}
	if answer != nil {
		copts.ElicitationHandler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) { return answer, nil }
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "downstream", Version: "1"}, copts).
		Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	if err != nil {
		t.Fatalf("downstream connect (%s): %v", protocol, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPBridge_ConfirmsForEveryProtocol(t *testing.T) {
	env := newMCPEnv(t, true)
	for _, protocol := range []string{mcpLegacy, mcpNew} {
		t.Run(protocol, func(t *testing.T) {
			note := env.tc.CreateDummyNote("bridged " + protocol)
			cs := bridged(t, env, protocol, accept)

			tools, err := cs.ListTools(context.Background(), nil)
			if err != nil || len(tools.Tools) < 20 {
				t.Fatalf("bridged tools: %v (%d)", err, len(tools.Tools))
			}
			res := callTool(t, cs, "delete", map[string]any{"entity": "note", "id": note.ID})
			if res.IsError {
				t.Fatalf("bridged confirmed delete failed: %s", resultText(res))
			}
			if env.noteExists(t, note.ID) {
				t.Fatal("note survived a confirmed bridged delete")
			}
		})
	}
}

func TestMCPBridge_DeclineAndNoElicitationKeep(t *testing.T) {
	env := newMCPEnv(t, true)
	for _, c := range []struct {
		name     string
		protocol string
		answer   *mcp.ElicitResult
		want     string
	}{
		{"legacy decline", mcpLegacy, &mcp.ElicitResult{Action: "decline"}, "Cancelled"},
		{"new decline", mcpNew, &mcp.ElicitResult{Action: "decline"}, "Cancelled"},
		{"legacy no elicitation", mcpLegacy, nil, "cannot show"},
		{"new no elicitation", mcpNew, nil, "cannot show"},
	} {
		t.Run(c.name, func(t *testing.T) {
			note := env.tc.CreateDummyNote("kept " + c.name)
			cs := bridged(t, env, c.protocol, c.answer)
			res := callTool(t, cs, "delete", map[string]any{"entity": "note", "id": note.ID})
			if !strings.Contains(resultText(res), c.want) {
				t.Fatalf("result = %v %q, want %q", res.IsError, resultText(res), c.want)
			}
			if !env.noteExists(t, note.ID) {
				t.Fatal("note deleted without an accepted confirmation")
			}
		})
	}
}

func TestMCPBridge_ForwardsReads(t *testing.T) {
	env := newMCPEnv(t, true)
	note := env.tc.CreateDummyNote("bridged read")
	cs := bridged(t, env, mcpLegacy, nil)
	res := callTool(t, cs, "get_note", map[string]any{"id": note.ID})
	if res.IsError || !strings.Contains(resultText(res), "bridged read") {
		t.Fatalf("bridged get_note = %s", resultText(res))
	}
}

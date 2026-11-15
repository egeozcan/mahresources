// Package mcpbridge serves a remote mahresources MCP endpoint over stdio, for
// MCP clients that only launch local servers.
//
// The bridge holds no state and adds no tools. It mirrors the remote tool
// list and forwards each call. Confirmations pass through untouched: when the
// server answers input_required, the bridge hands the same input requests and
// requestState to its own client, and forwards the retry's answers back. A
// 2026-07-28 client fulfils them itself; for an older client, the SDK's
// server-side middleware asks it over the stdio session, which is stateful, so
// an older client that supports elicitation can confirm through the bridge
// even though it could not confirm against the stateless HTTP endpoint.
package mcpbridge

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures a bridge.
type Options struct {
	// Endpoint is the remote MCP URL, e.g. http://localhost:8181/mcp.
	Endpoint string
	// HTTPClient sends the remote requests; it carries the credentials.
	HTTPClient *http.Client
	// Version is reported as the bridge's version.
	Version string
}

// Run connects to the remote endpoint and serves it on local until ctx ends
// or the local client disconnects.
func Run(ctx context.Context, opts Options, local mcp.Transport) error {
	remote, err := Connect(ctx, opts)
	if err != nil {
		return err
	}
	defer remote.Close()

	server, err := NewServer(ctx, remote, opts.Version)
	if err != nil {
		return err
	}
	return server.Run(ctx, local)
}

// Connect opens the remote session as a 2026-07-28 client that advertises
// form elicitation and leaves input requests to the bridge's own client.
func Connect(ctx context.Context, opts Options) (*mcp.ClientSession, error) {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mr-mcp-bridge", Version: opts.Version}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{Form: &mcp.FormElicitationCapabilities{}},
		},
		MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
	})
	transport := &mcp.StreamableClientTransport{Endpoint: opts.Endpoint, HTTPClient: opts.HTTPClient}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", opts.Endpoint, err)
	}
	return session, nil
}

// NewServer builds the local server that mirrors remote's tools.
func NewServer(ctx context.Context, remote *mcp.ClientSession, version string) (*mcp.Server, error) {
	if version == "" {
		version = "dev"
	}
	init := remote.InitializeResult()
	instructions := ""
	if init != nil {
		instructions = init.Instructions
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "mahresources", Title: "Mahresources (via mr)", Version: version},
		&mcp.ServerOptions{Instructions: instructions})
	for tool, err := range remote.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list remote tools: %w", err)
		}
		server.AddTool(tool, forward(remote, tool.Name))
	}
	return server, nil
}

func forward(remote *mcp.ClientSession, name string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := remote.CallTool(ctx, &mcp.CallToolParams{
			Name:           name,
			Arguments:      req.Params.Arguments,
			InputResponses: req.Params.InputResponses,
			RequestState:   req.Params.RequestState,
		})
		if err != nil {
			return nil, err
		}
		if !res.NeedsInput() {
			return res, nil
		}
		if !canElicit(req) {
			return refusal(res), nil
		}
		return &mcp.CallToolResult{InputRequests: res.InputRequests, RequestState: res.RequestState}, nil
	}
}

func canElicit(req *mcp.CallToolRequest) bool {
	caps := req.ClientCapabilities()
	return caps != nil && caps.Elicitation != nil && (caps.Elicitation.Form != nil || caps.Elicitation.URL == nil)
}

// refusal explains, with the server's own preview, why a confirmation could
// not be asked. Nothing ran: the server only acts on a confirmed retry.
func refusal(res *mcp.CallToolResult) *mcp.CallToolResult {
	var previews []string
	for _, ir := range res.InputRequests {
		if p, ok := ir.(*mcp.ElicitParams); ok && p.Message != "" {
			previews = append(previews, p.Message)
		}
	}
	msg := strings.Join(previews, "\n\n")
	if msg != "" {
		msg += "\n\n"
	}
	msg += "This action needs a confirmation this client cannot show, so it was refused and nothing was changed."
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

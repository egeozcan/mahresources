package commands

import (
	"embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/mcpbridge"
)

//go:embed mcp_help/*.md
var mcpHelpFS embed.FS

// NewMCPCmd returns "mr mcp", a stdio bridge to the server's MCP endpoint.
func NewMCPCmd(c *client.Client) *cobra.Command {
	help := helptext.Load(mcpHelpFS, "mcp_help/mcp.md")
	return &cobra.Command{
		Use:         "mcp",
		Short:       "Serve the server's MCP endpoint over stdio",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcpbridge.Run(cmd.Context(), mcpbridge.Options{
				Endpoint:   c.BaseURL + "/mcp",
				HTTPClient: c.HTTPClient(),
			}, &mcp.StdioTransport{})
		},
	}
}

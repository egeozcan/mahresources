package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"
)

// Categories historically had only name/description edits. Partial carrier edits
// also let a caller update or clear template slots without recreating the carrier.
func newTemplateCarrierEditCmd(c *client.Client, opts *output.Options, member, endpoint string, help helptext.Help) *cobra.Command {
	var id uint
	var slots *customSlotFlags
	values := make(map[string]*string)
	fields := []struct{ flag, field, usage string }{
		{"name", "Name", "Carrier name"},
		{"description", "Description", "Carrier description"},
		{"meta-schema", "MetaSchema", "JSON Schema defining member metadata"},
		{"section-config", "SectionConfig", "JSON controlling member detail-page sections"},
	}
	cmd := &cobra.Command{
		Use: "edit", Short: "Edit a " + carrierNouns[member], Long: help.Long, Example: help.Example, Annotations: help.Annotations, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == 0 {
				return fmt.Errorf("--id must be positive")
			}
			body := map[string]any{"ID": id}
			for _, field := range fields {
				if cmd.Flags().Changed(field.flag) {
					body[field.field] = *values[field.flag]
				}
			}
			if err := slots.applyChangedAny(body); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := c.Post(endpoint, nil, body, &raw); err != nil {
				return err
			}
			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Updated " + carrierNouns[member] + ".")
			}
			return nil
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "Carrier ID (required)")
	cmd.MarkFlagRequired("id")
	for _, field := range fields {
		values[field.flag] = cmd.Flags().String(field.flag, "", field.usage)
	}
	slots = registerCustomSlotFlags(cmd, member)
	return cmd
}

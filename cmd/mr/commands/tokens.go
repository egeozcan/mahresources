package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
)

var tokenExitCodes = map[string]string{"exitCodes": "0 success; 1 error (not authenticated, network error, or token not found)"}

type apiTokenView struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	LastUsedAt string `json:"lastUsedAt"`
	ExpiresAt  string `json:"expiresAt"`
}

// NewTokensCmd builds the `mr token` command group for managing the current
// user's own API tokens.
func NewTokensCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "token",
		Short:       "Manage your API tokens",
		Long:        "List, create, and revoke the API tokens for the authenticated account. Tokens are bearer credentials used by the CLI and other non-browser clients.",
		Annotations: tokenExitCodes,
	}
	cmd.AddCommand(newTokenListCmd(c, opts))
	cmd.AddCommand(newTokenCreateCmd(c, opts))
	cmd.AddCommand(newTokenRevokeCmd(c, opts))
	return cmd
}

func newTokenListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your API tokens",
		Long:  "Show the API tokens for the authenticated account, including their id, label, and display prefix. The secret value itself is never shown after creation.",
		Example: strings.Join([]string{
			"  # List your tokens",
			"  mr token list",
			"",
			"  # As raw JSON",
			"  mr token list --json",
		}, "\n"),
		Annotations: tokenExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/account/tokens", nil, &raw); err != nil {
				return err
			}
			var tokens []apiTokenView
			_ = json.Unmarshal(raw, &tokens)
			rows := make([][]string, 0, len(tokens))
			for _, t := range tokens {
				rows = append(rows, []string{strconv.FormatUint(uint64(t.ID), 10), t.Name, t.Prefix, t.LastUsedAt})
			}
			output.Print(*opts, []string{"ID", "Name", "Prefix", "LastUsed"}, rows, raw)
			return nil
		},
	}
}

func newTokenCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, expiresIn string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a new API token",
		Long:  "Create a new API token for the authenticated account and print the secret once. Store it securely; it cannot be retrieved again.",
		Example: strings.Join([]string{
			"  # Create a token labelled 'ci'",
			"  mr token create --name ci",
			"",
			"  # Create a token that expires in 30 days",
			"  mr token create --name temp --expires-in 720h",
		}, "\n"),
		Annotations: tokenExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"name": name}
			if expiresIn != "" {
				body["expiresIn"] = expiresIn
			}
			var out struct {
				Token  string `json:"token"`
				ID     uint   `json:"id"`
				Name   string `json:"name"`
				Prefix string `json:"prefix"`
			}
			if err := c.Post("/v1/account/tokens", nil, body, &out); err != nil {
				return err
			}
			raw, _ := json.Marshal(out)
			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(out.ID), 10)},
				{Key: "Name", Value: out.Name},
				{Key: "Token", Value: out.Token},
			}, raw)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "mr cli", "Label for the token")
	cmd.Flags().StringVar(&expiresIn, "expires-in", "", "Optional expiry as a Go duration (e.g. 720h); empty = never")
	return cmd
}

func newTokenRevokeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke one of your API tokens",
		Long:  "Invalidate an API token by its id so it can no longer authenticate. This affects every client using that token.",
		Example: strings.Join([]string{
			"  # Revoke token 3",
			"  mr token revoke 3",
			"",
			"  # Revoke after listing",
			"  mr token revoke 5",
		}, "\n"),
		Annotations: tokenExitCodes,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			if err := c.Post("/v1/account/tokens/delete", url.Values{"id": {strconv.FormatUint(id, 10)}}, nil, nil); err != nil {
				return err
			}
			output.PrintMessage("Token revoked.")
			return nil
		},
	}
}

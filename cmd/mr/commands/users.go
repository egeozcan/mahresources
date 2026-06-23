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

var userExitCodes = map[string]string{"exitCodes": "0 success; 1 error (not authenticated, insufficient role, validation error, or user not found)"}

// userView is the subset of the User model the CLI renders.
type userView struct {
	ID           uint   `json:"ID"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	Role         string `json:"role"`
	ScopeGroupId *uint  `json:"scopeGroupId"`
	Disabled     bool   `json:"disabled"`
	LastLoginAt  string `json:"lastLoginAt"`
}

func scopeLabel(id *uint) string {
	if id == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*id), 10)
}

// NewUsersCmd builds the `mr user` admin command group for managing accounts.
// Requires an admin identity (the server enforces this); the no-auth deployment
// treats every caller as admin, so these work there too.
func NewUsersCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "user",
		Short:       "Administer user accounts (admin only)",
		Long:        "List, inspect, create, update, and delete user accounts. These commands target the admin user-management API and require an administrator identity. When the server runs without auth every request is an implicit admin, so they work there too.",
		Annotations: userExitCodes,
	}
	cmd.AddCommand(newUserListCmd(c, opts))
	cmd.AddCommand(newUserGetCmd(c, opts))
	cmd.AddCommand(newUserCreateCmd(c, opts))
	cmd.AddCommand(newUserUpdateCmd(c, opts))
	cmd.AddCommand(newUserDeleteCmd(c, opts))
	return cmd
}

func newUserListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var offset, limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List user accounts",
		Long:  "Show all user accounts with their id, username, role, scope group, and disabled state. Password hashes are never returned.",
		Example: strings.Join([]string{
			"  # List all users",
			"  mr user list",
			"",
			"  # As raw JSON",
			"  mr user list --json",
		}, "\n"),
		Annotations: userExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			var raw json.RawMessage
			if err := c.Get("/v1/users", q, &raw); err != nil {
				return err
			}
			var users []userView
			_ = json.Unmarshal(raw, &users)
			rows := make([][]string, 0, len(users))
			for _, u := range users {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(u.ID), 10), u.Username, u.Role,
					scopeLabel(u.ScopeGroupId), strconv.FormatBool(u.Disabled),
				})
			}
			output.Print(*opts, []string{"ID", "Username", "Role", "Scope", "Disabled"}, rows, raw)
			return nil
		},
	}
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of users to skip")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum users to return (0 = server default)")
	return cmd
}

func newUserGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single user account",
		Long:  "Fetch one user account by its numeric id and print its details. Useful before an update to confirm the current role and scope.",
		Example: strings.Join([]string{
			"  # Show user 4",
			"  mr user get 4",
			"",
			"  # As raw JSON",
			"  mr user get 4 --json",
		}, "\n"),
		Annotations: userExitCodes,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			var raw json.RawMessage
			if err := c.Get("/v1/user", url.Values{"id": {strconv.FormatUint(id, 10)}}, &raw); err != nil {
				return err
			}
			var u userView
			_ = json.Unmarshal(raw, &u)
			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(u.ID), 10)},
				{Key: "Username", Value: u.Username},
				{Key: "DisplayName", Value: u.DisplayName},
				{Key: "Role", Value: u.Role},
				{Key: "Scope", Value: scopeLabel(u.ScopeGroupId)},
				{Key: "Disabled", Value: strconv.FormatBool(u.Disabled)},
			}, raw)
			return nil
		},
	}
}

func newUserCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var username, password, role, displayName string
	var scopeGroup uint
	var disabled bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user account",
		Long:  "Create a new user account with a username, password, and role (admin, editor, user, or guest). Guests require a scope group; users may optionally have one; admins and editors must not.",
		Example: strings.Join([]string{
			"  # Create an editor",
			"  mr user create --username alice --password s3cret --role editor",
			"",
			"  # Create a guest confined to group 7",
			"  mr user create --username bob --password s3cret --role guest --scope-group 7",
		}, "\n"),
		Annotations: userExitCodes,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"username":    username,
				"password":    password,
				"role":        role,
				"displayName": displayName,
				"disabled":    disabled,
			}
			if cmd.Flags().Changed("scope-group") {
				body["scopeGroupId"] = scopeGroup
			}
			var raw json.RawMessage
			if err := c.Post("/v1/users", nil, body, &raw); err != nil {
				return err
			}
			var u userView
			_ = json.Unmarshal(raw, &u)
			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(u.ID), 10)},
				{Key: "Username", Value: u.Username},
				{Key: "Role", Value: u.Role},
				{Key: "Scope", Value: scopeLabel(u.ScopeGroupId)},
			}, raw)
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Username (required, unique)")
	cmd.Flags().StringVar(&password, "password", "", "Password (required)")
	cmd.Flags().StringVar(&role, "role", "", "Role: admin, editor, user, or guest (required)")
	cmd.Flags().StringVar(&displayName, "display-name", "", "Optional display name")
	cmd.Flags().UintVar(&scopeGroup, "scope-group", 0, "Scope group id (required for guest, optional for user)")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "Create the account disabled")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("password")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

func newUserUpdateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var username, password, role, displayName string
	var scopeGroup uint
	var disabled, enable bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a user account",
		Long:  "Update an existing user account. Only the flags you pass are changed; the rest are preserved by reading the current account first. Use --disabled to lock an account (revoking its sessions and tokens) and --enable to unlock it.",
		Example: strings.Join([]string{
			"  # Promote user 4 to editor",
			"  mr user update 4 --role editor",
			"",
			"  # Disable an account and reset its password",
			"  mr user update 4 --disabled --password newpass",
		}, "\n"),
		Annotations: userExitCodes,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			if disabled && enable {
				return fmt.Errorf("--disabled and --enable are mutually exclusive")
			}
			// The update API is a full replace, so read the current account and
			// override only the explicitly-set flags (client-side partial update).
			var cur userView
			if err := c.Get("/v1/user", url.Values{"id": {strconv.FormatUint(id, 10)}}, &cur); err != nil {
				return err
			}
			body := map[string]any{
				"id":           id,
				"username":     cur.Username,
				"displayName":  cur.DisplayName,
				"role":         cur.Role,
				"scopeGroupId": cur.ScopeGroupId,
				"disabled":     cur.Disabled,
			}
			if cmd.Flags().Changed("username") {
				body["username"] = username
			}
			if cmd.Flags().Changed("display-name") {
				body["displayName"] = displayName
			}
			if cmd.Flags().Changed("role") {
				body["role"] = role
			}
			if cmd.Flags().Changed("scope-group") {
				body["scopeGroupId"] = scopeGroup
			}
			if cmd.Flags().Changed("password") {
				body["password"] = password
			}
			if disabled {
				body["disabled"] = true
			}
			if enable {
				body["disabled"] = false
			}
			var raw json.RawMessage
			if err := c.Post("/v1/user", nil, body, &raw); err != nil {
				return err
			}
			var u userView
			_ = json.Unmarshal(raw, &u)
			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(u.ID), 10)},
				{Key: "Username", Value: u.Username},
				{Key: "Role", Value: u.Role},
				{Key: "Scope", Value: scopeLabel(u.ScopeGroupId)},
				{Key: "Disabled", Value: strconv.FormatBool(u.Disabled)},
			}, raw)
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "New username")
	cmd.Flags().StringVar(&password, "password", "", "New password (omit to keep the current one)")
	cmd.Flags().StringVar(&role, "role", "", "New role: admin, editor, user, or guest")
	cmd.Flags().StringVar(&displayName, "display-name", "", "New display name")
	cmd.Flags().UintVar(&scopeGroup, "scope-group", 0, "New scope group id")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "Disable the account (revokes its sessions and tokens)")
	cmd.Flags().BoolVar(&enable, "enable", false, "Re-enable a disabled account")
	return cmd
}

func newUserDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a user account",
		Long:  "Permanently delete a user account by its numeric id, removing its sessions and API tokens. This cannot be undone.",
		Example: strings.Join([]string{
			"  # Delete user 4",
			"  mr user delete 4",
			"",
			"  # Delete after listing",
			"  mr user delete 9",
		}, "\n"),
		Annotations: userExitCodes,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			if err := c.Post("/v1/user/delete", url.Values{"id": {strconv.FormatUint(id, 10)}}, nil, nil); err != nil {
				return err
			}
			output.PrintMessage("User deleted.")
			return nil
		},
	}
}

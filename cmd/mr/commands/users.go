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
		Long:  "Show all user accounts with their id, username, role, scope group, and disabled state. Password hashes are never returned. Pagination is --offset and --limit; the global --page flag does not apply to this command.",
		Example: strings.Join([]string{
			"  # List all users",
			"  mr user list",
			"",
			"  # As raw JSON",
			"  mr user list --json",
			"",
			// The delete is cleanup, and it runs from a trap because the block runs
			// under `bash -eo pipefail`: as a trailing line it was skipped by any
			// earlier failure, and a durable account survived the run. The trap is
			// armed *before* the create and resolves the account by its unique
			// username at exit time, never from an id captured afterwards -- armed
			// after `ID=$(create | jq ...)` it would still miss the interleaving that
			// matters, where the account has committed but the pipeline reading its id
			// fails and `bash -e` exits with no trap installed. The username is chosen
			// before the create, so the trap needs nothing the create returns, and
			// this block no longer captures an id at all.
			//
			// Every statement inside the trap is guarded and the body ends in
			// `return 0`. `set -e` is still in force inside an EXIT trap, so an
			// unguarded failure there truncates the rest of the cleanup and also
			// overwrites the block's own exit status, which would report a passing
			// block as a failure. The other four blocks here follow the same shape.
			"  # mr-doctest: the list carries the account just created - found by name rather than by position",
			"  U=\"doctest-list-$$-$RANDOM\"",
			"  cleanup() {",
			"    LEFTOVER=$(mr user list --json | jq -r --arg u \"$U\" 'map(select(.username == $u)) | .[0].ID // empty') || LEFTOVER=\"\"",
			"    [ -n \"$LEFTOVER\" ] && mr user delete \"$LEFTOVER\" > /dev/null 2>&1 || true",
			"    return 0",
			"  }",
			"  trap cleanup EXIT",
			"  mr user create --username \"$U\" --password doctestpw1 --role editor --json > /dev/null",
			"  mr user list --json | jq -e --arg u \"$U\" 'map(select(.username == $u)) | length == 1' > /dev/null",
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
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum users to return (0 = no limit)")
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
			"",
			// The block still needs the id, but the trap does not: it is armed before
			// the create and finds the account by username, so a failure in the
			// capture pipeline cannot leak the account. See `user list` above.
			"  # mr-doctest: get returns the account the id names",
			"  U=\"doctest-get-$$-$RANDOM\"",
			"  cleanup() {",
			"    LEFTOVER=$(mr user list --json | jq -r --arg u \"$U\" 'map(select(.username == $u)) | .[0].ID // empty') || LEFTOVER=\"\"",
			"    [ -n \"$LEFTOVER\" ] && mr user delete \"$LEFTOVER\" > /dev/null 2>&1 || true",
			"    return 0",
			"  }",
			"  trap cleanup EXIT",
			"  ID=$(mr user create --username \"$U\" --password doctestpw1 --role editor --json | jq -r '.ID')",
			"  mr user get $ID --json | jq -e --arg u \"$U\" '.username == $u and .role == \"editor\"' > /dev/null",
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
		Long:  "Create a new user account with a username, password, and role (admin, editor, user, or guest). Guests require a scope group; users may optionally have one; a scope group passed for an admin or editor is dropped rather than refused. Passwords must be at least 8 characters and at most 72 bytes.",
		Example: strings.Join([]string{
			"  # Create an editor",
			"  mr user create --username alice --password password1 --role editor",
			"",
			"  # Create a guest confined to group 7",
			"  mr user create --username bob --password password1 --role guest --scope-group 7",
			"",
			// The username moves into $U so the trap can name it. It was inline before,
			// which meant the only handle on the created account was the id the very
			// next pipeline had to parse -- and a failure there leaked the account.
			// See `user list` above.
			"  # mr-doctest: create an editor and assert the returned id is real",
			"  U=\"doctest-create-$$-$RANDOM\"",
			"  cleanup() {",
			"    LEFTOVER=$(mr user list --json | jq -r --arg u \"$U\" 'map(select(.username == $u)) | .[0].ID // empty') || LEFTOVER=\"\"",
			"    [ -n \"$LEFTOVER\" ] && mr user delete \"$LEFTOVER\" > /dev/null 2>&1 || true",
			"    return 0",
			"  }",
			"  trap cleanup EXIT",
			"  ID=$(mr user create --username \"$U\" --password doctestpw1 --role editor --json | jq -r '.ID')",
			"  test \"$ID\" -gt 0",
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
		Long:  "Update an existing user account. Only the flags you pass are sent and changed; omitted fields are preserved by the server. Use --disabled to lock an account and --enable to unlock it; disabling an account or setting a new password revokes its sessions and API tokens. --disabled and --enable cannot be combined. Passwords must be at least 8 characters and at most 72 bytes. Demoting or disabling the last enabled administrator is refused with HTTP 409 Conflict, so an instance can never be left without an admin.",
		Example: strings.Join([]string{
			"  # Promote user 4 to editor",
			"  mr user update 4 --role editor",
			"",
			"  # Disable an account and reset its password",
			"  mr user update 4 --disabled --password password2",
			"",
			// The update leaves the username alone, so the trap's by-name lookup still
			// finds the account after it. See `user list` above.
			"  # mr-doctest: update changes only the field it names",
			"  U=\"doctest-update-$$-$RANDOM\"",
			"  cleanup() {",
			"    LEFTOVER=$(mr user list --json | jq -r --arg u \"$U\" 'map(select(.username == $u)) | .[0].ID // empty') || LEFTOVER=\"\"",
			"    [ -n \"$LEFTOVER\" ] && mr user delete \"$LEFTOVER\" > /dev/null 2>&1 || true",
			"    return 0",
			"  }",
			"  trap cleanup EXIT",
			"  ID=$(mr user create --username \"$U\" --password doctestpw1 --role editor --json | jq -r '.ID')",
			"  mr user update $ID --role user",
			"  mr user get $ID --json | jq -e '.role == \"user\"' > /dev/null",
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
			// The server update contract is presence-aware. Sending only explicit
			// flags avoids restoring unrelated state from a stale GET snapshot.
			body := map[string]any{"id": id}
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
	cmd.Flags().StringVar(&password, "password", "", "New password (omit to keep the current one; setting one revokes the account's sessions and tokens)")
	cmd.Flags().StringVar(&role, "role", "", "New role: admin, editor, user, or guest")
	cmd.Flags().StringVar(&displayName, "display-name", "", "New display name")
	cmd.Flags().UintVar(&scopeGroup, "scope-group", 0, "New scope group id (use 0 to clear)")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "Disable the account (revokes its sessions and tokens)")
	cmd.Flags().BoolVar(&enable, "enable", false, "Re-enable a disabled account")
	return cmd
}

func newUserDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a user account",
		Long:  "Permanently delete a user account by its numeric id, removing its sessions and API tokens, and nulling the creator on any content they stamped (the content is kept). This cannot be undone. Deleting the last enabled administrator is refused with HTTP 409 Conflict.",
		Example: strings.Join([]string{
			"  # Delete user 4",
			"  mr user delete 4",
			"",
			"  # Delete after listing",
			"  mr user delete 9",
			"",
			// Here the delete IS the assertion, so it stays inline and the trap is only
			// the backstop, covering the create and the id capture that follows it.
			// After the inline delete succeeds the trap's lookup finds nothing and it
			// does nothing, so the two never collide. See `user list` above.
			"  # mr-doctest: delete removes the account so a follow-up get fails",
			"  U=\"doctest-delete-$$-$RANDOM\"",
			"  cleanup() {",
			"    LEFTOVER=$(mr user list --json | jq -r --arg u \"$U\" 'map(select(.username == $u)) | .[0].ID // empty') || LEFTOVER=\"\"",
			"    [ -n \"$LEFTOVER\" ] && mr user delete \"$LEFTOVER\" > /dev/null 2>&1 || true",
			"    return 0",
			"  }",
			"  trap cleanup EXIT",
			"  ID=$(mr user create --username \"$U\" --password doctestpw1 --role editor --json | jq -r '.ID')",
			"  mr user delete $ID",
			"  ! mr user get $ID 2>/dev/null",
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

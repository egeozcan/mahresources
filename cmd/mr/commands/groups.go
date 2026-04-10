package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// groupResponse is a lightweight struct matching the API's Group JSON shape.
type groupResponse struct {
	ID          uint      `json:"ID"`
	Name        string    `json:"Name"`
	Description string    `json:"Description"`
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
	OwnerId     *uint     `json:"OwnerId"`
	CategoryId  *uint     `json:"CategoryId"`
}

// treeNodeResponse matches the GroupTreeNode JSON shape.
type treeNodeResponse struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	CategoryName string `json:"categoryName"`
	ChildCount   int    `json:"childCount"`
	OwnerID      *uint  `json:"ownerId"`
}

// NewGroupCmd returns the singular "group" command with subcommands.
func NewGroupCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Get, create, edit, delete, or clone a group",
	}

	cmd.AddCommand(newGroupGetCmd(c, opts))
	cmd.AddCommand(newGroupCreateCmd(c, opts))
	cmd.AddCommand(newGroupDeleteCmd(c, opts))
	cmd.AddCommand(newGroupEditNameCmd(c, opts))
	cmd.AddCommand(newGroupEditDescriptionCmd(c, opts))
	cmd.AddCommand(newGroupEditMetaCmd(c, opts))
	cmd.AddCommand(newGroupParentsCmd(c, opts))
	cmd.AddCommand(newGroupChildrenCmd(c, opts))
	cmd.AddCommand(newGroupCloneCmd(c, opts))

	return cmd
}

func newGroupGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a group by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/group", q, &raw); err != nil {
				return err
			}

			var group groupResponse
			if err := json.Unmarshal(raw, &group); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fields := []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(group.ID), 10)},
				{Key: "Name", Value: group.Name},
				{Key: "Description", Value: group.Description},
				{Key: "Created", Value: group.CreatedAt.Format(time.RFC3339)},
				{Key: "Updated", Value: group.UpdatedAt.Format(time.RFC3339)},
			}
			if group.OwnerId != nil {
				fields = append(fields, output.KeyValue{Key: "OwnerId", Value: strconv.FormatUint(uint64(*group.OwnerId), 10)})
			}
			if group.CategoryId != nil {
				fields = append(fields, output.KeyValue{Key: "CategoryId", Value: strconv.FormatUint(uint64(*group.CategoryId), 10)})
			}

			output.PrintSingle(*opts, fields, raw)
			return nil
		},
	}
}

func newGroupCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, tagsStr, groupsStr, meta, urlStr string
	var ownerID, categoryID uint

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new group",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"Name": name}
			if description != "" {
				body["Description"] = description
			}
			if tagsStr != "" {
				tags, err := parseUintList(tagsStr)
				if err != nil {
					return fmt.Errorf("parsing --tags: %w", err)
				}
				body["Tags"] = tags
			}
			if groupsStr != "" {
				groups, err := parseUintList(groupsStr)
				if err != nil {
					return fmt.Errorf("parsing --groups: %w", err)
				}
				body["Groups"] = groups
			}
			if meta != "" {
				body["Meta"] = meta
			}
			if urlStr != "" {
				body["Url"] = urlStr
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerId"] = ownerID
			}
			if cmd.Flags().Changed("category-id") {
				body["CategoryId"] = categoryID
			}

			var raw json.RawMessage
			if err := c.Post("/v1/group", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var group groupResponse
				if err := json.Unmarshal(raw, &group); err == nil {
					output.PrintMessage(fmt.Sprintf("Created group %d: %s", group.ID, group.Name))
				} else {
					output.PrintMessage("Group created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Group name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Group description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string")
	cmd.Flags().StringVar(&urlStr, "url", "", "URL")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")
	cmd.Flags().UintVar(&categoryID, "category-id", 0, "Category ID")

	return cmd
}

func newGroupDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a group by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/group/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Group deleted successfully.")
			}
			return nil
		},
	}
}

func newGroupEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a group's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/group/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Group name updated successfully.")
			}
			return nil
		},
	}
}

func newGroupEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a group's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/group/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Group description updated successfully.")
			}
			return nil
		},
	}
}

func newGroupEditMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-meta <id> <path> <value>",
		Short: "Edit a single metadata field by JSON path",
		Long: `Edit a single metadata field using deep-merge-by-path.

The path is a dot-separated JSON path (e.g., "address.city") and the value
is a JSON literal (e.g., '"Berlin"', '42', '{"nested":"obj"}').

Examples:
  mr group edit-meta 5 status '"active"'
  mr group edit-meta 5 address.city '"Berlin"'
  mr group edit-meta 5 scores '[1,2,3]'`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("path", args[1])
			form.Set("value", args[2])

			var raw json.RawMessage
			if err := c.PostForm("/v1/group/editMeta", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Group metadata updated successfully.")
			}
			return nil
		},
	}
}

func newGroupParentsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "parents <id>",
		Short: "List parent groups of a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/group/parents", q, &raw); err != nil {
				return err
			}

			var groups []groupResponse
			if err := json.Unmarshal(raw, &groups); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "CATEGORY_ID", "CREATED"}
			var rows [][]string
			for _, g := range groups {
				catID := ""
				if g.CategoryId != nil {
					catID = strconv.FormatUint(uint64(*g.CategoryId), 10)
				}
				rows = append(rows, []string{
					strconv.FormatUint(uint64(g.ID), 10),
					output.Truncate(g.Name, 40),
					catID,
					g.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}
}

func newGroupChildrenCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "children <id>",
		Short: "List child groups (tree children) of a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/group/tree/children", q, &raw); err != nil {
				return err
			}

			var nodes []treeNodeResponse
			if err := json.Unmarshal(raw, &nodes); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "CHILD_COUNT"}
			var rows [][]string
			for _, n := range nodes {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(n.ID), 10),
					output.Truncate(n.Name, 40),
					strconv.Itoa(n.ChildCount),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}
}

func newGroupCloneCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "clone <id>",
		Short: "Clone a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			body := map[string]any{
				"ID": uint(id),
			}

			var raw json.RawMessage
			if err := c.Post("/v1/group/clone", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var group groupResponse
				if err := json.Unmarshal(raw, &group); err == nil {
					output.PrintMessage(fmt.Sprintf("Cloned group as %d: %s", group.ID, group.Name))
				} else {
					output.PrintMessage("Group cloned successfully.")
				}
			}
			return nil
		},
	}
}

// NewGroupsCmd returns the plural "groups" command with list/bulk subcommands.
func NewGroupsCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "List, merge, or bulk-edit groups",
	}

	cmd.AddCommand(newGroupsListCmd(c, opts, page))
	cmd.AddCommand(newGroupsAddTagsCmd(c, opts))
	cmd.AddCommand(newGroupsRemoveTagsCmd(c, opts))
	cmd.AddCommand(newGroupsAddMetaCmd(c, opts))
	cmd.AddCommand(newGroupsDeleteCmd(c, opts))
	cmd.AddCommand(newGroupsMergeCmd(c, opts))
	cmd.AddCommand(newGroupsMetaKeysCmd(c, opts))
	cmd.AddCommand(newGroupsTimelineCmd(c, opts))

	return cmd
}

func newGroupsListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description, tagsStr, groupsStr, urlStr, createdBefore, createdAfter string
	var ownerID, categoryID uint

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if name != "" {
				q.Set("name", name)
			}
			if description != "" {
				q.Set("description", description)
			}
			if tagsStr != "" {
				tags, err := parseUintList(tagsStr)
				if err != nil {
					return fmt.Errorf("parsing --tags: %w", err)
				}
				for _, t := range tags {
					q.Add("tags", strconv.FormatUint(uint64(t), 10))
				}
			}
			if groupsStr != "" {
				groups, err := parseUintList(groupsStr)
				if err != nil {
					return fmt.Errorf("parsing --groups: %w", err)
				}
				for _, g := range groups {
					q.Add("groups", strconv.FormatUint(uint64(g), 10))
				}
			}
			if cmd.Flags().Changed("owner-id") {
				q.Set("ownerId", strconv.FormatUint(uint64(ownerID), 10))
			}
			if cmd.Flags().Changed("category-id") {
				q.Set("categoryId", strconv.FormatUint(uint64(categoryID), 10))
			}
			if urlStr != "" {
				q.Set("url", urlStr)
			}
			if createdBefore != "" {
				q.Set("createdBefore", createdBefore)
			}
			if createdAfter != "" {
				q.Set("createdAfter", createdAfter)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/groups", q, &raw); err != nil {
				return err
			}

			var groups []groupResponse
			if err := json.Unmarshal(raw, &groups); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "CATEGORY_ID", "OWNER_ID", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, g := range groups {
				catID := ""
				if g.CategoryId != nil {
					catID = strconv.FormatUint(uint64(*g.CategoryId), 10)
				}
				ownerStr := ""
				if g.OwnerId != nil {
					ownerStr = strconv.FormatUint(uint64(*g.OwnerId), 10)
				}
				rows = append(rows, []string{
					strconv.FormatUint(uint64(g.ID), 10),
					output.Truncate(g.Name, 40),
					catID,
					ownerStr,
					output.Truncate(g.Description, 50),
					g.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs to filter by")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs to filter by")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Filter by owner group ID")
	cmd.Flags().UintVar(&categoryID, "category-id", 0, "Filter by category ID")
	cmd.Flags().StringVar(&urlStr, "url", "", "Filter by URL")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "Filter by creation date (before)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Filter by creation date (after)")

	return cmd
}

func newGroupsAddTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "add-tags",
		Short: "Add tags to multiple groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}
			tags, err := parseUintList(tagsStr)
			if err != nil {
				return fmt.Errorf("parsing --tags: %w", err)
			}

			body := map[string]any{
				"ID":       ids,
				"EditedId": tags,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/groups/addTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags added to groups successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated group IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newGroupsRemoveTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "remove-tags",
		Short: "Remove tags from multiple groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}
			tags, err := parseUintList(tagsStr)
			if err != nil {
				return fmt.Errorf("parsing --tags: %w", err)
			}

			body := map[string]any{
				"ID":       ids,
				"EditedId": tags,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/groups/removeTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags removed from groups successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated group IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newGroupsAddMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, meta string

	cmd := &cobra.Command{
		Use:   "add-meta",
		Short: "Add metadata to multiple groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}

			body := map[string]any{
				"ID":   ids,
				"Meta": meta,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/groups/addMeta", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Metadata added to groups successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated group IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string (required)")
	cmd.MarkFlagRequired("meta")

	return cmd
}

func newGroupsDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete multiple groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}

			body := map[string]any{
				"ID": ids,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/groups/delete", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Groups deleted successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated group IDs to delete (required)")
	cmd.MarkFlagRequired("ids")

	return cmd
}

func newGroupsMergeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var winner uint
	var losersStr string

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge groups into a winner",
		RunE: func(cmd *cobra.Command, args []string) error {
			losers, err := parseUintList(losersStr)
			if err != nil {
				return fmt.Errorf("parsing --losers: %w", err)
			}

			body := map[string]any{
				"Winner": winner,
				"Losers": losers,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/groups/merge", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Groups merged successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&winner, "winner", 0, "Winning group ID (required)")
	cmd.MarkFlagRequired("winner")
	cmd.Flags().StringVar(&losersStr, "losers", "", "Comma-separated loser group IDs (required)")
	cmd.MarkFlagRequired("losers")

	return cmd
}

func newGroupsMetaKeysCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "meta-keys",
		Short: "List all unique metadata keys used across groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/groups/meta/keys", nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var keys []string
				if err := json.Unmarshal(raw, &keys); err != nil {
					// Fallback: print raw
					output.PrintSingle(*opts, nil, raw)
					return nil
				}
				for _, k := range keys {
					output.PrintMessage(k)
				}
			}
			return nil
		},
	}
}

func newGroupsTimelineCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		tFlags                                                                          timelineFlags
		name, description, tagsStr, groupsStr, urlStr, createdBefore, createdAfter string
		ownerID, categoryID                                                             uint
	)

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Display a timeline of group activity",
		Long: `Display a timeline of group creation and update activity as an ASCII bar chart.

Examples:
  mr groups timeline
  mr groups timeline --granularity=weekly --columns=20
  mr groups timeline --granularity=yearly --anchor=2020-01-01
  mr groups timeline --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if name != "" {
				q.Set("name", name)
			}
			if description != "" {
				q.Set("description", description)
			}
			if tagsStr != "" {
				tags, err := parseUintList(tagsStr)
				if err != nil {
					return fmt.Errorf("parsing --tags: %w", err)
				}
				for _, t := range tags {
					q.Add("tags", strconv.FormatUint(uint64(t), 10))
				}
			}
			if groupsStr != "" {
				groups, err := parseUintList(groupsStr)
				if err != nil {
					return fmt.Errorf("parsing --groups: %w", err)
				}
				for _, g := range groups {
					q.Add("groups", strconv.FormatUint(uint64(g), 10))
				}
			}
			if cmd.Flags().Changed("owner-id") {
				q.Set("ownerId", strconv.FormatUint(uint64(ownerID), 10))
			}
			if cmd.Flags().Changed("category-id") {
				q.Set("categoryId", strconv.FormatUint(uint64(categoryID), 10))
			}
			if urlStr != "" {
				q.Set("url", urlStr)
			}
			if createdBefore != "" {
				q.Set("createdBefore", createdBefore)
			}
			if createdAfter != "" {
				q.Set("createdAfter", createdAfter)
			}

			return fetchAndPrintTimeline(c, *opts, "/v1/groups/timeline", buildTimelineQuery(&tFlags, q))
		},
	}

	addTimelineFlags(cmd, &tFlags)
	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs to filter by")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs to filter by")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Filter by owner group ID")
	cmd.Flags().UintVar(&categoryID, "category-id", 0, "Filter by category ID")
	cmd.Flags().StringVar(&urlStr, "url", "", "Filter by URL")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "Filter by creation date (before)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Filter by creation date (after)")

	return cmd
}

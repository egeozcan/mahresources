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

// noteResponse is a lightweight struct matching the API's Note JSON shape.
type noteResponse struct {
	ID         uint      `json:"ID"`
	Name       string    `json:"Name"`
	Description string   `json:"Description"`
	CreatedAt  time.Time `json:"CreatedAt"`
	UpdatedAt  time.Time `json:"UpdatedAt"`
	OwnerId    *uint     `json:"OwnerId"`
	NoteTypeId *uint     `json:"NoteTypeId"`
	ShareToken *string   `json:"ShareToken"`
}

// NewNoteCmd returns the singular "note" command with get/create/delete/edit/share subcommands.
func NewNoteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note",
		Short: "Get, create, edit, delete, or share a note",
	}

	cmd.AddCommand(newNoteGetCmd(c, opts))
	cmd.AddCommand(newNoteCreateCmd(c, opts))
	cmd.AddCommand(newNoteDeleteCmd(c, opts))
	cmd.AddCommand(newNoteEditNameCmd(c, opts))
	cmd.AddCommand(newNoteEditDescriptionCmd(c, opts))
	cmd.AddCommand(newNoteShareCmd(c, opts))
	cmd.AddCommand(newNoteUnshareCmd(c, opts))

	return cmd
}

func newNoteGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a note by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/note", q, &raw); err != nil {
				return err
			}

			var note noteResponse
			if err := json.Unmarshal(raw, &note); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fields := []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(note.ID), 10)},
				{Key: "Name", Value: note.Name},
				{Key: "Description", Value: note.Description},
				{Key: "Created", Value: note.CreatedAt.Format(time.RFC3339)},
				{Key: "Updated", Value: note.UpdatedAt.Format(time.RFC3339)},
			}
			if note.OwnerId != nil {
				fields = append(fields, output.KeyValue{Key: "OwnerId", Value: strconv.FormatUint(uint64(*note.OwnerId), 10)})
			}
			if note.NoteTypeId != nil {
				fields = append(fields, output.KeyValue{Key: "NoteTypeId", Value: strconv.FormatUint(uint64(*note.NoteTypeId), 10)})
			}
			if note.ShareToken != nil {
				fields = append(fields, output.KeyValue{Key: "ShareToken", Value: *note.ShareToken})
			}

			output.PrintSingle(*opts, fields, raw)
			return nil
		},
	}
}

func newNoteCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, tagsStr, groupsStr, resourcesStr, meta string
	var ownerID, noteTypeID uint

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new note",
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
			if resourcesStr != "" {
				resources, err := parseUintList(resourcesStr)
				if err != nil {
					return fmt.Errorf("parsing --resources: %w", err)
				}
				body["Resources"] = resources
			}
			if meta != "" {
				body["Meta"] = meta
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerId"] = ownerID
			}
			if cmd.Flags().Changed("note-type-id") {
				body["NoteTypeId"] = noteTypeID
			}

			var raw json.RawMessage
			if err := c.Post("/v1/note", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var note noteResponse
				if err := json.Unmarshal(raw, &note); err == nil {
					output.PrintMessage(fmt.Sprintf("Created note %d: %s", note.ID, note.Name))
				} else {
					output.PrintMessage("Note created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Note name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Note description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().StringVar(&resourcesStr, "resources", "", "Comma-separated resource IDs")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")
	cmd.Flags().UintVar(&noteTypeID, "note-type-id", 0, "Note type ID")

	return cmd
}

func newNoteDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a note by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/note/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note deleted successfully.")
			}
			return nil
		},
	}
}

func newNoteEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a note's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/note/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note name updated successfully.")
			}
			return nil
		},
	}
}

func newNoteEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a note's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/note/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note description updated successfully.")
			}
			return nil
		},
	}
}

func newNoteShareCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "share <id>",
		Short: "Generate a share token for a note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("noteId", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/note/share", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note shared successfully.")
			}
			return nil
		},
	}
}

func newNoteUnshareCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "unshare <id>",
		Short: "Remove the share token from a note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("noteId", args[0])

			var raw json.RawMessage
			if err := c.Delete("/v1/note/share", q, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note unshared successfully.")
			}
			return nil
		},
	}
}

// NewNotesCmd returns the plural "notes" command with list/bulk subcommands.
func NewNotesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "List notes and bulk tag/group/meta operations",
	}

	cmd.AddCommand(newNotesListCmd(c, opts, page))
	cmd.AddCommand(newNotesAddTagsCmd(c, opts))
	cmd.AddCommand(newNotesRemoveTagsCmd(c, opts))
	cmd.AddCommand(newNotesAddGroupsCmd(c, opts))
	cmd.AddCommand(newNotesAddMetaCmd(c, opts))
	cmd.AddCommand(newNotesDeleteCmd(c, opts))
	cmd.AddCommand(newNotesMetaKeysCmd(c, opts))
	cmd.AddCommand(newNotesTimelineCmd(c, opts))

	return cmd
}

func newNotesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description, tagsStr, groupsStr, createdBefore, createdAfter string
	var ownerID, noteTypeID uint

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List notes",
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
			if cmd.Flags().Changed("note-type-id") {
				q.Set("noteTypeId", strconv.FormatUint(uint64(noteTypeID), 10))
			}
			if createdBefore != "" {
				q.Set("createdBefore", createdBefore)
			}
			if createdAfter != "" {
				q.Set("createdAfter", createdAfter)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/notes", q, &raw); err != nil {
				return err
			}

			var notes []noteResponse
			if err := json.Unmarshal(raw, &notes); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "TYPE_ID", "OWNER_ID", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, n := range notes {
				typeID := ""
				if n.NoteTypeId != nil {
					typeID = strconv.FormatUint(uint64(*n.NoteTypeId), 10)
				}
				ownerStr := ""
				if n.OwnerId != nil {
					ownerStr = strconv.FormatUint(uint64(*n.OwnerId), 10)
				}
				rows = append(rows, []string{
					strconv.FormatUint(uint64(n.ID), 10),
					output.Truncate(n.Name, 40),
					typeID,
					ownerStr,
					output.Truncate(n.Description, 50),
					n.CreatedAt.Format(time.RFC3339),
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
	cmd.Flags().UintVar(&noteTypeID, "note-type-id", 0, "Filter by note type ID")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "Filter by creation date (before)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Filter by creation date (after)")

	return cmd
}

func newNotesAddTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "add-tags",
		Short: "Add tags to multiple notes",
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
			if err := c.Post("/v1/notes/addTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags added to notes successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated note IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newNotesRemoveTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "remove-tags",
		Short: "Remove tags from multiple notes",
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
			if err := c.Post("/v1/notes/removeTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags removed from notes successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated note IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newNotesAddGroupsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, groupsStr string

	cmd := &cobra.Command{
		Use:   "add-groups",
		Short: "Add groups to multiple notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}
			groups, err := parseUintList(groupsStr)
			if err != nil {
				return fmt.Errorf("parsing --groups: %w", err)
			}

			body := map[string]any{
				"ID":       ids,
				"EditedId": groups,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/notes/addGroups", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Groups added to notes successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated note IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs (required)")
	cmd.MarkFlagRequired("groups")

	return cmd
}

func newNotesAddMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, meta string

	cmd := &cobra.Command{
		Use:   "add-meta",
		Short: "Add metadata to multiple notes",
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
			if err := c.Post("/v1/notes/addMeta", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Metadata added to notes successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated note IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string (required)")
	cmd.MarkFlagRequired("meta")

	return cmd
}

func newNotesDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete multiple notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}

			body := map[string]any{
				"ID": ids,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/notes/delete", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Notes deleted successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated note IDs to delete (required)")
	cmd.MarkFlagRequired("ids")

	return cmd
}

func newNotesMetaKeysCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "meta-keys",
		Short: "List all unique metadata keys used across notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/notes/meta/keys", nil, &raw); err != nil {
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

func newNotesTimelineCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		tFlags                                                      timelineFlags
		name, description, tagsStr, groupsStr, createdBefore, createdAfter string
		ownerID, noteTypeID                                         uint
	)

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Display a timeline of note activity",
		Long: `Display a timeline of note creation and update activity as an ASCII bar chart.

Examples:
  mr notes timeline
  mr notes timeline --granularity=weekly --columns=20
  mr notes timeline --granularity=yearly --anchor=2020-01-01
  mr notes timeline --json`,
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
			if cmd.Flags().Changed("note-type-id") {
				q.Set("noteTypeId", strconv.FormatUint(uint64(noteTypeID), 10))
			}
			if createdBefore != "" {
				q.Set("createdBefore", createdBefore)
			}
			if createdAfter != "" {
				q.Set("createdAfter", createdAfter)
			}

			return fetchAndPrintTimeline(c, *opts, "/v1/notes/timeline", buildTimelineQuery(&tFlags, q))
		},
	}

	addTimelineFlags(cmd, &tFlags)
	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs to filter by")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs to filter by")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Filter by owner group ID")
	cmd.Flags().UintVar(&noteTypeID, "note-type-id", 0, "Filter by note type ID")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "Filter by creation date (before)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Filter by creation date (after)")

	return cmd
}

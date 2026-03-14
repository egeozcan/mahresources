package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// resourceResponse is a lightweight struct matching the API's Resource JSON shape.
type resourceResponse struct {
	ID                 uint      `json:"ID"`
	Name               string    `json:"Name"`
	Description        string    `json:"Description"`
	OriginalName       string    `json:"OriginalName"`
	ContentType        string    `json:"ContentType"`
	ContentCategory    string    `json:"ContentCategory"`
	FileSize           int64     `json:"FileSize"`
	Width              uint      `json:"Width"`
	Height             uint      `json:"Height"`
	Hash               string    `json:"Hash"`
	OwnerId            *uint     `json:"OwnerId"`
	ResourceCategoryId *uint     `json:"ResourceCategoryId"`
	SeriesID           *uint     `json:"SeriesID"`
	CreatedAt          time.Time `json:"CreatedAt"`
	UpdatedAt          time.Time `json:"UpdatedAt"`
}

// resourceVersionResponse matches the API's ResourceVersion JSON shape.
type resourceVersionResponse struct {
	ID            uint      `json:"ID"`
	ResourceID    uint      `json:"ResourceID"`
	VersionNumber int       `json:"VersionNumber"`
	Hash          string    `json:"Hash"`
	FileSize      int64     `json:"FileSize"`
	ContentType   string    `json:"ContentType"`
	Width         uint      `json:"Width"`
	Height        uint      `json:"Height"`
	Comment       string    `json:"Comment"`
	CreatedAt     time.Time `json:"CreatedAt"`
}

// versionComparisonResponse matches the API's version comparison JSON shape.
type versionComparisonResponse struct {
	SizeDelta      int64 `json:"SizeDelta"`
	SameHash       bool  `json:"SameHash"`
	SameType       bool  `json:"SameType"`
	DimensionsDiff bool  `json:"DimensionsDiff"`
}

func formatDimensions(w, h uint) string {
	if w == 0 && h == 0 {
		return "-"
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func ptrUintStr(p *uint) string {
	if p == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*p), 10)
}

// ---------------------------------------------------------------------------
// Singular "resource" command
// ---------------------------------------------------------------------------

// NewResourceCmd returns the singular "resource" command tree.
func NewResourceCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Upload, download, edit, or version a resource",
	}

	cmd.AddCommand(newResourceGetCmd(c, opts))
	cmd.AddCommand(newResourceEditCmd(c, opts))
	cmd.AddCommand(newResourceDeleteCmd(c, opts))
	cmd.AddCommand(newResourceEditNameCmd(c, opts))
	cmd.AddCommand(newResourceEditDescriptionCmd(c, opts))
	cmd.AddCommand(newResourceUploadCmd(c, opts))
	cmd.AddCommand(newResourceDownloadCmd(c, opts))
	cmd.AddCommand(newResourcePreviewCmd(c, opts))
	cmd.AddCommand(newResourceFromURLCmd(c, opts))
	cmd.AddCommand(newResourceFromLocalCmd(c, opts))
	cmd.AddCommand(newResourceRotateCmd(c, opts))
	cmd.AddCommand(newResourceRecalcDimsCmd(c, opts))
	cmd.AddCommand(newResourceVersionsCmd(c, opts))
	cmd.AddCommand(newResourceVersionCmd(c, opts))
	cmd.AddCommand(newResourceVersionUploadCmd(c, opts))
	cmd.AddCommand(newResourceVersionDownloadCmd(c, opts))
	cmd.AddCommand(newResourceVersionRestoreCmd(c, opts))
	cmd.AddCommand(newResourceVersionDeleteCmd(c, opts))
	cmd.AddCommand(newResourceVersionsCleanupCmd(c, opts))
	cmd.AddCommand(newResourceVersionsCompareCmd(c, opts))

	return cmd
}

func newResourceGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a resource by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/resource", q, &raw); err != nil {
				return err
			}

			var res resourceResponse
			if err := json.Unmarshal(raw, &res); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fields := []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(res.ID), 10)},
				{Key: "Name", Value: res.Name},
				{Key: "OriginalName", Value: res.OriginalName},
				{Key: "ContentType", Value: res.ContentType},
				{Key: "FileSize", Value: formatFileSize(res.FileSize)},
				{Key: "Dimensions", Value: formatDimensions(res.Width, res.Height)},
				{Key: "Hash", Value: res.Hash},
				{Key: "Owner", Value: ptrUintStr(res.OwnerId)},
				{Key: "Created", Value: res.CreatedAt.Format(time.RFC3339)},
				{Key: "Updated", Value: res.UpdatedAt.Format(time.RFC3339)},
			}

			output.PrintSingle(*opts, fields, raw)
			return nil
		},
	}
}

func newResourceEditCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		name, description, tagsStr, groupsStr, notesStr string
		meta, category, originalName, originalLocation   string
		ownerID, resourceCategoryID, seriesID            uint
		width, height                                    uint
	)

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			body := map[string]any{"ID": uint(id)}

			if cmd.Flags().Changed("name") {
				body["Name"] = name
			}
			if cmd.Flags().Changed("description") {
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
			if notesStr != "" {
				notes, err := parseUintList(notesStr)
				if err != nil {
					return fmt.Errorf("parsing --notes: %w", err)
				}
				body["Notes"] = notes
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerId"] = ownerID
			}
			if cmd.Flags().Changed("meta") {
				body["Meta"] = meta
			}
			if cmd.Flags().Changed("category") {
				body["Category"] = category
			}
			if cmd.Flags().Changed("resource-category-id") {
				body["ResourceCategoryId"] = resourceCategoryID
			}
			if cmd.Flags().Changed("original-name") {
				body["OriginalName"] = originalName
			}
			if cmd.Flags().Changed("original-location") {
				body["OriginalLocation"] = originalLocation
			}
			if cmd.Flags().Changed("width") {
				body["Width"] = width
			}
			if cmd.Flags().Changed("height") {
				body["Height"] = height
			}
			if cmd.Flags().Changed("series-id") {
				body["SeriesID"] = seriesID
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resource/edit", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Resource name")
	cmd.Flags().StringVar(&description, "description", "", "Resource description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().StringVar(&notesStr, "notes", "", "Comma-separated note IDs")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string")
	cmd.Flags().StringVar(&category, "category", "", "Category")
	cmd.Flags().UintVar(&resourceCategoryID, "resource-category-id", 0, "Resource category ID")
	cmd.Flags().StringVar(&originalName, "original-name", "", "Original file name")
	cmd.Flags().StringVar(&originalLocation, "original-location", "", "Original file location")
	cmd.Flags().UintVar(&width, "width", 0, "Width in pixels")
	cmd.Flags().UintVar(&height, "height", 0, "Height in pixels")
	cmd.Flags().UintVar(&seriesID, "series-id", 0, "Series ID")

	return cmd
}

func newResourceDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a resource by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/resource/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource deleted successfully.")
			}
			return nil
		},
	}
}

func newResourceEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a resource's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("value", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/resource/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource name updated successfully.")
			}
			return nil
		},
	}
}

func newResourceEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a resource's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("value", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/resource/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource description updated successfully.")
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// File operations
// ---------------------------------------------------------------------------

func newResourceUploadCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		name, description, meta, category       string
		contentCategory, originalName            string
		ownerID, resourceCategoryID              uint
	)

	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload a file as a new resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			extra := map[string]string{}

			if cmd.Flags().Changed("name") {
				extra["Name"] = name
			}
			if cmd.Flags().Changed("description") {
				extra["Description"] = description
			}
			if cmd.Flags().Changed("owner-id") {
				extra["OwnerId"] = strconv.FormatUint(uint64(ownerID), 10)
			}
			if cmd.Flags().Changed("meta") {
				extra["Meta"] = meta
			}
			if cmd.Flags().Changed("category") {
				extra["Category"] = category
			}
			if cmd.Flags().Changed("content-category") {
				extra["ContentCategory"] = contentCategory
			}
			if cmd.Flags().Changed("resource-category-id") {
				extra["ResourceCategoryId"] = strconv.FormatUint(uint64(resourceCategoryID), 10)
			}
			if cmd.Flags().Changed("original-name") {
				extra["OriginalName"] = originalName
			}

			var raw json.RawMessage
			if err := c.UploadFile("/v1/resource", nil, "resource", filePath, extra, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var res resourceResponse
				if err := json.Unmarshal(raw, &res); err == nil {
					output.PrintMessage(fmt.Sprintf("Created resource %d: %s", res.ID, res.Name))
				} else {
					output.PrintMessage("Resource uploaded successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Resource name")
	cmd.Flags().StringVar(&description, "description", "", "Resource description")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string")
	cmd.Flags().StringVar(&category, "category", "", "Category")
	cmd.Flags().StringVar(&contentCategory, "content-category", "", "Content category")
	cmd.Flags().UintVar(&resourceCategoryID, "resource-category-id", 0, "Resource category ID")
	cmd.Flags().StringVar(&originalName, "original-name", "", "Original file name")

	return cmd
}

func newResourceDownloadCmd(c *client.Client, _ *output.Options) *cobra.Command {
	var outFile string

	cmd := &cobra.Command{
		Use:   "download <id>",
		Short: "Download a resource file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			dest := outFile
			if dest == "" {
				dest = fmt.Sprintf("resource_%s", args[0])
			}

			n, err := c.DownloadFile("/v1/resource/view", q, dest)
			if err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Downloaded %s (%s)", dest, formatFileSize(n)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outFile, "output", "o", "", "Output file path (default: resource_<id>)")

	return cmd
}

func newResourcePreviewCmd(c *client.Client, _ *output.Options) *cobra.Command {
	var (
		outFile        string
		width, height  uint
	)

	cmd := &cobra.Command{
		Use:   "preview <id>",
		Short: "Download a scaled thumbnail of a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("ID", args[0])
			if cmd.Flags().Changed("width") {
				q.Set("Width", strconv.FormatUint(uint64(width), 10))
			}
			if cmd.Flags().Changed("height") {
				q.Set("Height", strconv.FormatUint(uint64(height), 10))
			}

			dest := outFile
			if dest == "" {
				dest = fmt.Sprintf("preview_%s", args[0])
			}

			n, err := c.DownloadFile("/v1/resource/preview", q, dest)
			if err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Downloaded %s (%s)", dest, formatFileSize(n)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outFile, "output", "o", "", "Output file path (default: preview_<id>)")
	cmd.Flags().UintVarP(&width, "width", "w", 0, "Preview width")
	cmd.Flags().UintVar(&height, "height", 0, "Preview height")

	return cmd
}

func newResourceFromURLCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		remoteURL, name, description string
		tagsStr, groupsStr           string
		ownerID                      uint
		meta, fileName               string
	)

	cmd := &cobra.Command{
		Use:   "from-url",
		Short: "Create a resource from a remote URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"URL": remoteURL}

			if cmd.Flags().Changed("name") {
				body["Name"] = name
			}
			if cmd.Flags().Changed("description") {
				body["Description"] = description
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerId"] = ownerID
			}
			if cmd.Flags().Changed("meta") {
				body["Meta"] = meta
			}
			if cmd.Flags().Changed("file-name") {
				body["FileName"] = fileName
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

			var raw json.RawMessage
			if err := c.Post("/v1/resource/remote", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var res resourceResponse
				if err := json.Unmarshal(raw, &res); err == nil {
					output.PrintMessage(fmt.Sprintf("Created resource %d from URL", res.ID))
				} else {
					output.PrintMessage("Resource created from URL successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&remoteURL, "url", "", "Remote URL (required)")
	cmd.MarkFlagRequired("url")
	cmd.Flags().StringVar(&name, "name", "", "Resource name")
	cmd.Flags().StringVar(&description, "description", "", "Resource description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string")
	cmd.Flags().StringVar(&fileName, "file-name", "", "Override file name")

	return cmd
}

func newResourceFromLocalCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		localPath, name, description string
		tagsStr, groupsStr           string
		ownerID                      uint
		meta                         string
	)

	cmd := &cobra.Command{
		Use:   "from-local",
		Short: "Create a resource from a local server path",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"LocalPath": localPath}

			if cmd.Flags().Changed("name") {
				body["Name"] = name
			}
			if cmd.Flags().Changed("description") {
				body["Description"] = description
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerId"] = ownerID
			}
			if cmd.Flags().Changed("meta") {
				body["Meta"] = meta
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

			var raw json.RawMessage
			if err := c.Post("/v1/resource/local", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var res resourceResponse
				if err := json.Unmarshal(raw, &res); err == nil {
					output.PrintMessage(fmt.Sprintf("Created resource %d from local path", res.ID))
				} else {
					output.PrintMessage("Resource created from local path successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&localPath, "path", "", "Local server path (required)")
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVar(&name, "name", "", "Resource name")
	cmd.Flags().StringVar(&description, "description", "", "Resource description")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string")

	return cmd
}

func newResourceRotateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var degrees int

	cmd := &cobra.Command{
		Use:   "rotate <id>",
		Short: "Rotate a resource image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			body := map[string]any{
				"ID":      uint(id),
				"Degrees": degrees,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resources/rotate", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource rotated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&degrees, "degrees", 0, "Rotation degrees (required)")
	cmd.MarkFlagRequired("degrees")

	return cmd
}

func newResourceRecalcDimsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "recalculate-dimensions <id>",
		Short: "Recalculate resource dimensions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			body := map[string]any{"ID": []uint{uint(id)}}

			var raw json.RawMessage
			if err := c.Post("/v1/resource/recalculateDimensions", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Dimensions recalculated successfully.")
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// Version subcommands
// ---------------------------------------------------------------------------

func newResourceVersionsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "versions <resource-id>",
		Short: "List versions of a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("resourceId", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/resource/versions", q, &raw); err != nil {
				return err
			}

			var versions []resourceVersionResponse
			if err := json.Unmarshal(raw, &versions); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "VERSION", "SIZE", "TYPE", "COMMENT", "CREATED"}
			var rows [][]string
			for _, v := range versions {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(v.ID), 10),
					strconv.Itoa(v.VersionNumber),
					formatFileSize(v.FileSize),
					v.ContentType,
					output.Truncate(v.Comment, 40),
					v.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}
}

func newResourceVersionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version <version-id>",
		Short: "Get a specific version by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/resource/version", q, &raw); err != nil {
				return err
			}

			var v resourceVersionResponse
			if err := json.Unmarshal(raw, &v); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fields := []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(v.ID), 10)},
				{Key: "ResourceID", Value: strconv.FormatUint(uint64(v.ResourceID), 10)},
				{Key: "Version", Value: strconv.Itoa(v.VersionNumber)},
				{Key: "Hash", Value: v.Hash},
				{Key: "FileSize", Value: formatFileSize(v.FileSize)},
				{Key: "ContentType", Value: v.ContentType},
				{Key: "Dimensions", Value: formatDimensions(v.Width, v.Height)},
				{Key: "Comment", Value: v.Comment},
				{Key: "Created", Value: v.CreatedAt.Format(time.RFC3339)},
			}

			output.PrintSingle(*opts, fields, raw)
			return nil
		},
	}
}

func newResourceVersionUploadCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var comment string

	cmd := &cobra.Command{
		Use:   "version-upload <resource-id> <file>",
		Short: "Upload a new version of a resource",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("resourceId", args[0])

			extra := map[string]string{}
			if cmd.Flags().Changed("comment") {
				extra["Comment"] = comment
			}

			var raw json.RawMessage
			if err := c.UploadFile("/v1/resource/versions", q, "resource", args[1], extra, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Version uploaded successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&comment, "comment", "", "Version comment")

	return cmd
}

func newResourceVersionDownloadCmd(c *client.Client, _ *output.Options) *cobra.Command {
	var outFile string

	cmd := &cobra.Command{
		Use:   "version-download <version-id>",
		Short: "Download a specific version file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("versionId", args[0])

			dest := outFile
			if dest == "" {
				dest = fmt.Sprintf("version_%s", args[0])
			}

			n, err := c.DownloadFile("/v1/resource/version/file", q, dest)
			if err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Downloaded %s (%s)", dest, formatFileSize(n)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outFile, "output", "o", "", "Output file path (default: version_<id>)")

	return cmd
}

func newResourceVersionRestoreCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var resourceID, versionID uint
	var comment string

	cmd := &cobra.Command{
		Use:   "version-restore",
		Short: "Restore a resource to a previous version",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"ResourceID": resourceID,
				"VersionID":  versionID,
			}
			if cmd.Flags().Changed("comment") {
				body["Comment"] = comment
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resource/version/restore", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Version restored successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&resourceID, "resource-id", 0, "Resource ID (required)")
	cmd.MarkFlagRequired("resource-id")
	cmd.Flags().UintVar(&versionID, "version-id", 0, "Version ID (required)")
	cmd.MarkFlagRequired("version-id")
	cmd.Flags().StringVar(&comment, "comment", "", "Restore comment")

	return cmd
}

func newResourceVersionDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var resourceID, versionID uint

	cmd := &cobra.Command{
		Use:   "version-delete",
		Short: "Delete a specific version",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("resourceId", strconv.FormatUint(uint64(resourceID), 10))
			q.Set("versionId", strconv.FormatUint(uint64(versionID), 10))

			var raw json.RawMessage
			if err := c.Delete("/v1/resource/version", q, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Version deleted successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&resourceID, "resource-id", 0, "Resource ID (required)")
	cmd.MarkFlagRequired("resource-id")
	cmd.Flags().UintVar(&versionID, "version-id", 0, "Version ID (required)")
	cmd.MarkFlagRequired("version-id")

	return cmd
}

func newResourceVersionsCleanupCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var keep, olderThanDays uint
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "versions-cleanup <resource-id>",
		Short: "Clean up old versions of a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			body := map[string]any{"ResourceID": uint(id)}

			if cmd.Flags().Changed("keep") {
				body["KeepLast"] = keep
			}
			if cmd.Flags().Changed("older-than-days") {
				body["OlderThanDays"] = olderThanDays
			}
			if dryRun {
				body["DryRun"] = true
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resource/versions/cleanup", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				if dryRun {
					output.PrintMessage("Dry run completed.")
				} else {
					output.PrintMessage("Versions cleaned up successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&keep, "keep", 0, "Number of versions to keep")
	cmd.Flags().UintVar(&olderThanDays, "older-than-days", 0, "Delete versions older than N days")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without deleting")

	return cmd
}

func newResourceVersionsCompareCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var v1, v2 uint

	cmd := &cobra.Command{
		Use:   "versions-compare <resource-id>",
		Short: "Compare two versions of a resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("resourceId", args[0])
			q.Set("v1", strconv.FormatUint(uint64(v1), 10))
			q.Set("v2", strconv.FormatUint(uint64(v2), 10))

			var raw json.RawMessage
			if err := c.Get("/v1/resource/versions/compare", q, &raw); err != nil {
				return err
			}

			var cmp versionComparisonResponse
			if err := json.Unmarshal(raw, &cmp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fields := []output.KeyValue{
				{Key: "SizeDelta", Value: formatFileSize(cmp.SizeDelta)},
				{Key: "SameHash", Value: strconv.FormatBool(cmp.SameHash)},
				{Key: "SameType", Value: strconv.FormatBool(cmp.SameType)},
				{Key: "DimensionsDiff", Value: strconv.FormatBool(cmp.DimensionsDiff)},
			}

			output.PrintSingle(*opts, fields, raw)
			return nil
		},
	}

	cmd.Flags().UintVar(&v1, "v1", 0, "First version ID (required)")
	cmd.MarkFlagRequired("v1")
	cmd.Flags().UintVar(&v2, "v2", 0, "Second version ID (required)")
	cmd.MarkFlagRequired("v2")

	return cmd
}

// ---------------------------------------------------------------------------
// Plural "resources" command
// ---------------------------------------------------------------------------

// NewResourcesCmd returns the plural "resources" command tree.
func NewResourcesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resources",
		Short: "List, merge, or bulk-edit resources",
	}

	cmd.AddCommand(newResourcesListCmd(c, opts, page))
	cmd.AddCommand(newResourcesAddTagsCmd(c, opts))
	cmd.AddCommand(newResourcesRemoveTagsCmd(c, opts))
	cmd.AddCommand(newResourcesReplaceTagsCmd(c, opts))
	cmd.AddCommand(newResourcesAddGroupsCmd(c, opts))
	cmd.AddCommand(newResourcesAddMetaCmd(c, opts))
	cmd.AddCommand(newResourcesDeleteCmd(c, opts))
	cmd.AddCommand(newResourcesMergeCmd(c, opts))
	cmd.AddCommand(newResourcesSetDimensionsCmd(c, opts))
	cmd.AddCommand(newResourcesVersionsCleanupCmd(c, opts))
	cmd.AddCommand(newResourcesMetaKeysCmd(c, opts))

	return cmd
}

func newResourcesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var (
		name, description, contentType string
		tagsStr, groupsStr, notesStr   string
		createdBefore, createdAfter    string
		hash, originalName             string
		sortByStr                      string
		ownerID, resourceCategoryID    uint
		minWidth, minHeight            uint
		maxWidth, maxHeight            uint
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if name != "" {
				q.Set("name", name)
			}
			if description != "" {
				q.Set("description", description)
			}
			if contentType != "" {
				q.Set("contentType", contentType)
			}
			if cmd.Flags().Changed("owner-id") {
				q.Set("ownerId", strconv.FormatUint(uint64(ownerID), 10))
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
			if notesStr != "" {
				notes, err := parseUintList(notesStr)
				if err != nil {
					return fmt.Errorf("parsing --notes: %w", err)
				}
				for _, n := range notes {
					q.Add("notes", strconv.FormatUint(uint64(n), 10))
				}
			}
			if cmd.Flags().Changed("resource-category-id") {
				q.Set("resourceCategoryId", strconv.FormatUint(uint64(resourceCategoryID), 10))
			}
			if createdBefore != "" {
				q.Set("createdBefore", createdBefore)
			}
			if createdAfter != "" {
				q.Set("createdAfter", createdAfter)
			}
			if cmd.Flags().Changed("min-width") {
				q.Set("minWidth", strconv.FormatUint(uint64(minWidth), 10))
			}
			if cmd.Flags().Changed("min-height") {
				q.Set("minHeight", strconv.FormatUint(uint64(minHeight), 10))
			}
			if cmd.Flags().Changed("max-width") {
				q.Set("maxWidth", strconv.FormatUint(uint64(maxWidth), 10))
			}
			if cmd.Flags().Changed("max-height") {
				q.Set("maxHeight", strconv.FormatUint(uint64(maxHeight), 10))
			}
			if hash != "" {
				q.Set("hash", hash)
			}
			if originalName != "" {
				q.Set("originalName", originalName)
			}
			if sortByStr != "" {
				parts := strings.Split(sortByStr, ",")
				for _, s := range parts {
					s = strings.TrimSpace(s)
					if s != "" {
						q.Add("sortBy", s)
					}
				}
			}

			var raw json.RawMessage
			if err := c.Get("/v1/resources", q, &raw); err != nil {
				return err
			}

			var resources []resourceResponse
			if err := json.Unmarshal(raw, &resources); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "TYPE", "SIZE", "DIMENSIONS", "OWNER_ID", "CREATED"}
			var rows [][]string
			for _, r := range resources {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(r.ID), 10),
					output.Truncate(r.Name, 40),
					r.ContentType,
					formatFileSize(r.FileSize),
					formatDimensions(r.Width, r.Height),
					ptrUintStr(r.OwnerId),
					r.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Filter by content type")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Filter by owner group ID")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs to filter by")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs to filter by")
	cmd.Flags().StringVar(&notesStr, "notes", "", "Comma-separated note IDs to filter by")
	cmd.Flags().UintVar(&resourceCategoryID, "resource-category-id", 0, "Filter by resource category ID")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "Filter by creation date (before)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Filter by creation date (after)")
	cmd.Flags().UintVar(&minWidth, "min-width", 0, "Minimum width")
	cmd.Flags().UintVar(&minHeight, "min-height", 0, "Minimum height")
	cmd.Flags().UintVar(&maxWidth, "max-width", 0, "Maximum width")
	cmd.Flags().UintVar(&maxHeight, "max-height", 0, "Maximum height")
	cmd.Flags().StringVar(&hash, "hash", "", "Filter by hash")
	cmd.Flags().StringVar(&originalName, "original-name", "", "Filter by original name")
	cmd.Flags().StringVar(&sortByStr, "sort-by", "", "Comma-separated sort fields")

	return cmd
}

func newResourcesAddTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "add-tags",
		Short: "Add tags to multiple resources",
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
			if err := c.Post("/v1/resources/addTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags added to resources successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newResourcesRemoveTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "remove-tags",
		Short: "Remove tags from multiple resources",
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
			if err := c.Post("/v1/resources/removeTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags removed from resources successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newResourcesReplaceTagsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, tagsStr string

	cmd := &cobra.Command{
		Use:   "replace-tags",
		Short: "Replace tags on multiple resources",
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
			if err := c.Post("/v1/resources/replaceTags", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags replaced on resources successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs (required)")
	cmd.MarkFlagRequired("tags")

	return cmd
}

func newResourcesAddGroupsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, groupsStr string

	cmd := &cobra.Command{
		Use:   "add-groups",
		Short: "Add groups to multiple resources",
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
			if err := c.Post("/v1/resources/addGroups", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Groups added to resources successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs (required)")
	cmd.MarkFlagRequired("groups")

	return cmd
}

func newResourcesAddMetaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr, meta string

	cmd := &cobra.Command{
		Use:   "add-meta",
		Short: "Add metadata to multiple resources",
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
			if err := c.Post("/v1/resources/addMeta", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Metadata added to resources successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().StringVar(&meta, "meta", "", "Meta JSON string (required)")
	cmd.MarkFlagRequired("meta")

	return cmd
}

func newResourcesDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete multiple resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}

			body := map[string]any{
				"ID": ids,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resources/delete", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resources deleted successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs to delete (required)")
	cmd.MarkFlagRequired("ids")

	return cmd
}

func newResourcesMergeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var winner uint
	var losersStr string

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge resources into a winner",
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
			if err := c.Post("/v1/resources/merge", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resources merged successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&winner, "winner", 0, "Winning resource ID (required)")
	cmd.MarkFlagRequired("winner")
	cmd.Flags().StringVar(&losersStr, "losers", "", "Comma-separated loser resource IDs (required)")
	cmd.MarkFlagRequired("losers")

	return cmd
}

func newResourcesSetDimensionsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr string
	var width, height uint

	cmd := &cobra.Command{
		Use:   "set-dimensions",
		Short: "Set dimensions on multiple resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseUintList(idsStr)
			if err != nil {
				return fmt.Errorf("parsing --ids: %w", err)
			}

			body := map[string]any{
				"ID":     ids,
				"Width":  width,
				"Height": height,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resources/setDimensions", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Dimensions set successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated resource IDs (required)")
	cmd.MarkFlagRequired("ids")
	cmd.Flags().UintVar(&width, "width", 0, "Width in pixels (required)")
	cmd.MarkFlagRequired("width")
	cmd.Flags().UintVar(&height, "height", 0, "Height in pixels (required)")
	cmd.MarkFlagRequired("height")

	return cmd
}

func newResourcesVersionsCleanupCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var keep, olderThanDays, ownerID uint
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "versions-cleanup",
		Short: "Clean up old versions across resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}

			if cmd.Flags().Changed("keep") {
				body["KeepLast"] = keep
			}
			if cmd.Flags().Changed("older-than-days") {
				body["OlderThanDays"] = olderThanDays
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerID"] = ownerID
			}
			if dryRun {
				body["DryRun"] = true
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resources/versions/cleanup", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				if dryRun {
					output.PrintMessage("Dry run completed.")
				} else {
					output.PrintMessage("Versions cleaned up successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&keep, "keep", 0, "Number of versions to keep")
	cmd.Flags().UintVar(&olderThanDays, "older-than-days", 0, "Delete versions older than N days")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Filter by owner group ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without deleting")

	return cmd
}

func newResourcesMetaKeysCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "meta-keys",
		Short: "List all unique metadata keys used across resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/resources/meta/keys", nil, &raw); err != nil {
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

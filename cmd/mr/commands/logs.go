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

// logEntryResponse matches the API's LogEntry JSON shape (lowercase keys).
type logEntryResponse struct {
	ID          uint            `json:"id"`
	Level       string          `json:"level"`
	Action      string          `json:"action"`
	EntityType  string          `json:"entityType"`
	EntityID    *uint           `json:"entityId"`
	EntityName  string          `json:"entityName"`
	Message     string          `json:"message"`
	Details     json.RawMessage `json:"details"`
	RequestPath string          `json:"requestPath"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// logsListResponse wraps the paginated logs response.
type logsListResponse struct {
	Logs       []logEntryResponse `json:"logs"`
	TotalCount int                `json:"totalCount"`
	Page       int                `json:"page"`
	PerPage    int                `json:"perPage"`
}

// NewLogCmd returns the singular "log" command with get/entity subcommands.
func NewLogCmd(c *client.Client, opts *output.Options) *cobra.Command {
	logCmd := &cobra.Command{
		Use:   "log",
		Short: "Operate on a single log entry",
	}

	logCmd.AddCommand(newLogGetCmd(c, opts))
	logCmd.AddCommand(newLogEntityCmd(c, opts))

	return logCmd
}

func newLogGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a log entry by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/log", q, &raw); err != nil {
				return err
			}

			var entry logEntryResponse
			if err := json.Unmarshal(raw, &entry); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			entityID := ""
			if entry.EntityID != nil {
				entityID = strconv.FormatUint(uint64(*entry.EntityID), 10)
			}

			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(entry.ID), 10)},
				{Key: "Level", Value: entry.Level},
				{Key: "Action", Value: entry.Action},
				{Key: "EntityType", Value: entry.EntityType},
				{Key: "EntityID", Value: entityID},
				{Key: "EntityName", Value: entry.EntityName},
				{Key: "Message", Value: entry.Message},
				{Key: "Details", Value: string(entry.Details)},
				{Key: "RequestPath", Value: entry.RequestPath},
				{Key: "Created", Value: entry.CreatedAt.Format(time.RFC3339)},
			}, raw)
			return nil
		},
	}
}

func newLogEntityCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var entityType string
	var entityID uint

	cmd := &cobra.Command{
		Use:   "entity",
		Short: "Get log entries for a specific entity",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("entityType", entityType)
			q.Set("entityId", strconv.FormatUint(uint64(entityID), 10))

			var raw json.RawMessage
			if err := c.Get("/v1/logs/entity", q, &raw); err != nil {
				return err
			}

			var entries []logEntryResponse
			if err := json.Unmarshal(raw, &entries); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			printLogEntries(*opts, entries, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&entityType, "entity-type", "", "Entity type (required)")
	cmd.MarkFlagRequired("entity-type")
	cmd.Flags().UintVar(&entityID, "entity-id", 0, "Entity ID (required)")
	cmd.MarkFlagRequired("entity-id")

	return cmd
}

// NewLogsCmd returns the plural "logs" command with list subcommand.
func NewLogsCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Operate on multiple log entries",
	}

	logsCmd.AddCommand(newLogsListCmd(c, opts, page))

	return logsCmd
}

func newLogsListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var level, action, entityType, message, createdBefore, createdAfter string
	var entityID uint

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if level != "" {
				q.Set("level", level)
			}
			if action != "" {
				q.Set("action", action)
			}
			if entityType != "" {
				q.Set("entityType", entityType)
			}
			if cmd.Flags().Changed("entity-id") {
				q.Set("entityId", strconv.FormatUint(uint64(entityID), 10))
			}
			if message != "" {
				q.Set("message", message)
			}
			if createdBefore != "" {
				q.Set("createdBefore", createdBefore)
			}
			if createdAfter != "" {
				q.Set("createdAfter", createdAfter)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/logs", q, &raw); err != nil {
				return err
			}

			var resp logsListResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			printLogEntries(*opts, resp.Logs, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&level, "level", "", "Filter by level (info/warning/error)")
	cmd.Flags().StringVar(&action, "action", "", "Filter by action (create/update/delete/system)")
	cmd.Flags().StringVar(&entityType, "entity-type", "", "Filter by entity type")
	cmd.Flags().UintVar(&entityID, "entity-id", 0, "Filter by entity ID")
	cmd.Flags().StringVar(&message, "message", "", "Filter by message")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "Filter by created before (RFC3339)")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "Filter by created after (RFC3339)")

	return cmd
}

func printLogEntries(opts output.Options, entries []logEntryResponse, raw json.RawMessage) {
	columns := []string{"ID", "LEVEL", "ACTION", "ENTITY_TYPE", "ENTITY_ID", "MESSAGE", "CREATED"}
	var rows [][]string
	for _, e := range entries {
		eid := ""
		if e.EntityID != nil {
			eid = strconv.FormatUint(uint64(*e.EntityID), 10)
		}
		rows = append(rows, []string{
			strconv.FormatUint(uint64(e.ID), 10),
			e.Level,
			e.Action,
			e.EntityType,
			eid,
			output.Truncate(e.Message, 50),
			e.CreatedAt.Format(time.RFC3339),
		})
	}

	output.Print(opts, columns, rows, raw)
}

package commands

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed admin_help/*.md
var adminHelpFS embed.FS

// adminServerStatsResponse matches the ServerStats JSON shape from admin_context.go.
type adminServerStatsResponse struct {
	Uptime              string    `json:"uptime"`
	UptimeSeconds       float64   `json:"uptimeSeconds"`
	StartedAt           time.Time `json:"startedAt"`
	HeapAlloc           uint64    `json:"heapAlloc"`
	HeapInUse           uint64    `json:"heapInUse"`
	Sys                 uint64    `json:"sys"`
	NumGC               uint32    `json:"numGC"`
	HeapAllocFmt        string    `json:"heapAllocFmt"`
	HeapInUseFmt        string    `json:"heapInUseFmt"`
	SysFmt              string    `json:"sysFmt"`
	Goroutines          int       `json:"goroutines"`
	GoVersion           string    `json:"goVersion"`
	DBType              string    `json:"dbType"`
	DBOpenConns         int       `json:"dbOpenConns"`
	DBIdleConns         int       `json:"dbIdleConns"`
	DBInUse             int       `json:"dbInUse"`
	DBFileSizeBytes     int64     `json:"dbFileSizeBytes"`
	DBFileSizeFmt       string    `json:"dbFileSizeFmt"`
	HashWorkerEnabled   bool      `json:"hashWorkerEnabled"`
	HashWorkerCount     int       `json:"hashWorkerCount"`
	DownloadQueueLength int       `json:"downloadQueueLength"`
}

// adminEntityCountsResponse matches the EntityCounts JSON shape.
type adminEntityCountsResponse struct {
	Resources          int64 `json:"resources"`
	Notes              int64 `json:"notes"`
	Groups             int64 `json:"groups"`
	Tags               int64 `json:"tags"`
	Categories         int64 `json:"categories"`
	ResourceCategories int64 `json:"resourceCategories"`
	NoteTypes          int64 `json:"noteTypes"`
	Queries            int64 `json:"queries"`
	Relations          int64 `json:"relations"`
	RelationTypes      int64 `json:"relationTypes"`
	LogEntries         int64 `json:"logEntries"`
	ResourceVersions   int64 `json:"resourceVersions"`
}

// adminGrowthPeriodsResponse matches the GrowthPeriods JSON shape.
type adminGrowthPeriodsResponse struct {
	Resources int64 `json:"resources"`
	Notes     int64 `json:"notes"`
	Groups    int64 `json:"groups"`
}

// adminGrowthStatsResponse matches the GrowthStats JSON shape.
type adminGrowthStatsResponse struct {
	Last7Days  adminGrowthPeriodsResponse `json:"last7Days"`
	Last30Days adminGrowthPeriodsResponse `json:"last30Days"`
	Last90Days adminGrowthPeriodsResponse `json:"last90Days"`
}

// adminConfigSummaryResponse matches the ConfigSummary JSON shape.
type adminConfigSummaryResponse struct {
	DbType                  string   `json:"dbType"`
	EphemeralMode           bool     `json:"ephemeralMode"`
	MemoryDB                bool     `json:"memoryDb"`
	MemoryFS                bool     `json:"memoryFs"`
	FTSEnabled              bool     `json:"ftsEnabled"`
	HashWorkerEnabled       bool     `json:"hashWorkerEnabled"`
	BindAddress             string   `json:"bindAddress"`
	FileSavePath            string   `json:"fileSavePath"`
	DbDsn                   string   `json:"dbDsn"`
	HasReadOnlyDB           bool     `json:"hasReadOnlyDB"`
	FfmpegAvailable         bool     `json:"ffmpegAvailable"`
	LibreOfficeAvailable    bool     `json:"libreOfficeAvailable"`
	HashWorkerCount         int      `json:"hashWorkerCount"`
	HashBatchSize           int      `json:"hashBatchSize"`
	HashPollInterval        string   `json:"hashPollInterval"`
	HashSimilarityThreshold int      `json:"hashSimilarityThreshold"`
	HashCacheSize           int      `json:"hashCacheSize"`
	AltFileSystems          []string `json:"altFileSystems"`
	MaxDBConnections        int      `json:"maxDBConnections"`
	RemoteConnectTimeout    string   `json:"remoteConnectTimeout"`
	RemoteIdleTimeout       string   `json:"remoteIdleTimeout"`
	RemoteOverallTimeout    string   `json:"remoteOverallTimeout"`
}

// adminDataStatsResponse matches the DataStats JSON shape.
type adminDataStatsResponse struct {
	Entities                     adminEntityCountsResponse `json:"entities"`
	StorageTotalBytes            int64                     `json:"storageTotalBytes"`
	StorageTotalFmt              string                    `json:"storageTotalFmt"`
	TotalVersionStorageBytes     int64                     `json:"totalVersionStorageBytes"`
	TotalVersionStorageFormatted string                    `json:"totalVersionStorageFormatted"`
	Growth                       adminGrowthStatsResponse  `json:"growth"`
	Config                       adminConfigSummaryResponse `json:"config"`
}

// adminContentTypeStorageResponse matches the ContentTypeStorage JSON shape.
type adminContentTypeStorageResponse struct {
	ContentType string `json:"contentType"`
	TotalBytes  int64  `json:"totalBytes"`
	TotalFmt    string `json:"totalFmt"`
	Count       int64  `json:"count"`
}

// adminOrphanStatsResponse matches the OrphanStats JSON shape.
type adminOrphanStatsResponse struct {
	WithoutTags   int64 `json:"withoutTags"`
	WithoutGroups int64 `json:"withoutGroups"`
}

// adminSimilarityInfoResponse matches the SimilarityInfo JSON shape.
type adminSimilarityInfoResponse struct {
	TotalHashes       int64 `json:"totalHashes"`
	SimilarPairsFound int64 `json:"similarPairsFound"`
}

// adminLogStatsInfoResponse matches the LogStatsInfo JSON shape.
type adminLogStatsInfoResponse struct {
	TotalEntries int64            `json:"totalEntries"`
	ByLevel      map[string]int64 `json:"byLevel"`
	RecentErrors int64            `json:"recentErrors"`
}

// adminExpensiveStatsResponse matches the ExpensiveStats JSON shape.
type adminExpensiveStatsResponse struct {
	StorageByContentType []adminContentTypeStorageResponse `json:"storageByContentType"`
	TopTags              []struct {
		ID    uint   `json:"id"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	} `json:"topTags"`
	TopCategories []struct {
		ID    uint   `json:"id"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	} `json:"topCategories"`
	Orphans    adminOrphanStatsResponse    `json:"orphans"`
	Similarity adminSimilarityInfoResponse `json:"similarity"`
	LogStats   adminLogStatsInfoResponse   `json:"logStats"`
}

// NewAdminCmd returns the "admin" command with optional --server and --data flags.
func NewAdminCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var serverOnly bool
	var dataOnly bool

	help := helptext.Load(adminHelpFS, "admin_help/admin.md")
	cmd := &cobra.Command{
		Use:         "admin",
		Short:       "Show server and data statistics",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine which sections to fetch
			fetchServer := serverOnly || (!serverOnly && !dataOnly)
			fetchData := dataOnly || (!serverOnly && !dataOnly)
			fetchExpensive := !serverOnly && !dataOnly

			if opts.JSON {
				// In JSON mode, fetch only one endpoint (or all three separately)
				if serverOnly {
					var raw json.RawMessage
					if err := c.Get("/v1/admin/server-stats", nil, &raw); err != nil {
						return err
					}
					output.PrintRawJSON(raw)
					return nil
				}
				if dataOnly {
					var raw json.RawMessage
					if err := c.Get("/v1/admin/data-stats", nil, &raw); err != nil {
						return err
					}
					output.PrintRawJSON(raw)
					return nil
				}
				// Default: fetch all three and print each
				var serverRaw, dataRaw, expensiveRaw json.RawMessage
				if err := c.Get("/v1/admin/server-stats", nil, &serverRaw); err != nil {
					return err
				}
				if err := c.Get("/v1/admin/data-stats", nil, &dataRaw); err != nil {
					return err
				}
				if err := c.Get("/v1/admin/data-stats/expensive", nil, &expensiveRaw); err != nil {
					return err
				}
				// Combine into a single JSON object
				combined := map[string]json.RawMessage{
					"serverStats":    serverRaw,
					"dataStats":      dataRaw,
					"expensiveStats": expensiveRaw,
				}
				raw, err := json.Marshal(combined)
				if err != nil {
					return fmt.Errorf("combining stats: %w", err)
				}
				output.PrintRawJSON(raw)
				return nil
			}

			// Human-readable output
			if fetchServer {
				var raw json.RawMessage
				if err := c.Get("/v1/admin/server-stats", nil, &raw); err != nil {
					return err
				}
				var s adminServerStatsResponse
				if err := json.Unmarshal(raw, &s); err != nil {
					return fmt.Errorf("parsing server stats: %w", err)
				}
				fmt.Println("=== Server Health ===")
				output.PrintSingle(*opts, []output.KeyValue{
					{Key: "Uptime", Value: s.Uptime},
					{Key: "Started At", Value: s.StartedAt.Format(time.RFC3339)},
					{Key: "Go Version", Value: s.GoVersion},
					{Key: "Goroutines", Value: strconv.Itoa(s.Goroutines)},
					{Key: "Heap Alloc", Value: s.HeapAllocFmt},
					{Key: "Heap In Use", Value: s.HeapInUseFmt},
					{Key: "Sys Memory", Value: s.SysFmt},
					{Key: "GC Cycles", Value: strconv.FormatUint(uint64(s.NumGC), 10)},
					{Key: "DB Type", Value: s.DBType},
					{Key: "DB Open Conns", Value: strconv.Itoa(s.DBOpenConns)},
					{Key: "DB Idle Conns", Value: strconv.Itoa(s.DBIdleConns)},
					{Key: "DB In Use", Value: strconv.Itoa(s.DBInUse)},
					{Key: "DB File Size", Value: s.DBFileSizeFmt},
					{Key: "Hash Worker Enabled", Value: strconv.FormatBool(s.HashWorkerEnabled)},
					{Key: "Hash Worker Count", Value: strconv.Itoa(s.HashWorkerCount)},
					{Key: "Download Queue Length", Value: strconv.Itoa(s.DownloadQueueLength)},
				}, nil)
			}

			if fetchData {
				var raw json.RawMessage
				if err := c.Get("/v1/admin/data-stats", nil, &raw); err != nil {
					return err
				}
				var d adminDataStatsResponse
				if err := json.Unmarshal(raw, &d); err != nil {
					return fmt.Errorf("parsing data stats: %w", err)
				}
				fmt.Println("\n=== Data Stats ===")
				output.PrintSingle(*opts, []output.KeyValue{
					{Key: "Resources", Value: strconv.FormatInt(d.Entities.Resources, 10)},
					{Key: "Notes", Value: strconv.FormatInt(d.Entities.Notes, 10)},
					{Key: "Groups", Value: strconv.FormatInt(d.Entities.Groups, 10)},
					{Key: "Tags", Value: strconv.FormatInt(d.Entities.Tags, 10)},
					{Key: "Categories", Value: strconv.FormatInt(d.Entities.Categories, 10)},
					{Key: "Resource Categories", Value: strconv.FormatInt(d.Entities.ResourceCategories, 10)},
					{Key: "Note Types", Value: strconv.FormatInt(d.Entities.NoteTypes, 10)},
					{Key: "Queries", Value: strconv.FormatInt(d.Entities.Queries, 10)},
					{Key: "Relations", Value: strconv.FormatInt(d.Entities.Relations, 10)},
					{Key: "Relation Types", Value: strconv.FormatInt(d.Entities.RelationTypes, 10)},
					{Key: "Log Entries", Value: strconv.FormatInt(d.Entities.LogEntries, 10)},
					{Key: "Resource Versions", Value: strconv.FormatInt(d.Entities.ResourceVersions, 10)},
					{Key: "Storage Total", Value: d.StorageTotalFmt},
					{Key: "Version Storage", Value: d.TotalVersionStorageFormatted},
					{Key: "Growth (7d) Resources", Value: strconv.FormatInt(d.Growth.Last7Days.Resources, 10)},
					{Key: "Growth (7d) Notes", Value: strconv.FormatInt(d.Growth.Last7Days.Notes, 10)},
					{Key: "Growth (7d) Groups", Value: strconv.FormatInt(d.Growth.Last7Days.Groups, 10)},
					{Key: "Growth (30d) Resources", Value: strconv.FormatInt(d.Growth.Last30Days.Resources, 10)},
					{Key: "Growth (30d) Notes", Value: strconv.FormatInt(d.Growth.Last30Days.Notes, 10)},
					{Key: "Growth (30d) Groups", Value: strconv.FormatInt(d.Growth.Last30Days.Groups, 10)},
					{Key: "Growth (90d) Resources", Value: strconv.FormatInt(d.Growth.Last90Days.Resources, 10)},
					{Key: "Growth (90d) Notes", Value: strconv.FormatInt(d.Growth.Last90Days.Notes, 10)},
					{Key: "Growth (90d) Groups", Value: strconv.FormatInt(d.Growth.Last90Days.Groups, 10)},
				}, nil)

				fmt.Println("\n--- Config ---")
				output.PrintSingle(*opts, []output.KeyValue{
					{Key: "DB Type", Value: d.Config.DbType},
					{Key: "Bind Address", Value: d.Config.BindAddress},
					{Key: "File Save Path", Value: d.Config.FileSavePath},
					{Key: "DB DSN", Value: d.Config.DbDsn},
					{Key: "Ephemeral Mode", Value: strconv.FormatBool(d.Config.EphemeralMode)},
					{Key: "Memory DB", Value: strconv.FormatBool(d.Config.MemoryDB)},
					{Key: "Memory FS", Value: strconv.FormatBool(d.Config.MemoryFS)},
					{Key: "FTS Enabled", Value: strconv.FormatBool(d.Config.FTSEnabled)},
					{Key: "Has Read-Only DB", Value: strconv.FormatBool(d.Config.HasReadOnlyDB)},
					{Key: "FFmpeg Available", Value: strconv.FormatBool(d.Config.FfmpegAvailable)},
					{Key: "LibreOffice Available", Value: strconv.FormatBool(d.Config.LibreOfficeAvailable)},
					{Key: "Max DB Connections", Value: strconv.Itoa(d.Config.MaxDBConnections)},
					{Key: "Remote Connect Timeout", Value: d.Config.RemoteConnectTimeout},
					{Key: "Remote Idle Timeout", Value: d.Config.RemoteIdleTimeout},
					{Key: "Remote Overall Timeout", Value: d.Config.RemoteOverallTimeout},
				}, nil)
			}

			if fetchExpensive {
				var raw json.RawMessage
				if err := c.Get("/v1/admin/data-stats/expensive", nil, &raw); err != nil {
					return err
				}
				var e adminExpensiveStatsResponse
				if err := json.Unmarshal(raw, &e); err != nil {
					return fmt.Errorf("parsing expensive stats: %w", err)
				}

				fmt.Println("\n=== Storage by Content Type ===")
				columns := []string{"CONTENT_TYPE", "COUNT", "SIZE"}
				var rows [][]string
				for _, ct := range e.StorageByContentType {
					ct := ct
					name := ct.ContentType
					if name == "" {
						name = "(unknown)"
					}
					rows = append(rows, []string{
						name,
						strconv.FormatInt(ct.Count, 10),
						ct.TotalFmt,
					})
				}
				output.Print(*opts, columns, rows, nil)

				fmt.Println("\n=== Orphan Stats ===")
				output.PrintSingle(*opts, []output.KeyValue{
					{Key: "Resources Without Tags", Value: strconv.FormatInt(e.Orphans.WithoutTags, 10)},
					{Key: "Resources Without Groups", Value: strconv.FormatInt(e.Orphans.WithoutGroups, 10)},
				}, nil)

				fmt.Println("\n=== Similarity Stats ===")
				output.PrintSingle(*opts, []output.KeyValue{
					{Key: "Total Hashes", Value: strconv.FormatInt(e.Similarity.TotalHashes, 10)},
					{Key: "Similar Pairs Found", Value: strconv.FormatInt(e.Similarity.SimilarPairsFound, 10)},
				}, nil)

				fmt.Println("\n=== Log Stats ===")
				kvs := []output.KeyValue{
					{Key: "Total Log Entries", Value: strconv.FormatInt(e.LogStats.TotalEntries, 10)},
					{Key: "Recent Errors (24h)", Value: strconv.FormatInt(e.LogStats.RecentErrors, 10)},
				}
				for level, count := range e.LogStats.ByLevel {
					kvs = append(kvs, output.KeyValue{
						Key:   "Level: " + level,
						Value: strconv.FormatInt(count, 10),
					})
				}
				output.PrintSingle(*opts, kvs, nil)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&serverOnly, "server-only", false, "Show server stats only")
	cmd.Flags().BoolVar(&dataOnly, "data-only", false, "Show data stats only")

	return cmd
}

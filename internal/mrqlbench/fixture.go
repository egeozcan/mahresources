package mrqlbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	modeltypes "mahresources/models/types"
)

var FixedEpoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

type PrepareOptions struct {
	Profile   Profile
	Dialect   string
	BatchSize int
	Revision  string
}

type benchmarkMarker struct {
	ID              uint   `gorm:"primaryKey"`
	Marker          string `gorm:"uniqueIndex"`
	Version         string
	ProfileID       string
	ResourceCount   int
	LogicalChecksum string
	SchemaRevision  string
	FTSReady        bool
	Dirty           bool
}

func (benchmarkMarker) TableName() string { return "mrql_benchmark_marker" }

type resourceTag struct{ ResourceID, TagID uint }

func (resourceTag) TableName() string { return "resource_tags" }

type noteTag struct{ NoteID, TagID uint }

func (noteTag) TableName() string { return "note_tags" }

type groupTag struct{ GroupID, TagID uint }

func (groupTag) TableName() string { return "group_tags" }

type groupResource struct{ GroupID, ResourceID uint }

func (groupResource) TableName() string { return "groups_related_resources" }

type groupNote struct{ GroupID, NoteID uint }

func (groupNote) TableName() string { return "groups_related_notes" }

type resourceNote struct{ ResourceID, NoteID uint }

func (resourceNote) TableName() string { return "resource_notes" }

func TinyProfile() Profile {
	return Profile{ID: "tiny", Resources: 240, Notes: 60, Groups: 32, Tags: 8, Seed: 1}
}

func PrepareFixture(ctx context.Context, db *gorm.DB, options PrepareOptions) (FixtureManifest, error) {
	started := time.Now()
	if options.Profile.Resources <= 0 || options.Profile.Notes < 0 || options.Profile.Groups < 4 || options.Profile.Tags < 2 || options.Profile.Seed < 0 {
		return FixtureManifest{}, errors.New("fixture profile has invalid cardinalities")
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 500
	}
	if options.Revision == "" {
		options.Revision = "production-schema-v1"
	}
	if db.Migrator().HasTable(&benchmarkMarker{}) {
		return FixtureManifest{}, errors.New("database already contains an MRQL benchmark fixture marker")
	}
	if err := migrateFixtureSchema(db, options.Dialect); err != nil {
		return FixtureManifest{}, err
	}
	marker := benchmarkMarker{ID: 1, Marker: "mahresources-mrql-benchmark", Version: GeneratorVersion}
	if err := db.Create(&marker).Error; err != nil {
		return FixtureManifest{}, fmt.Errorf("create fixture marker: %w", err)
	}

	checksum := sha256.New()
	checksumWrite(checksum, "profile", options.Profile.ID, options.Profile.Resources, options.Profile.Notes, options.Profile.Groups, options.Profile.Tags, options.Profile.Seed)
	counts := map[string]int64{}
	anchors := map[string]any{
		"rootGroupId": uint(1), "deepRootGroupId": uint(2), "emptyGroupId": uint(3),
		"similarityTargetResourceId": uint(1), "commonTagId": uint(1), "ftsTerm": "benchmarkneedle",
	}
	if err := seedLookups(ctx, db, options.Profile.Seed, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := seedGroups(ctx, db, options, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := seedTags(ctx, db, options, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := seedResources(ctx, db, options, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := seedNotes(ctx, db, options, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := seedRelations(ctx, db, options, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := seedSimilarity(ctx, db, options, checksum); err != nil {
		return FixtureManifest{}, err
	}
	if err := models.EnsureSupplementalIndexes(db); err != nil {
		return FixtureManifest{}, fmt.Errorf("supplemental indexes: %w", err)
	}

	appCtx := application_context.NewMahresourcesContext(afero.NewMemMapFs(), db, nil, &application_context.MahresourcesConfig{
		DbType: options.Dialect, AltFileSystems: map[string]string{}, PluginsDisabled: true, MRQLDefaultLimit: 500,
	})
	if err := appCtx.InitFTS(); err != nil {
		return FixtureManifest{}, fmt.Errorf("initialize FTS: %w", err)
	}
	ftsChecks, ftsDigest, err := collectFTSChecks(db, "benchmarkneedle")
	if err != nil {
		return FixtureManifest{}, err
	}
	if err := analyzeFixture(db); err != nil {
		return FixtureManifest{}, err
	}
	if err := collectFixtureCounts(db, counts); err != nil {
		return FixtureManifest{}, err
	}
	if err := installFixtureMutationGuards(db); err != nil {
		return FixtureManifest{}, err
	}
	if counts["resources"] != int64(options.Profile.Resources) || counts["notes"] != int64(options.Profile.Notes) || counts["groups"] != int64(options.Profile.Groups) {
		return FixtureManifest{}, fmt.Errorf("fixture count validation failed: %#v", counts)
	}
	version, err := databaseVersion(db)
	if err != nil {
		return FixtureManifest{}, err
	}
	manifest := FixtureManifest{
		SchemaVersion: ArtifactSchemaVersion, GeneratorVersion: GeneratorVersion,
		Profile: options.Profile, Dialect: normalizeDialect(options.Dialect), DatabaseVersion: version,
		SchemaRevision: options.Revision, FixedEpoch: FixedEpoch, BatchSize: options.BatchSize,
		Features: map[string]bool{"fts": true, "similarity": true, "hierarchy": true, "metadata": true},
		Counts:   counts, FTSChecks: ftsChecks, FTSDigest: ftsDigest, Anchors: anchors, LogicalChecksum: hex.EncodeToString(checksum.Sum(nil)),
		PreparedAt: time.Now().UTC(), PreparationNanos: time.Since(started).Nanoseconds(), Marker: marker.Marker,
	}
	if err := db.Model(&benchmarkMarker{}).Where("id = ?", marker.ID).Updates(map[string]any{
		"profile_id": manifest.Profile.ID, "resource_count": manifest.Profile.Resources,
		"logical_checksum": manifest.LogicalChecksum, "schema_revision": manifest.SchemaRevision, "fts_ready": true, "dirty": false,
	}).Error; err != nil {
		return FixtureManifest{}, fmt.Errorf("finalize fixture marker: %w", err)
	}
	return manifest, nil
}

// CountPostgresUserTables counts every non-system schema so preparation cannot
// be redirected to an existing schema through connection parameters.
func CountPostgresUserTables(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') AND table_schema NOT LIKE 'pg_toast%'`).Scan(&count).Error
	return count, err
}

func ValidateFixture(db *gorm.DB, manifest FixtureManifest) error {
	if manifest.SchemaVersion != ArtifactSchemaVersion || manifest.GeneratorVersion != GeneratorVersion || manifest.Marker != "mahresources-mrql-benchmark" {
		return errors.New("fixture manifest is incompatible")
	}
	var marker benchmarkMarker
	if err := db.First(&marker, 1).Error; err != nil {
		return fmt.Errorf("read fixture marker: %w", err)
	}
	if marker.Marker != manifest.Marker || marker.Version != manifest.GeneratorVersion || marker.ProfileID != manifest.Profile.ID ||
		marker.ResourceCount != manifest.Profile.Resources || marker.LogicalChecksum != manifest.LogicalChecksum ||
		marker.SchemaRevision != manifest.SchemaRevision || !marker.FTSReady || marker.Dirty {
		return errors.New("fixture marker does not match manifest")
	}
	if normalizeDialect(db.Dialector.Name()) != manifest.Dialect {
		return fmt.Errorf("fixture database dialect %q does not match manifest %q", db.Dialector.Name(), manifest.Dialect)
	}
	for key, model := range fixtureCountModels() {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			return fmt.Errorf("validate fixture %s count: %w", key, err)
		}
		if expected, ok := manifest.Counts[key]; !ok || count != expected {
			return fmt.Errorf("fixture %s count %d does not match manifest %d", key, count, expected)
		}
	}
	term, _ := manifest.Anchors["ftsTerm"].(string)
	ftsChecks, ftsDigest, err := collectFTSChecks(db, term)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(ftsChecks, manifest.FTSChecks) || ftsDigest != manifest.FTSDigest {
		return errors.New("fixture FTS integrity does not match manifest")
	}
	return nil
}

func migrateFixtureSchema(db *gorm.DB, dialect string) error {
	if normalizeDialect(dialect) == "sqlite" {
		if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		defer db.Exec("PRAGMA foreign_keys = ON")
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Series{}, &models.Tag{}, &models.Category{}, &models.ResourceCategory{},
		&models.NoteType{}, &models.LogEntry{}, &models.PluginState{}, &models.PluginKV{},
		&models.RuntimeSetting{}, &models.SavedMRQLQuery{}, &models.TemplatePartial{}, &models.Group{},
		&models.GroupRelationType{}, &models.Resource{}, &models.User{}, &models.UserSetting{}, &models.Note{},
		&models.ResourceVersion{}, &models.NoteBlock{}, &models.Preview{}, &models.GroupRelation{},
		&models.ImageHash{}, &models.ResourceSimilarity{}, &models.Session{}, &models.ApiToken{}, &benchmarkMarker{},
	); err != nil {
		return fmt.Errorf("migrate fixture schema: %w", err)
	}
	return nil
}

func seedLookups(ctx context.Context, db *gorm.DB, seed int64, checksum hash.Hash) error {
	resourceCategoryGUID := deterministicGUID("resource-category", 1, seed)
	noteTypeGUID := deterministicGUID("note-type", 1, seed)
	categoryGUID := deterministicGUID("category", 1, seed)
	relationTypeGUID := deterministicGUID("relation-type", 1, seed)
	lookups := []any{
		&models.ResourceCategory{ID: 1, CreatedAt: FixedEpoch, UpdatedAt: FixedEpoch, GUID: &resourceCategoryGUID, Name: "benchmark-resource", CustomMRQLResult: `<article>[property path="Name"]</article>`, CustomCSS: ".benchmark{}"},
		&models.NoteType{ID: 1, CreatedAt: FixedEpoch, UpdatedAt: FixedEpoch, GUID: &noteTypeGUID, Name: "benchmark-note", CustomMRQLResult: `<article>[property path="Name"]</article>`},
		&models.Category{ID: 1, CreatedAt: FixedEpoch, UpdatedAt: FixedEpoch, GUID: &categoryGUID, Name: "benchmark-group", CustomMRQLResult: `<article>[property path="Name"] [mrql query='type = "resource" ORDER BY id ASC LIMIT 1' format="compact"]</article>`},
		&models.Query{ID: 1, CreatedAt: FixedEpoch, UpdatedAt: FixedEpoch, Name: "benchmark-query", Text: `SELECT 1`},
		&models.GroupRelationType{ID: 1, CreatedAt: FixedEpoch, UpdatedAt: FixedEpoch, GUID: &relationTypeGUID, Name: "benchmark-related"},
	}
	for _, row := range lookups {
		if err := ctx.Err(); err != nil {
			return err
		}
		checksumModel(checksum, fmt.Sprintf("lookup:%T", row), row)
		if err := db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Create(row).Error; err != nil {
			return fmt.Errorf("seed lookup %T: %w", row, err)
		}
	}
	checksumWrite(checksum, "lookups", len(lookups))
	return nil
}

func seedGroups(ctx context.Context, db *gorm.DB, options PrepareOptions, checksum hash.Hash) error {
	return inBatches(options.Profile.Groups, options.BatchSize, func(start, end int) error {
		rows := make([]models.Group, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			name := fmt.Sprintf("benchmark-group-%06d", id)
			var owner *uint
			switch {
			case id == 1 || id == 2 || id == 3:
			case id <= uint(options.Profile.Groups/2):
				parent := uint(1)
				owner = &parent
			default:
				deepStart := uint(options.Profile.Groups/2 + 1)
				depth := (id - deepStart) % 32
				parent := id - 1
				if depth == 0 {
					parent = 2
				}
				owner = &parent
			}
			if id == uint(options.Profile.Groups) {
				name = "benchmark-leaf"
			}
			guid := deterministicGUID("group", id, options.Profile.Seed)
			categoryID := uint(1)
			rows = append(rows, models.Group{ID: id, CreatedAt: FixedEpoch.Add(time.Duration(id) * time.Second), UpdatedAt: FixedEpoch.Add(time.Duration(id) * time.Second), GUID: &guid, Name: name, Description: "benchmark hierarchy", Meta: modeltypes.JSON(`{"tier":1}`), OwnerId: owner, CategoryId: &categoryID})
			checksumModel(checksum, "group", rows[len(rows)-1])
		}
		return createBatch(db, rows)
	})
}

func seedTags(ctx context.Context, db *gorm.DB, options PrepareOptions, checksum hash.Hash) error {
	return inBatches(options.Profile.Tags, options.BatchSize, func(start, end int) error {
		rows := make([]models.Tag, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			name := fmt.Sprintf("benchmark-tag-%03d", id)
			if id == 1 {
				name = "common"
			}
			if id == 2 {
				name = "rare"
			}
			guid := deterministicGUID("tag", id, options.Profile.Seed)
			rows = append(rows, models.Tag{ID: id, CreatedAt: FixedEpoch, UpdatedAt: FixedEpoch, GUID: &guid, Name: name, Description: "benchmark tag", Meta: modeltypes.JSON(`{"kind":"benchmark"}`)})
			checksumModel(checksum, "tag", rows[len(rows)-1])
		}
		return createBatch(db, rows)
	})
}

var contentTypes = []string{"image/jpeg", "image/png", "video/mp4", "application/pdf", "text/plain", "audio/mpeg", "application/zip", "application/json"}

func seedResources(ctx context.Context, db *gorm.DB, options PrepareOptions, checksum hash.Hash) error {
	return inBatches(options.Profile.Resources, options.BatchSize, func(start, end int) error {
		rows := make([]models.Resource, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			owner := resourceOwner(id, options.Profile.Groups)
			guid := deterministicGUID("resource", id, options.Profile.Seed)
			name := fmt.Sprintf("benchmark-resource-%09d", id)
			selector := seeded(id, options.Profile.Seed)
			description := "deterministic fixture content"
			if selector%5 == 0 {
				description += " benchmarkneedle"
			}
			meta := modeltypes.JSON(fmt.Sprintf(`{"rating":%d,"region":"r%d","active":%t}`, selector%5, selector%7, selector%2 == 0))
			fileSize := int64(selector%1_000_000 + 1)
			if selector%100 == 0 {
				fileSize = 50_000_000 + int64(selector%1_000_000)
			}
			createdAt := FixedEpoch.Add(time.Duration(id) * time.Second)
			contentType := contentTypes[int(selector%uint64(len(contentTypes)))]
			width, height := uint(selector%4096+1), uint(selector%2160+1)
			rows = append(rows, models.Resource{
				ID: id, CreatedAt: createdAt, UpdatedAt: createdAt, GUID: &guid,
				Name: name, OriginalName: name, OriginalLocation: "/benchmark", Location: fmt.Sprintf("benchmark/%d", id), Description: description,
				Meta: meta, OwnMeta: modeltypes.JSON(`{"source":"benchmark"}`), Width: width, Height: height,
				FileSize: fileSize, ContentType: contentType, ContentCategory: "benchmark",
				ResourceCategoryId: 1, OwnerId: &owner,
			})
			checksumModel(checksum, "resource", rows[len(rows)-1])
		}
		return createBatch(db, rows)
	})
}

func seedNotes(ctx context.Context, db *gorm.DB, options PrepareOptions, checksum hash.Hash) error {
	return inBatches(options.Profile.Notes, options.BatchSize, func(start, end int) error {
		rows := make([]models.Note, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			selector := seeded(id, options.Profile.Seed)
			owner := resourceOwner(uint(selector*3), options.Profile.Groups)
			guid := deterministicGUID("note", id, options.Profile.Seed)
			noteType := uint(1)
			createdAt := FixedEpoch.Add(time.Duration(id) * time.Minute)
			meta := modeltypes.JSON(fmt.Sprintf(`{"rating":%d}`, selector%5))
			name := fmt.Sprintf("benchmark-note-%09d", id)
			rows = append(rows, models.Note{ID: id, CreatedAt: createdAt, UpdatedAt: createdAt, GUID: &guid, Name: name, Description: "benchmark note benchmarkneedle", Meta: meta, OwnerId: &owner, NoteTypeId: &noteType})
			checksumModel(checksum, "note", rows[len(rows)-1])
		}
		return createBatch(db, rows)
	})
}

func seedRelations(ctx context.Context, db *gorm.DB, options PrepareOptions, checksum hash.Hash) error {
	if err := inBatches(options.Profile.Resources, options.BatchSize, func(start, end int) error {
		resourceTags := make([]resourceTag, 0, (end-start)*2)
		groupResources := make([]groupResource, 0, relationBatchCapacity(start, end))
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			selector := seeded(id, options.Profile.Seed)
			if selector%4 != 0 {
				edge := resourceTag{ResourceID: id, TagID: 1}
				resourceTags = append(resourceTags, edge)
				checksumWrite(checksum, "resource-tag", edge.ResourceID, edge.TagID)
			}
			if selector%10 == 0 {
				edge := resourceTag{ResourceID: id, TagID: uint(selector%uint64(options.Profile.Tags-1) + 2)}
				resourceTags = append(resourceTags, edge)
				checksumWrite(checksum, "resource-tag", edge.ResourceID, edge.TagID)
			}
			if selector%2 == 0 {
				edge := groupResource{GroupID: resourceOwner(uint(selector+1), options.Profile.Groups), ResourceID: id}
				groupResources = append(groupResources, edge)
				checksumWrite(checksum, "group-resource", edge.GroupID, edge.ResourceID)
			}
		}
		if err := createBatch(db, resourceTags); err != nil {
			return err
		}
		return createBatch(db, groupResources)
	}); err != nil {
		return err
	}

	if err := inBatches(options.Profile.Notes, options.BatchSize, func(start, end int) error {
		noteTags := make([]noteTag, 0, end-start)
		groupNotes := make([]groupNote, 0, end-start)
		resourceNotes := make([]resourceNote, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			selector := seeded(id, options.Profile.Seed)
			if selector%3 != 0 {
				edge := noteTag{NoteID: id, TagID: 1}
				noteTags = append(noteTags, edge)
				checksumWrite(checksum, "note-tag", edge.NoteID, edge.TagID)
			}
			groupEdge := groupNote{GroupID: resourceOwner(uint(selector), options.Profile.Groups), NoteID: id}
			resourceEdge := resourceNote{ResourceID: uint((selector-1)%uint64(options.Profile.Resources) + 1), NoteID: id}
			groupNotes = append(groupNotes, groupEdge)
			resourceNotes = append(resourceNotes, resourceEdge)
			checksumWrite(checksum, "group-note", groupEdge.GroupID, groupEdge.NoteID)
			checksumWrite(checksum, "resource-note", resourceEdge.ResourceID, resourceEdge.NoteID)
		}
		if err := createBatch(db, noteTags); err != nil {
			return err
		}
		if err := createBatch(db, groupNotes); err != nil {
			return err
		}
		return createBatch(db, resourceNotes)
	}); err != nil {
		return err
	}

	groupTags := make([]groupTag, 0, options.Profile.Groups)
	for id := 1; id <= options.Profile.Groups; id++ {
		edge := groupTag{GroupID: uint(id), TagID: uint((uint64(id)+uint64(options.Profile.Seed))%uint64(options.Profile.Tags) + 1)}
		groupTags = append(groupTags, edge)
		checksumWrite(checksum, "group-tag", edge.GroupID, edge.TagID)
	}
	if err := createBatch(db, groupTags); err != nil {
		return err
	}
	return nil
}

func seedSimilarity(ctx context.Context, db *gorm.DB, options PrepareOptions, checksum hash.Hash) error {
	pairs := options.Profile.Resources - 1
	if pairs > 100_000 {
		pairs = 100_000
	}
	if pairs < 0 {
		pairs = 0
	}
	if err := inBatches(pairs, options.BatchSize, func(start, end int) error {
		rows := make([]models.ResourceSimilarity, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			selector := uint64(index+1) + uint64(options.Profile.Seed)
			distance := uint8(selector % 16)
			pDistance, aDistance := distance, uint8(selector%8)
			row := models.ResourceSimilarity{ID: uint(index + 1), ResourceID1: 1, ResourceID2: uint(index + 2), HammingDistance: distance, PDistance: &pDistance, ADistance: &aDistance}
			rows = append(rows, row)
			checksumWrite(checksum, "similarity", row.ID, row.ResourceID1, row.ResourceID2, distance, pDistance, aDistance)
		}
		return createBatch(db, rows)
	}); err != nil {
		return err
	}

	hashes := options.Profile.Resources
	if hashes > 100_000 {
		hashes = 100_000
	}
	if err := inBatches(hashes, options.BatchSize, func(start, end int) error {
		rows := make([]models.ImageHash, 0, end-start)
		for index := start; index < end; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			id := uint(index + 1)
			resourceID := id
			selector := int64(index) + options.Profile.Seed
			dhash := selector / 2
			ahash := selector % 1024
			version := 2
			phash := selector
			row := models.ImageHash{ID: id, ResourceId: &resourceID, DHashInt: &dhash, AHashInt: &ahash, HashVersion: &version, PHashInt: &phash, Status: models.HashStatusOK}
			rows = append(rows, row)
			checksumWrite(checksum, "image-hash", id, resourceID, dhash, ahash, version, phash, row.Status)
		}
		return createBatch(db, rows)
	}); err != nil {
		return err
	}
	return nil
}

func createBatch[T any](db *gorm.DB, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Create(&rows).Error; err != nil {
		return err
	}
	return nil
}

func relationBatchCapacity(start, end int) int { return max(0, (end-start)/2) }

func inBatches(total, batchSize int, fn func(start, end int) error) error {
	for start := 0; start < total; start += batchSize {
		end := min(start+batchSize, total)
		if err := fn(start, end); err != nil {
			return err
		}
	}
	return nil
}

func seeded(id uint, seed int64) uint64 { return uint64(id) + uint64(seed) }

func resourceOwner(id uint, groupCount int) uint {
	if id%3 == 0 {
		return 1
	}
	deepStart := uint(groupCount/2 + 1)
	deepWidth := uint(max(1, groupCount-groupCount/2))
	return deepStart + id%deepWidth
}

func ownerValue(owner *uint) uint {
	if owner == nil {
		return 0
	}
	return *owner
}
func deterministicGUID(kind string, id uint, seed int64) string {
	sum := sha256.Sum256([]byte(kind + ":" + strconv.FormatUint(uint64(id), 10) + ":" + strconv.FormatInt(seed, 10)))
	hexValue := hex.EncodeToString(sum[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:32])
}
func checksumModel(h hash.Hash, label string, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal deterministic fixture checksum %s: %v", label, err))
	}
	checksumWrite(h, label, string(encoded))
}

func checksumWrite(h hash.Hash, values ...any) {
	for _, value := range values {
		_, _ = fmt.Fprintf(h, "%T:%v|", value, value)
	}
	_, _ = h.Write([]byte("\n"))
}

func analyzeFixture(db *gorm.DB) error {
	if err := db.Exec("ANALYZE").Error; err != nil {
		return fmt.Errorf("analyze fixture: %w", err)
	}
	return nil
}

func fixtureCountModels() map[string]any {
	return map[string]any{
		"resources": &models.Resource{}, "notes": &models.Note{}, "groups": &models.Group{}, "tags": &models.Tag{},
		"resourceSimilarities": &models.ResourceSimilarity{}, "imageHashes": &models.ImageHash{},
		"resourceTags": &resourceTag{}, "noteTags": &noteTag{}, "groupTags": &groupTag{},
		"groupResources": &groupResource{}, "groupNotes": &groupNote{}, "resourceNotes": &resourceNote{},
	}
}

var fixtureGuardTables = []string{
	"resources", "notes", "groups", "tags", "resource_similarities", "image_hashes",
	"resource_tags", "note_tags", "group_tags", "groups_related_resources", "groups_related_notes", "resource_notes",
	"resource_categories", "note_types", "categories", "queries", "group_relation_types",
}

func installFixtureMutationGuards(db *gorm.DB) error {
	if db.Dialector.Name() == "postgres" {
		if err := db.Exec(`CREATE OR REPLACE FUNCTION mrql_benchmark_mark_dirty() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE mrql_benchmark_marker SET dirty = TRUE WHERE id = 1;
  RETURN NULL;
END $$`).Error; err != nil {
			return fmt.Errorf("create fixture mutation guard function: %w", err)
		}
		for _, table := range fixtureGuardTables {
			trigger := "mrql_benchmark_dirty_" + table
			if err := db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s", trigger, table)).Error; err != nil {
				return fmt.Errorf("drop fixture mutation guard for %s: %w", table, err)
			}
			if err := db.Exec(fmt.Sprintf("CREATE TRIGGER %s AFTER INSERT OR UPDATE OR DELETE ON %s FOR EACH STATEMENT EXECUTE FUNCTION mrql_benchmark_mark_dirty()", trigger, table)).Error; err != nil {
				return fmt.Errorf("create fixture mutation guard for %s: %w", table, err)
			}
		}
		return nil
	}
	for _, table := range fixtureGuardTables {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			trigger := fmt.Sprintf("mrql_benchmark_dirty_%s_%s", table, strings.ToLower(operation))
			statement := fmt.Sprintf("CREATE TRIGGER %s AFTER %s ON %s BEGIN UPDATE mrql_benchmark_marker SET dirty = 1 WHERE id = 1; END", trigger, operation, table)
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("create fixture mutation guard for %s %s: %w", table, operation, err)
			}
		}
	}
	return nil
}

func collectFixtureCounts(db *gorm.DB, counts map[string]int64) error {
	for key, model := range fixtureCountModels() {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			return fmt.Errorf("count %s: %w", key, err)
		}
		counts[key] = count
	}
	return nil
}

func collectFTSChecks(db *gorm.DB, term string) (map[string]int64, string, error) {
	if term == "" {
		return nil, "", errors.New("fixture FTS term is missing")
	}
	query := "SELECT rowid FROM resources_fts WHERE resources_fts MATCH ? ORDER BY rowid"
	if db.Dialector.Name() == "postgres" {
		query = "SELECT id FROM resources WHERE search_vector @@ plainto_tsquery('english', ?) ORDER BY id"
	}
	rows, err := db.Raw(query, term).Rows()
	if err != nil {
		return nil, "", fmt.Errorf("query resource FTS membership: %w", err)
	}
	digest := sha256.New()
	var matches int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, "", fmt.Errorf("scan resource FTS membership: %w", err)
		}
		checksumWrite(digest, id)
		matches++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, "", fmt.Errorf("read resource FTS membership: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, "", fmt.Errorf("close resource FTS membership: %w", err)
	}
	checks := map[string]int64{"resourceMatches": matches}
	if db.Dialector.Name() == "postgres" {
		var indexes int64
		if err := db.Raw("SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_resources_fts'").Scan(&indexes).Error; err != nil {
			return nil, "", fmt.Errorf("validate PostgreSQL resource FTS index: %w", err)
		}
		checks["resourceIndexes"] = indexes
	} else {
		var tables int64
		if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'resources_fts'").Scan(&tables).Error; err != nil {
			return nil, "", fmt.Errorf("validate SQLite resource FTS table: %w", err)
		}
		checks["resourceTables"] = tables
	}
	return checks, hex.EncodeToString(digest.Sum(nil)), nil
}

func databaseVersion(db *gorm.DB) (string, error) {
	var version string
	query := "SELECT sqlite_version()"
	if db.Dialector.Name() == "postgres" {
		query = "SHOW server_version"
	}
	if err := db.Raw(query).Scan(&version).Error; err != nil {
		return "", fmt.Errorf("database version: %w", err)
	}
	return version, nil
}

func normalizeDialect(dialect string) string {
	if dialect == constants.DbTypePosgres || dialect == "postgres" || dialect == "POSTGRES" {
		return "postgres"
	}
	return "sqlite"
}

package application_context

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// createTestContext creates a self-sufficient test context with in-memory database
// and filesystem, without requiring any .env files
func createTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Run migrations
	err = db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.LogEntry{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Try to find ffmpeg in PATH
	ffmpegPath, _ := exec.LookPath("ffmpeg")

	config := &MahresourcesConfig{
		DbType:     constants.DbTypeSqlite,
		FfmpegPath: ffmpegPath,
	}

	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")

	return NewMahresourcesContext(fs, db, readOnlyDB, config)
}

func getMeTheFileOrPanic(path string) io.ReadSeeker {
	file, err := os.Open(path)

	if err != nil {
		// no file... panic!!!
		panic(err)
	}

	// got the file!!! DO NOT PANIC.
	return file
}

func TestMahresourcesContext_createThumbFromVideo(t *testing.T) {
	ctx := createTestContext(t)

	if ctx.Config.FfmpegPath == "" {
		t.Skip("ffmpeg not found in PATH, skipping video thumbnail test")
	}

	if err := ctx.createThumbFromVideo(context.TODO(), getMeTheFileOrPanic("../test_data/pexels-thirdman-5862328.mp4"), bytes.NewBuffer(make([]byte, 0))); err != nil {
		t.Errorf("createThumbFromVideo() error = %v", err)
	}
}

// bytesFile wraps a bytes.Reader to implement interfaces.File (io.Reader + io.Closer)
type bytesFile struct {
	*bytes.Reader
}

func (b *bytesFile) Close() error {
	return nil
}

func newBytesFile(data []byte) *bytesFile {
	return &bytesFile{Reader: bytes.NewReader(data)}
}

func TestAddResource_ConcurrentSameHash(t *testing.T) {
	// Create an in-memory SQLite database for this test
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Run migrations
	err = db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.LogEntry{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Create config and context
	config := &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")

	testCtx := NewMahresourcesContext(fs, db, readOnlyDB, config)
	if testCtx == nil {
		t.Fatal("Failed to create test context")
	}

	// Create a group to serve as owner for the resources
	ownerGroup := &models.Group{Name: "test-owner", Description: "test owner group"}
	if err := db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("Failed to create owner group: %v", err)
	}
	ownerGroupID := ownerGroup.ID

	// The file content - all goroutines will try to upload the same content
	fileContent := []byte("this is the same file content for all concurrent uploads")

	concurrency := 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	successCount := 0
	errorCount := 0
	var countMu sync.Mutex

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()

			file := newBytesFile(fileContent)
			creator := &query_models.ResourceCreator{
				ResourceQueryBase: query_models.ResourceQueryBase{
					Name:    "test-file.txt",
					OwnerId: ownerGroupID,
				},
			}

			_, err := testCtx.AddResource(file, "test-file.txt", creator)

			countMu.Lock()
			defer countMu.Unlock()

			if err == nil {
				successCount++
			} else {
				// Expected error: "existing resource (X) with same parent"
				if strings.Contains(err.Error(), "existing resource") {
					errorCount++
				} else {
					t.Errorf("Unexpected error from goroutine %d: %v", idx, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Exactly one goroutine should succeed, all others should get "existing resource" error
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful upload, got %d", successCount)
	}
	if errorCount != concurrency-1 {
		t.Errorf("Expected %d 'existing resource' errors, got %d", concurrency-1, errorCount)
	}

	// Verify only one resource exists in the database
	var resourceCount int64
	if err := db.Table("resources").Count(&resourceCount).Error; err != nil {
		t.Fatalf("Failed to count resources: %v", err)
	}

	if resourceCount != 1 {
		t.Errorf("Expected exactly 1 resource in database, got %d", resourceCount)
	}
}

// slowReader is a test helper that delays between reads
type slowReader struct {
	data    []byte
	pos     int
	delay   time.Duration
	readLen int
}

func (s *slowReader) Read(p []byte) (n int, err error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	time.Sleep(s.delay)
	n = copy(p, s.data[s.pos:min(s.pos+s.readLen, len(s.data))])
	s.pos += n
	return n, nil
}

func TestTimeoutReader_NormalRead(t *testing.T) {
	data := []byte("hello world")
	reader := bytes.NewReader(data)

	tr := newTimeoutReader(reader, 5*time.Second)
	defer tr.Close()

	result, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(data) {
		t.Errorf("expected %q, got %q", data, result)
	}
}

func TestTimeoutReader_IdleTimeout(t *testing.T) {
	// Reader that blocks forever
	r, _ := io.Pipe()
	tr := newTimeoutReader(r, 200*time.Millisecond)
	defer tr.Close()

	buf := make([]byte, 10)
	_, err := tr.Read(buf)
	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Errorf("expected idle timeout error, got: %v", err)
	}
}

func TestTimeoutReader_SlowButActiveRead(t *testing.T) {
	// Reader that sends data slowly but consistently (faster than timeout)
	data := []byte("abcdefghij")
	slow := &slowReader{
		data:    data,
		delay:   20 * time.Millisecond,
		readLen: 2,
	}

	tr := newTimeoutReader(slow, 100*time.Millisecond)
	defer tr.Close()

	result, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(data) {
		t.Errorf("expected %q, got %q", data, result)
	}
}

func TestTimeoutReader_CloseStopsWatcher(t *testing.T) {
	reader := bytes.NewReader([]byte("test"))
	tr := newTimeoutReader(reader, time.Hour)

	// Close should not panic and should stop the goroutine
	err := tr.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Give goroutine time to exit
	time.Sleep(10 * time.Millisecond)
}


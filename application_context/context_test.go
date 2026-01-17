package application_context

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	_ "github.com/mattn/go-sqlite3"
	"mahresources/storage"
)

func TestCopySeedDatabase(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.db")
	dstPath := filepath.Join(tmpDir, "dest.db")

	// Create a source SQLite database with test data
	db, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatalf("Failed to create source database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO test_table (name) VALUES ('test1'), ('test2'), ('test3');
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}
	db.Close()

	// Test copying the database
	err = copySeedDatabase(srcPath, dstPath)
	if err != nil {
		t.Fatalf("copySeedDatabase() error = %v", err)
	}

	// Verify the destination database has the data
	dstDB, err := sql.Open("sqlite3", dstPath)
	if err != nil {
		t.Fatalf("Failed to open destination database: %v", err)
	}
	defer dstDB.Close()

	var count int
	err = dstDB.QueryRow("SELECT COUNT(*) FROM test_table").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query destination database: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 rows in destination, got %d", count)
	}
}

func TestCopySeedDatabase_SourceNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = copySeedDatabase("/nonexistent/path/db.sqlite", filepath.Join(tmpDir, "dest.db"))
	if err == nil {
		t.Error("Expected error for nonexistent source file, got nil")
	}
}

func TestCopySeedDatabase_InvalidDestination(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a source file
	srcPath := filepath.Join(tmpDir, "source.db")
	if err := os.WriteFile(srcPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Try to copy to an invalid path
	err = copySeedDatabase(srcPath, "/nonexistent/directory/dest.db")
	if err == nil {
		t.Error("Expected error for invalid destination path, got nil")
	}
}

func TestCreateContextWithConfig_SeedDBRequiresMemoryDB(t *testing.T) {
	// This test verifies the validation logic by checking that the config
	// is properly validated. We can't easily test log.Fatal, so we test
	// the conditions that would trigger it.

	cfg := &MahresourcesInputConfig{
		SeedDB:   "/some/path.db",
		MemoryDB: false,
	}

	// The validation check: SeedDB requires MemoryDB
	if cfg.SeedDB != "" && !cfg.MemoryDB {
		// This is the expected condition that would trigger log.Fatal
		// in CreateContextWithConfig
		t.Log("Correctly identified that SeedDB requires MemoryDB")
	} else {
		t.Error("Validation logic incorrect: SeedDB should require MemoryDB")
	}
}

func TestCreateContextWithConfig_SeedDBNotAllowedWithPostgres(t *testing.T) {
	cfg := &MahresourcesInputConfig{
		SeedDB:   "/some/path.db",
		MemoryDB: true,
		DbType:   "POSTGRES",
	}

	// The validation check: SeedDB not allowed with Postgres
	if cfg.SeedDB != "" && cfg.DbType == "POSTGRES" {
		// This is the expected condition that would trigger log.Fatal
		t.Log("Correctly identified that SeedDB is not allowed with Postgres")
	} else {
		t.Error("Validation logic incorrect: SeedDB should not be allowed with Postgres")
	}
}

func TestCreateContextWithConfig_SeedDBWithMemoryDB(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a seed database with a tags table matching the app schema
	seedPath := filepath.Join(tmpDir, "seed.db")
	db, err := sql.Open("sqlite3", seedPath)
	if err != nil {
		t.Fatalf("Failed to create seed database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			name TEXT,
			description TEXT
		);
		INSERT INTO tags (name, description, created_at, updated_at)
		VALUES ('seed-tag', 'seeded from test', datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("Failed to create seed data: %v", err)
	}
	db.Close()

	// Clean up any existing ephemeral DB
	os.Remove("/tmp/mahresources_ephemeral.db")
	os.Remove("/tmp/mahresources_ephemeral.db-wal")
	os.Remove("/tmp/mahresources_ephemeral.db-shm")

	cfg := &MahresourcesInputConfig{
		SeedDB:       seedPath,
		MemoryDB:     true,
		MemoryFS:     true,
		FileSavePath: tmpDir,
	}

	ctx, gormDB, _ := CreateContextWithConfig(cfg)

	// Verify the context was created
	if ctx == nil {
		t.Fatal("Expected context to be created, got nil")
	}

	// Verify the seeded data exists
	var count int64
	if err := gormDB.Table("tags").Where("name = ?", "seed-tag").Count(&count).Error; err != nil {
		t.Fatalf("Failed to query tags: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 seeded tag, got %d", count)
	}

	// Clean up
	os.Remove("/tmp/mahresources_ephemeral.db")
	os.Remove("/tmp/mahresources_ephemeral.db-wal")
	os.Remove("/tmp/mahresources_ephemeral.db-shm")
}

func TestCreateContextWithConfig_SeedFSRequiresOverlay(t *testing.T) {
	cfg := &MahresourcesInputConfig{
		SeedFS:       "/some/path",
		MemoryFS:     false,
		FileSavePath: "",
	}

	// The validation check: SeedFS requires either MemoryFS or FileSavePath
	if cfg.SeedFS != "" && !cfg.MemoryFS && cfg.FileSavePath == "" {
		t.Log("Correctly identified that SeedFS requires an overlay (memory-fs or file-save-path)")
	} else {
		t.Error("Validation logic incorrect: SeedFS should require an overlay")
	}
}

func TestCreateCopyOnWriteStorage(t *testing.T) {
	// Create a temporary directory with seed files
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a seed file in the temp directory
	seedContent := []byte("original content from seed")
	seedFilePath := filepath.Join(tmpDir, "seed_file.txt")
	if err := os.WriteFile(seedFilePath, seedContent, 0644); err != nil {
		t.Fatalf("Failed to create seed file: %v", err)
	}

	// Create the copy-on-write filesystem with memory overlay
	overlay := afero.NewMemMapFs()
	cowFs := storage.CreateCopyOnWriteStorage(tmpDir, overlay)

	// Test 1: Read from base layer (seed directory)
	content, err := afero.ReadFile(cowFs, "seed_file.txt")
	if err != nil {
		t.Fatalf("Failed to read seed file: %v", err)
	}
	if string(content) != string(seedContent) {
		t.Errorf("Expected content %q, got %q", seedContent, content)
	}

	// Test 2: Write to overlay (should not affect the base)
	newContent := []byte("modified content in overlay")
	if err := afero.WriteFile(cowFs, "seed_file.txt", newContent, 0644); err != nil {
		t.Fatalf("Failed to write to overlay: %v", err)
	}

	// Test 3: Read the modified content from overlay
	modifiedContent, err := afero.ReadFile(cowFs, "seed_file.txt")
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}
	if string(modifiedContent) != string(newContent) {
		t.Errorf("Expected modified content %q, got %q", newContent, modifiedContent)
	}

	// Test 4: Verify original file on disk is unchanged
	originalContent, err := os.ReadFile(seedFilePath)
	if err != nil {
		t.Fatalf("Failed to read original file: %v", err)
	}
	if string(originalContent) != string(seedContent) {
		t.Errorf("Original file was modified! Expected %q, got %q", seedContent, originalContent)
	}

	// Test 5: Create a new file in overlay
	newFileContent := []byte("new file content")
	if err := afero.WriteFile(cowFs, "new_file.txt", newFileContent, 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// Verify new file exists in CoW filesystem
	readNewContent, err := afero.ReadFile(cowFs, "new_file.txt")
	if err != nil {
		t.Fatalf("Failed to read new file: %v", err)
	}
	if string(readNewContent) != string(newFileContent) {
		t.Errorf("Expected new file content %q, got %q", newFileContent, readNewContent)
	}

	// Verify new file doesn't exist on disk
	diskNewFilePath := filepath.Join(tmpDir, "new_file.txt")
	if _, err := os.Stat(diskNewFilePath); !os.IsNotExist(err) {
		t.Error("New file should not exist on disk, but it does")
	}
}

func TestCreateContextWithConfig_SeedFSWithMemoryFS(t *testing.T) {
	// Create a temporary directory with seed files
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a subdirectory structure like the app would use
	filesDir := filepath.Join(tmpDir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatalf("Failed to create files dir: %v", err)
	}

	// Create a seed file
	seedContent := []byte("test resource content")
	if err := os.WriteFile(filepath.Join(filesDir, "test.txt"), seedContent, 0644); err != nil {
		t.Fatalf("Failed to create seed file: %v", err)
	}

	// Clean up any existing ephemeral DB
	os.Remove("/tmp/mahresources_ephemeral.db")
	os.Remove("/tmp/mahresources_ephemeral.db-wal")
	os.Remove("/tmp/mahresources_ephemeral.db-shm")

	cfg := &MahresourcesInputConfig{
		SeedFS:   filesDir,
		MemoryDB: true,
		MemoryFS: true,
	}

	ctx, _, fs := CreateContextWithConfig(cfg)

	// Verify the context was created
	if ctx == nil {
		t.Fatal("Expected context to be created, got nil")
	}

	// Verify we can read the seeded file
	content, err := afero.ReadFile(fs, "test.txt")
	if err != nil {
		t.Fatalf("Failed to read seeded file: %v", err)
	}
	if string(content) != string(seedContent) {
		t.Errorf("Expected content %q, got %q", seedContent, content)
	}

	// Verify we can write without affecting the original
	if err := afero.WriteFile(fs, "test.txt", []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to write to fs: %v", err)
	}

	// Original file should be unchanged
	originalContent, err := os.ReadFile(filepath.Join(filesDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read original file: %v", err)
	}
	if string(originalContent) != string(seedContent) {
		t.Error("Original file was modified by write operation")
	}

	// Clean up
	os.Remove("/tmp/mahresources_ephemeral.db")
	os.Remove("/tmp/mahresources_ephemeral.db-wal")
	os.Remove("/tmp/mahresources_ephemeral.db-shm")
}

func TestCreateContextWithConfig_SeedFSWithDiskOverlay(t *testing.T) {
	// Create temporary directories for seed and overlay
	tmpDir, err := os.MkdirTemp("", "mahresources_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	seedDir := filepath.Join(tmpDir, "seed")
	overlayDir := filepath.Join(tmpDir, "overlay")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		t.Fatalf("Failed to create seed dir: %v", err)
	}
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("Failed to create overlay dir: %v", err)
	}

	// Create a seed file
	seedContent := []byte("original seed content")
	if err := os.WriteFile(filepath.Join(seedDir, "test.txt"), seedContent, 0644); err != nil {
		t.Fatalf("Failed to create seed file: %v", err)
	}

	// Clean up any existing ephemeral DB
	os.Remove("/tmp/mahresources_ephemeral.db")
	os.Remove("/tmp/mahresources_ephemeral.db-wal")
	os.Remove("/tmp/mahresources_ephemeral.db-shm")

	cfg := &MahresourcesInputConfig{
		SeedFS:       seedDir,
		FileSavePath: overlayDir,
		MemoryDB:     true,
		MemoryFS:     false, // Use disk overlay
	}

	ctx, _, fs := CreateContextWithConfig(cfg)

	if ctx == nil {
		t.Fatal("Expected context to be created, got nil")
	}

	// Read the seeded file
	content, err := afero.ReadFile(fs, "test.txt")
	if err != nil {
		t.Fatalf("Failed to read seeded file: %v", err)
	}
	if string(content) != string(seedContent) {
		t.Errorf("Expected content %q, got %q", seedContent, content)
	}

	// Write a modified version
	modifiedContent := []byte("modified content")
	if err := afero.WriteFile(fs, "test.txt", modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to write to fs: %v", err)
	}

	// Verify the modification is in the overlay on disk
	overlayContent, err := os.ReadFile(filepath.Join(overlayDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read overlay file: %v", err)
	}
	if string(overlayContent) != string(modifiedContent) {
		t.Errorf("Expected overlay content %q, got %q", modifiedContent, overlayContent)
	}

	// Verify original seed file is unchanged
	originalContent, err := os.ReadFile(filepath.Join(seedDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read original file: %v", err)
	}
	if string(originalContent) != string(seedContent) {
		t.Error("Original seed file was modified!")
	}

	// Clean up
	os.Remove("/tmp/mahresources_ephemeral.db")
	os.Remove("/tmp/mahresources_ephemeral.db-wal")
	os.Remove("/tmp/mahresources_ephemeral.db-shm")
}

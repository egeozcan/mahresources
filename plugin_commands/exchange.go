package plugin_commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	MaxListEntries   = 10_000
	MaxReadBytes     = 4 << 20
	MaxFileNameBytes = 255
)

var (
	ErrExchangeUnsupported      = errors.New("plugin command exchange folders are unsupported on this platform")
	ErrExchangeRunNotFound      = errors.New("run not found")
	ErrExchangeFileNotFound     = errors.New("file not found")
	ErrExchangeRunSwept         = errors.New("run swept")
	ErrExchangeOutputUnverified = errors.New("output unverified")
	ErrExchangeRunNotFinished   = errors.New("run not finished")
	ErrExchangeFileNotRegular   = errors.New("file is not a regular file")
	errExchangePathChanged      = errors.New("exchange path changed during operation")
)

type Entry struct {
	Name     string
	Size     int64
	Modified time.Time
}

type Listing struct {
	Entries   []Entry
	Truncated bool
}

type Exchange interface {
	List(Access, string) (Listing, error)
	Read(Access, string, string, int64) ([]byte, error)
	Discard(Access, string, string) error
	DiscardRun(Access, string) error
}

type exchangeService struct {
	store           Store
	settings        Settings
	leases          *LeaseManager
	afterLstat      func(string) // test barrier; nil in production
	afterOpenRunDir func()       // test barrier; nil in production
	beforeRemoveRun func()       // test barrier; nil in production
}

func NewExchange(store Store, settings Settings) Exchange {
	return NewExchangeWithLeases(store, settings, NewLeaseManager())
}

// NewExchangeWithLeases lets imports and the retention sweep share the same
// per-run coordination object as plugin-visible file operations.
func NewExchangeWithLeases(store Store, settings Settings, leases *LeaseManager) Exchange {
	if leases == nil {
		leases = NewLeaseManager()
	}
	return &exchangeService{store: store, settings: settings, leases: leases}
}

// HasRegularFile checks one admitted file using only the configured filesystem
// root. It shares the exchange lease with List, Read, imports and sweeping, and
// opens every path component and the file without following symlinks. Callers
// must establish the run and file identity through their own durable records.
func (e *exchangeService) HasRegularFile(pluginName, runID, name string) bool {
	if e == nil || e.settings == nil || e.leases == nil || exchangePlatformSupported() != nil {
		return false
	}
	if !validExchangeComponent(pluginName) || !validExchangeComponent(runID) || validateExchangeName(name) != nil {
		return false
	}
	release, err := e.leases.Acquire(runID)
	if err != nil {
		return false
	}
	defer release()

	dir, err := openExchangeRunDir(e.settings.StagingRoot(), pluginName, runID)
	if err != nil {
		return false
	}
	defer dir.Close()
	_, err = statExchangeRegularAt(dir, name)
	return err == nil
}

func (e *exchangeService) List(access Access, runID string) (Listing, error) {
	if err := exchangePlatformSupported(); err != nil {
		return Listing{}, err
	}
	run, release, err := e.authorizeAndLease(access, runID, false)
	if err != nil {
		return Listing{}, err
	}
	defer release()

	dir, err := openExchangeRunDir(e.settings.StagingRoot(), run.PluginName, run.ID)
	if err != nil {
		return Listing{}, classifyRunDirError(err)
	}
	defer dir.Close()
	if e.afterOpenRunDir != nil {
		e.afterOpenRunDir()
	}

	entries := make([]Entry, 0, min(MaxListEntries, 64))
	for {
		batch, readErr := dir.ReadDir(256)
		for _, item := range batch {
			info, infoErr := statExchangeRegularAt(dir, item.Name())
			if infoErr != nil {
				if errors.Is(infoErr, os.ErrNotExist) {
					continue
				}
				if errors.Is(infoErr, ErrExchangeFileNotRegular) {
					continue
				}
				return Listing{}, fmt.Errorf("list exchange file %q: %w", item.Name(), infoErr)
			}
			if len(entries) == MaxListEntries {
				return Listing{Entries: entries, Truncated: true}, nil
			}
			entries = append(entries, Entry{Name: item.Name(), Size: info.Size(), Modified: info.ModTime()})
		}
		if errors.Is(readErr, io.EOF) {
			return Listing{Entries: entries}, nil
		}
		if readErr != nil {
			return Listing{}, fmt.Errorf("list exchange folder: %w", readErr)
		}
	}
}

func (e *exchangeService) Read(access Access, runID, name string, maxBytes int64) ([]byte, error) {
	if err := exchangePlatformSupported(); err != nil {
		return nil, err
	}
	run, release, err := e.authorizeAndLease(access, runID, false)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := validateExchangeName(name); err != nil {
		return nil, err
	}
	if maxBytes < 1 || maxBytes > MaxReadBytes {
		return nil, fmt.Errorf("max_bytes must be between 1 and %d", MaxReadBytes)
	}

	dir, err := openExchangeRunDir(e.settings.StagingRoot(), run.PluginName, run.ID)
	if err != nil {
		return nil, classifyRunDirError(err)
	}
	defer dir.Close()
	file, err := openExchangeRegularAt(dir, name, e.afterLstat)
	if err != nil {
		return nil, classifyFileError(err)
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read exchange file: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("file exceeds max_bytes")
	}
	return body, nil
}

func (e *exchangeService) Discard(access Access, runID, name string) error {
	if err := exchangePlatformSupported(); err != nil {
		return err
	}
	run, release, err := e.authorizeAndLease(access, runID, false)
	if err != nil {
		return err
	}
	defer release()
	if err := validateExchangeName(name); err != nil {
		return err
	}

	dir, err := openExchangeRunDir(e.settings.StagingRoot(), run.PluginName, run.ID)
	if err != nil {
		return classifyRunDirError(err)
	}
	defer dir.Close()
	if err := unlinkExchangeRegularAt(dir, name, e.afterLstat); err != nil {
		return classifyFileError(err)
	}
	return nil
}

func (e *exchangeService) DiscardRun(access Access, runID string) error {
	if err := exchangePlatformSupported(); err != nil {
		return err
	}
	// Authorization precedes observable lease state. Revalidate after reserving
	// the sweep so actor deletion or a concurrent state change fails closed.
	run, err := e.authorizeRun(access, runID, true)
	if err != nil {
		return err
	}
	endSweep, ok := e.leases.BeginSweep(runID)
	if !ok {
		return fmt.Errorf("active file operation prevents discard_run")
	}
	defer endSweep()
	if run, err = e.authorizeRun(access, runID, true); err != nil {
		return err
	}
	active, err := e.store.HasNonterminalImports(runID)
	if err != nil {
		return fmt.Errorf("check nonterminal imports: %w", err)
	}
	if active {
		return fmt.Errorf("nonterminal import prevents discard_run")
	}

	if err := removeExchangeRunDir(e.settings.StagingRoot(), run.PluginName, run.ID, e.beforeRemoveRun); err != nil {
		return classifyRunDirError(err)
	}
	return nil
}

// authorizeAndLease authorizes before it leases, and authorizes again after, so
// an unauthorized caller never occupies a lease slot and a run that stopped
// being eligible between the two calls cannot be handed back with one held.
func (e *exchangeService) authorizeAndLease(access Access, runID string, allowUnverified bool) (RunRecord, func(), error) {
	if _, err := e.authorizeRun(access, runID, allowUnverified); err != nil {
		return RunRecord{}, nil, err
	}
	release, err := e.leases.Acquire(runID)
	if err != nil {
		return RunRecord{}, nil, err
	}
	run, err := e.authorizeRun(access, runID, allowUnverified)
	if err != nil {
		release()
		return RunRecord{}, nil, err
	}
	return run, release, nil
}

func (e *exchangeService) authorizeRun(access Access, runID string, allowUnverified bool) (RunRecord, error) {
	if e.store == nil || e.settings == nil {
		return RunRecord{}, fmt.Errorf("run not found: exchange service is unavailable")
	}
	run, _, err := e.store.Run(runID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return RunRecord{}, ErrExchangeRunNotFound
		}
		return RunRecord{}, fmt.Errorf("read command run: %w", err)
	}
	if !access.AllowsRun(run) || !validExchangeComponent(run.ID) || !validExchangeComponent(run.PluginName) {
		return RunRecord{}, ErrExchangeRunNotFound
	}
	if !RunStatusTerminal(run.Status) {
		return RunRecord{}, ErrExchangeRunNotFinished
	}
	if run.OutputUnverified && !allowUnverified {
		return RunRecord{}, ErrExchangeOutputUnverified
	}
	rootInfo, rootErr := os.Lstat(e.settings.StagingRoot())
	if rootErr == nil && rootInfo.Mode()&os.ModeSymlink != 0 {
		return RunRecord{}, fmt.Errorf("staging root is a symlink")
	}
	if rootErr != nil && !errors.Is(rootErr, os.ErrNotExist) {
		return RunRecord{}, fmt.Errorf("inspect staging root: %w", rootErr)
	}
	return run, nil
}

func validateExchangeName(name string) error {
	if !validExchangeComponent(name) || len(name) > MaxFileNameBytes {
		return fmt.Errorf("invalid file name")
	}
	return nil
}

func validExchangeComponent(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.ContainsAny(value, "/\\\x00")
}

func classifyRunDirError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return ErrExchangeRunSwept
	}
	if errors.Is(err, ErrExchangeUnsupported) {
		return err
	}
	return fmt.Errorf("open exchange folder: %w", err)
}

func classifyFileError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return ErrExchangeFileNotFound
	case errors.Is(err, ErrExchangeFileNotRegular):
		return ErrExchangeFileNotRegular
	case errors.Is(err, ErrExchangeUnsupported):
		return err
	default:
		return fmt.Errorf("access exchange file: %w", err)
	}
}

func exchangePlatformSupported() error {
	if runtime.GOOS == "windows" {
		return ErrExchangeUnsupported
	}
	return nil
}

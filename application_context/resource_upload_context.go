package application_context

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"mahresources/contracts"
	"mahresources/hash_worker"
	"mahresources/hls"
	"mahresources/hostfetch"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
	"gorm.io/gorm"

	// Register additional image formats for dimension detection
	_ "golang.org/x/image/tiff"
)

// Reason constants for ResourceExistsError.
const (
	ReasonSameParent   = "same parent"
	ReasonSameRelation = "same relation"
)

// ResourceExistsError is returned when an uploaded file's hash matches an
// existing resource.  It carries the duplicate resource's ID so callers
// (e.g. API handlers) can include it in a structured response.
type ResourceExistsError struct {
	ResourceID uint
	Reason     string
}

// Error describes the collision to whoever is reading it. Finding 103: the old
// wording — "existing resource (114) with same parent" — named an internal
// reason constant and left the id as bare text, so the reader could neither tell
// what had happened nor reach the file it collided with. The id stays in the
// sentence because API and CLI callers see only this string; the HTML form
// additionally receives it as ResourceID and renders it as a link.
func (e *ResourceExistsError) Error() string {
	switch e.Reason {
	case ReasonSameRelation:
		return fmt.Sprintf("a resource with identical content is already in that group (#%v)", e.ResourceID)
	default:
		return fmt.Sprintf("a resource with identical content already exists (#%v)", e.ResourceID)
	}
}

// InvalidImageError is returned when an uploaded file is declared as an image
// (content-type starts with "image/") but cannot be decoded or has zero dimensions.
// Callers (e.g. API handlers) should map this to HTTP 400.
type InvalidImageError struct {
	Cause error
}

func (e *InvalidImageError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("uploaded file is not a valid image (failed to decode): %v", e.Cause)
	}
	return "uploaded file is not a valid image (zero dimensions)"
}

func (e *InvalidImageError) Unwrap() error {
	return e.Cause
}

// timeoutReader wraps an io.Reader and returns an error if no data is read within the timeout period
type timeoutReader struct {
	reader      io.Reader
	idleTimeout time.Duration
	done        chan struct{}
	mu          sync.Mutex
	lastRead    time.Time
	err         error
}

func newTimeoutReader(r io.Reader, idleTimeout time.Duration) *timeoutReader {
	tr := &timeoutReader{
		reader:      r,
		idleTimeout: idleTimeout,
		lastRead:    time.Now(),
		done:        make(chan struct{}),
	}
	go tr.watchTimeout()
	return tr
}

func (tr *timeoutReader) watchTimeout() {
	// Check frequently enough to detect timeouts promptly, but not so frequently as to waste CPU
	checkInterval := tr.idleTimeout / 10
	if checkInterval < 100*time.Millisecond {
		checkInterval = 100 * time.Millisecond
	}
	if checkInterval > time.Second {
		checkInterval = time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-tr.done:
			return
		case <-ticker.C:
			tr.mu.Lock()
			elapsed := time.Since(tr.lastRead)
			if elapsed > tr.idleTimeout {
				tr.err = fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
				tr.mu.Unlock()
				return
			}
			tr.mu.Unlock()
		}
	}
}

type readResult struct {
	n   int
	err error
}

func (tr *timeoutReader) Read(p []byte) (n int, err error) {
	// Check for existing error
	tr.mu.Lock()
	if tr.err != nil {
		err := tr.err
		tr.mu.Unlock()
		return 0, err
	}
	tr.mu.Unlock()

	// Run read in goroutine so we can interrupt it on timeout.
	// Note: On timeout, this goroutine may outlive the Read call. It will exit
	// when the underlying reader returns (e.g., when the HTTP connection closes).
	buf := make([]byte, len(p))
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := tr.reader.Read(buf)
		resultCh <- readResult{n, err}
	}()

	// Wait for read to complete or timeout
	for {
		select {
		case result := <-resultCh:
			if result.n > 0 {
				copy(p[:result.n], buf[:result.n])
				tr.mu.Lock()
				tr.lastRead = time.Now()
				tr.mu.Unlock()
			}
			return result.n, result.err
		case <-tr.done:
			return 0, fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
		default:
			tr.mu.Lock()
			err := tr.err
			tr.mu.Unlock()
			if err != nil {
				return 0, err
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (tr *timeoutReader) Close() error {
	select {
	case <-tr.done:
		// already closed
	default:
		close(tr.done)
	}
	return nil
}

// createRemoteResourceHTTPClient creates an HTTP client with the given timeouts
func createRemoteResourceHTTPClient(connectTimeout, overallTimeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: overallTimeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: connectTimeout}).DialContext,
			TLSHandshakeTimeout:   connectTimeout / 2, // TLS handshake gets half the connect timeout
			ResponseHeaderTimeout: connectTimeout,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

// reqCtx bounds the transfer. The plugin path passes its invocation's context,
// so a plugin's create_resource_from_url can no longer hold that plugin's VM
// lock for the full -remote-overall-timeout (30m by default) while every other
// surface of the plugin waits; the HTTP path passes the request's. A context
// with no deadline leaves the previous behaviour exactly as it was.
//
// It bounds the transfer, not the whole creation: what follows a completed
// download (hooks, hashing, the content-hash lock, the writes) is not
// cancellable here and is not meant to be — an after-hook describes a committed
// write, and abandoning it half way is worse than finishing it. So the deadline
// is a bound on waiting for a remote server, which is the unbounded-by-design
// part, and not a guarantee that the call returns within it.
func (ctx *MahresourcesContext) AddRemoteResource(reqCtx context.Context, resourceQuery *query_models.ResourceFromRemoteCreator) (*models.Resource, error) {
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	urls := strings.Split(resourceQuery.URL, "\n")
	var firstResource *models.Resource
	var firstError error

	// Get timeout values from config
	connectTimeout := ctx.Config.RemoteResourceConnectTimeout
	idleTimeout := ctx.Config.RemoteResourceIdleTimeout
	overallTimeout := ctx.Config.RemoteResourceOverallTimeout

	httpClient := createRemoteResourceHTTPClient(connectTimeout, overallTimeout)

	// A plugin-triggered fetch is policed by that plugin's declared network
	// list; an operator-initiated one (POST /v1/resource, /v1/resource/remote)
	// is policed by the host policy, which allows any public host and no
	// private address the operator has not named. The client is built per call
	// just above, so decorating it here cannot leak a policed connection into
	// an unpoliced pool.
	//
	// There is no undecorated branch left. This used to be the operator's
	// escape from every layer, and it was the reachable half of the
	// confused-deputy chain the plugin work closed from the other end.
	policy := ctx.hostFetchPolicy
	switch {
	case ctx.pluginEgress != nil:
		policy = *ctx.pluginEgress
	case ctx.pluginFetch:
		// A plugin is fetching but its policy did not reach here. Falling back
		// to the host policy would be wrong in the permissive direction — it
		// allows every public host, which is what the plugin's own list exists
		// to narrow — so refuse rather than substitute.
		return nil, fmt.Errorf("refusing to fetch: this plugin's network policy is not available")
	}
	httpClient = plugin_system.ApplyEgressPolicy(httpClient, policy, connectTimeout)

	// The caller's headers are refused here rather than at request time: by
	// then the submitter is gone, and a malformed header would surface as
	// every URL in the batch failing with net/http's own wording.
	if err := hostfetch.ValidateHeaders(resourceQuery.Headers); err != nil {
		return nil, err
	}
	userAgent := ctx.RemoteUserAgent()

	// hostFetch is true when nothing but the host policy applies. A plugin's
	// refusals are sanitized at the plugin boundary instead, in that origin's
	// own wording.
	hostFetch := ctx.pluginEgress == nil

	setError := func(err error) {
		// The log keeps the full error, including the address the name resolved
		// to; the caller is told less. Under -auth an ordinary user may submit
		// this, so echoing the resolved address back would turn a list of
		// failures into an internal network scan.
		ctx.Logger().Warning(models.LogActionCreate, "resource", nil, "Remote resource error", err.Error(), nil)
		if hostFetch {
			if msg, blocked := plugin_system.HostFetchRefusal(err); blocked {
				err = errors.New(msg)
			}
		}
		if firstError == nil {
			firstError = err
		}
	}

	for _, url := range urls {
		(func(url string) {
			// One deadline per URL, taken before the request rather than after
			// it. An HLS assembly's own overall bound starts inside hls.Fetch,
			// which is *after* this response has arrived -- so a server that
			// spent twenty-nine of its thirty permitted minutes delivering the
			// playlist would then get a fresh thirty for the segments.
			urlCtx := reqCtx
			if overallTimeout > 0 {
				// Zero means nobody configured one, which a context built from
				// a bare config has; passing it through would mean "already
				// expired" and refuse every fetch.
				var cancelURL context.CancelFunc
				urlCtx, cancelURL = context.WithTimeout(reqCtx, overallTimeout)
				defer cancelURL()
			}

			// NewRequestWithContext rather than Get: the client's own Timeout
			// bounds the transfer, but only the context can end it early when
			// the caller's budget runs out first.
			// Decorated per URL, not once for the batch: the custom headers
			// are bound to the host of the URL they were submitted for, and
			// AddRemoteResource fetches every line of a newline-separated
			// list. One decoration for all of them would send the first URL's
			// Cookie to the last URL's host.
			//
			// The same decorated client is handed to hls.Fetch below, so every
			// playlist, key and segment carries the User-Agent too -- which is
			// the point, since the 403 this exists for is answered by the
			// media endpoints, not only by the page that lists them.
			urlClient := hostfetch.Decorate(httpClient, userAgent, resourceQuery.Headers, url)

			req, err := http.NewRequestWithContext(urlCtx, http.MethodGet, url, nil)
			if err != nil {
				setError(err)
				return
			}
			resp, err := urlClient.Do(req)

			if err != nil {
				setError(err)
				return
			}

			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				setError(fmt.Errorf("remote URL returned HTTP %d: %s", resp.StatusCode, resp.Status))
				return
			}

			// Wrap response body with timeout reader to detect stalled transfers
			timeoutBody := newTimeoutReader(resp.Body, idleTimeout)
			defer timeoutBody.Close()

			if resourceQuery.GroupName != "" {
				category := models.Category{Name: resourceQuery.GroupCategoryName}

				if resourceQuery.GroupCategoryName != "" {
					if err := ctx.db.Where(&category).First(&category).Error; err != nil {
						if err := ctx.db.Save(&category).Error; err != nil {
							setError(err)
							return
						}
					}
				}

				group := models.Group{CategoryId: &category.ID, Name: resourceQuery.GroupName}

				if err := ctx.db.Where(&group).First(&group).Error; err != nil {
					if valErr := ValidateMeta(resourceQuery.GroupMeta); valErr != nil {
						setError(valErr)
						return
					}
					group.Meta = []byte(resourceQuery.GroupMeta)
					if err := ctx.db.Save(&group).Error; err != nil {
						setError(err)
						return
					}
				}

				resourceQuery.OwnerId = group.ID
			}

			name := resourceQuery.Name
			if name == "" {
				name = resourceQuery.FileName
			}

			// if the name is an empty string, try to get the name from the URL
			if name == "" {
				name = path.Base(url)
			}
			name = TrimEntityName(name)

			// The response may be an HLS playlist rather than the media
			// itself, in which case saving what arrived would store a few
			// kilobytes of text named like a video. Sniffed from the bytes,
			// because the URLs this matters for are generated endpoints
			// carrying neither an .m3u8 extension nor a playlist content type.
			//
			// The same httpClient is handed to the fetch, so every segment and
			// key it goes on to retrieve is policed by the same policy this
			// request was -- the plugin's own allowlist when a plugin is
			// fetching, the host's otherwise. That is the whole reason the
			// segments are fetched here rather than by ffmpeg.
			head := make([]byte, hls.SniffLen())
			read, readErr := io.ReadFull(timeoutBody, head)
			// A short read is ordinary -- most files are longer than the sniff,
			// but a four-byte one is not -- while any other error is the
			// transfer failing. Swallowing it stored the bytes that had already
			// arrived as a complete resource, so a connection dropped ten bytes
			// in produced a ten-byte file reported as a successful download.
			if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
				setError(readErr)
				return
			}
			head = head[:read]
			var content contracts.File = io.NopCloser(io.MultiReader(bytes.NewReader(head), timeoutBody))
			fileName := resourceQuery.FileName

			if hls.IsPlaylist(head) {
				// The URL the playlist was *served from*, not the one asked
				// for. A redirect from /watch to /cdn/abc/index.m3u8 moves the
				// base by a directory, and every relative segment reference
				// resolves against it.
				base := url
				if resp.Request != nil && resp.Request.URL != nil {
					base = resp.Request.URL.String()
				}
				assembled, hlsErr := hls.Fetch(urlCtx, ctx.hlsDeps(urlClient, policy), base, head, timeoutBody, ctx.hlsOptions(), nil)
				if hlsErr != nil {
					setError(hlsErr)
					return
				}
				defer assembled.Cleanup()
				defer assembled.Body.Close()
				content = io.NopCloser(assembled.Body)
				name = hls.OutputName(name)
				fileName = hls.OutputName(fileName)
			}

			res, err := ctx.AddResource(content, fileName, &query_models.ResourceCreator{
				ResourceQueryBase: query_models.ResourceQueryBase{
					Name:               name,
					Description:        resourceQuery.Description,
					OwnerId:            resourceQuery.OwnerId,
					Groups:             resourceQuery.Groups,
					Tags:               resourceQuery.Tags,
					Notes:              resourceQuery.Notes,
					Meta:               resourceQuery.Meta,
					ContentCategory:    resourceQuery.ContentCategory,
					Category:           resourceQuery.Category,
					ResourceCategoryId: resourceQuery.ResourceCategoryId,
					OriginalName:       url,
					OriginalLocation:   url,
					SeriesSlug:         resourceQuery.SeriesSlug,
				},
				// BH-023: PathName sits beside the embedded base rather than in
				// it, and this conversion copied the base field by field, so the
				// storage location the caller chose was dropped here -- silently,
				// since an unread key is also an unvalidated one.
				PathName: resourceQuery.PathName,
			})

			if err != nil {
				setError(err)
				return
			}

			if firstResource == nil {
				firstResource = res
			}
		})(strings.TrimSpace(url))
	}

	if firstResource == nil {
		return nil, firstError
	}

	return firstResource, firstError
}

func (ctx *MahresourcesContext) AddLocalResource(fileName string, resourceQuery *query_models.ResourceFromLocalCreator) (*models.Resource, error) {
	var existingResource models.Resource

	// BH-023: an empty PathName means the default filesystem, which is stored
	// as NULL rather than ''. SQL equality never matches NULL, so dedup has to
	// ask for it by name or this create path stops being idempotent.
	dedupe := ctx.db.Where("location = ?", resourceQuery.LocalPath)
	if resourceQuery.PathName == "" {
		dedupe = dedupe.Where("storage_location IS NULL")
	} else {
		dedupe = dedupe.Where("storage_location = ?", resourceQuery.PathName)
	}

	query := dedupe.First(&existingResource)
	if err := query.Error; err == nil && existingResource.ID != 0 {
		ctx.Logger().Info(models.LogActionCreate, "resource", &existingResource.ID, existingResource.Name, "Resource already exists, skipping", nil)
		// this resource is already saved, return it instead
		return &existingResource, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// some other db problem. record not found would have been ok, as we actually expect it to be the case.
		// here something else went wrong
		return nil, err
	}

	// BH-023: select the source filesystem the way AddResource does at :903.
	// The pointer is never nil, so passing it unconditionally resolved
	// altFileSystems with an empty key and failed with "alt fs is not
	// attached" for every caller that named no alt filesystem -- which is the
	// ordinary case, and the only one `mr resource from-local` can produce,
	// since that command has no --path-name flag to set.
	var storageLocation *string
	if resourceQuery.PathName != "" {
		storageLocation = &resourceQuery.PathName
	}

	fs, err := ctx.GetFsForStorageLocation(storageLocation)

	if err != nil {
		return nil, err
	}

	file, err := fs.Open(resourceQuery.LocalPath)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	fileMime, err := mimetype.DetectReader(file)

	if err != nil {
		return nil, err
	}

	// Seek back to start after DetectReader consumed initial bytes
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek after MIME detection: %w", err)
		}
	}

	fileBytes, err := io.ReadAll(file)

	if err != nil {
		return nil, err
	}

	h := sha1.New()
	h.Write(fileBytes)
	hash := hex.EncodeToString(h.Sum(nil))

	// Decode image dimensions for auto-detect rules (same as AddResource)
	var width, height int
	if strings.HasPrefix(fileMime.String(), "image/") {
		if img, _, decErr := image.Decode(bytes.NewReader(fileBytes)); decErr == nil {
			bounds := img.Bounds()
			width = bounds.Max.X
			height = bounds.Max.Y
		}
	}

	// The handler calls this as AddLocalResource(creator.Name, &creator), so a
	// request naming nothing arrives with both empty. AddResource is handed a
	// real filename by the upload; here the local path is the only name there
	// is. Set before the hook, so a plugin sees the same kind of name the
	// upload path shows it rather than an empty string.
	if fileName == "" {
		fileName = path.Base(resourceQuery.LocalPath)
	}

	if resourceQuery.OriginalName == "" {
		resourceQuery.OriginalName = fileName
	}

	hookData := map[string]any{
		"id":                   float64(0),
		"name":                 fileName,
		"description":          resourceQuery.Description,
		"meta":                 resourceQuery.Meta,
		"resource_category_id": float64(resourceQuery.ResourceCategoryId),
		"owner_id":             float64(resourceQuery.OwnerId),
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_resource_create", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if hName, ok := hookData["name"].(string); ok {
		fileName = hName
	}
	if hDesc, ok := hookData["description"].(string); ok {
		resourceQuery.Description = hDesc
	}
	if hMeta, ok := hookData["meta"].(string); ok {
		resourceQuery.Meta = hMeta
	}

	// Every sibling create path does this and this one did not: AddResource at
	// :891, CreateGroup at group_crud_context.go:26, the note path at
	// note_context.go:33. Empty Meta becomes []byte("") on the model, an
	// invalid json.RawMessage, so encoding the created resource fails after the
	// row has committed and the status line has gone out -- the caller gets
	// HTTP 200 with a zero-byte body. It runs after the hook, as AddResource
	// does, so a hook that clears meta is defaulted too.
	if resourceQuery.Meta == "" {
		resourceQuery.Meta = "{}"
	}

	if err := ValidateMeta(resourceQuery.Meta); err != nil {
		return nil, err
	}

	res := &models.Resource{
		Name:               fileName,
		Hash:               hash,
		HashType:           "SHA1",
		Location:           resourceQuery.LocalPath,
		Meta:               []byte(resourceQuery.Meta),
		OwnMeta:            []byte("{}"),
		Category:           resourceQuery.Category,
		ContentType:        fileMime.String(),
		ContentCategory:    resourceQuery.ContentCategory,
		ResourceCategoryId: ctx.resolveResourceCategory(resourceQuery.ResourceCategoryId, fileMime.String(), uint(width), uint(height), int64(len(fileBytes))),
		FileSize:           int64(len(fileBytes)),
		OwnerId:            uintPtrOrNil(resourceQuery.OwnerId),
		StorageLocation:    storageLocation,
		Description:        resourceQuery.Description,
		OriginalLocation:   resourceQuery.OriginalLocation,
		OriginalName:       resourceQuery.OriginalName,
		Width:              uint(width),
		Height:             uint(height),
	}

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Save(res).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := lockUploadAssociations(tx, resourceQuery.Groups, resourceQuery.Notes, resourceQuery.Tags); err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(resourceQuery.Groups) > 0 {
		groups := BuildAssociationSlice(resourceQuery.Groups, GroupFromID)
		if err := tx.Model(&res).Association("Groups").Append(&groups); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if len(resourceQuery.Notes) > 0 {
		notes := BuildAssociationSlice(resourceQuery.Notes, NoteFromID)
		if err := tx.Model(&res).Association("Notes").Append(&notes); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if len(resourceQuery.Tags) > 0 {
		tags := BuildAssociationSlice(resourceQuery.Tags, TagFromID)
		if err := tx.Model(&res).Association("Tags").Append(&tags); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if resourceQuery.SeriesId != 0 {
		var series models.Series
		if err := tx.First(&series, resourceQuery.SeriesId).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("series with id %d not found: %w", resourceQuery.SeriesId, err)
		}
		if err := ctx.AssignResourceToSeries(tx, res, &series, false); err != nil {
			tx.Rollback()
			return nil, err
		}
	} else if resourceQuery.SeriesSlug != "" {
		series, isCreator, err := ctx.GetOrCreateSeriesForResource(tx, resourceQuery.SeriesSlug)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := ctx.AssignResourceToSeries(tx, res, series, isCreator); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Create version 1 for the new resource
	version := &models.ResourceVersion{
		ResourceID:      res.ID,
		VersionNumber:   1,
		Hash:            res.Hash,
		HashType:        res.HashType,
		FileSize:        res.FileSize,
		ContentType:     res.ContentType,
		Width:           res.Width,
		Height:          res.Height,
		Location:        res.Location,
		StorageLocation: res.StorageLocation,
		Comment:         "Initial version",
	}

	if err := tx.Create(version).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Update resource's current version
	if err := tx.Model(&models.Resource{}).Where("id = ?", res.ID).Updates(map[string]interface{}{"current_version_id": version.ID}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	ctx.syncMentionsForResource(res)

	ctx.Logger().Info(models.LogActionCreate, "resource", &res.ID, res.Name, "Created resource from local path", nil)

	ctx.RunAfterPluginHooks("after_resource_create", map[string]any{
		"id":                   float64(res.ID),
		"name":                 res.Name,
		"description":          res.Description,
		"meta":                 string(res.Meta),
		"resource_category_id": float64(res.ResourceCategoryId),
		"owner_id":             hookID(res.OwnerId),
	})

	ctx.InvalidateSearchCacheByType(EntityTypeResource)
	return res, nil
}

// uploadTxMaxAttempts bounds the retry of an upload's write transaction, and
// uploadTxRetryBackoff is multiplied by the attempt number between tries.
const (
	uploadTxMaxAttempts  = 4
	uploadTxRetryBackoff = 25 * time.Millisecond
)

// lockUploadAssociations validates and locks every association id an upload
// names, in one canonical order: groups, then notes, then tags.
//
// Every transaction in the upload path that appends associations goes through
// this, including the one that attaches a single owner group — an exception
// would only be safe until someone widened it.
//
// The order is the point. Each of these takes SELECT ... FOR UPDATE on Postgres,
// and two transactions taking the same locks in opposite orders deadlock — which
// is what the upload paths did to each other, one locking groups/notes/tags and
// the other groups/tags/notes. Postgres aborts one of them with 40P01, a
// transaction that did nothing wrong. Taking them through one function is what
// stops the order drifting apart again.
func lockUploadAssociations(tx *gorm.DB, groups, notes, tags []uint) error {
	if err := ValidateAndLockAssociationIDs[models.Group](tx, groups, "groups"); err != nil {
		return err
	}
	if err := ValidateAndLockAssociationIDs[models.Note](tx, notes, "notes"); err != nil {
		return err
	}
	return ValidateAndLockAssociationIDs[models.Tag](tx, tags, "tags")
}

// withUploadTxRetry runs a write transaction, retrying it on lock contention.
//
// Every write transaction in the upload path needs this, for the same reason: a
// bulk batch is many concurrent requests, SQLite has one writer, and losing that
// race is not a reason to fail an upload. Retrying is safe because a failed
// attempt rolled back, so nothing partial persisted and re-running appends the
// same rows exactly once.
func (ctx *MahresourcesContext) withUploadTxRetry(run func() error) error {
	for attempt := 0; ; attempt++ {
		err := run()
		if err == nil {
			return nil
		}
		// A deadlock is retried too. Every lock this path takes is ordered
		// (see ValidateAndLockAssociationIDs and lockUploadAssociations), so one
		// should not happen — but 40P01 aborts a transaction that did nothing
		// wrong, and reporting it as a failed upload would be the same mistake
		// as reporting lock contention.
		if attempt >= uploadTxMaxAttempts-1 || (!isLockContentionError(err) && !isDeadlockError(err)) {
			return err
		}
		time.Sleep(uploadTxRetryBackoff * time.Duration(attempt+1))
	}
}

// insertUploadedResource is AddResource's write transaction: the resource row,
// its associations, its series membership and its initial version.
//
// Extracted so it can be retried as a unit on lock contention. See the call site
// for why that is safe.
func (ctx *MahresourcesContext) insertUploadedResource(res *models.Resource, resourceQuery *query_models.ResourceCreator) (err error) {
	tx := ctx.db.Begin()
	// The named return is load-bearing. recover() here exists to guarantee the
	// rollback, but a bare recover in a function with unnamed results returns
	// the zero error — nil — so a panicking GORM callback would roll the row
	// back and then report success. AddResource would go on to log the create,
	// fire after_resource_create and hand back a resource whose row does not
	// exist. Converting it to an error keeps the rollback guarantee, keeps the
	// process alive (net/http recovers per connection, and nothing in this
	// server maps a panic to a response), and cannot be mistaken for success.
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			err = fmt.Errorf("panic while writing the uploaded resource: %v", r)
		}
	}()

	if err := tx.Save(res).Error; err != nil {
		tx.Rollback()
		return err
	}

	if lockErr := lockUploadAssociations(tx, resourceQuery.Groups, resourceQuery.Notes, resourceQuery.Tags); lockErr != nil {
		tx.Rollback()
		return lockErr
	}

	if len(resourceQuery.Groups) > 0 {
		groups := BuildAssociationSlice(resourceQuery.Groups, GroupFromID)

		if createGroupsErr := tx.Model(&res).Association("Groups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return createGroupsErr
		}
	}

	if len(resourceQuery.Notes) > 0 {
		notes := BuildAssociationSlice(resourceQuery.Notes, NoteFromID)

		if createNotesErr := tx.Model(&res).Association("Notes").Append(&notes); createNotesErr != nil {
			tx.Rollback()
			return createNotesErr
		}
	}

	if len(resourceQuery.Tags) > 0 {
		tags := BuildAssociationSlice(resourceQuery.Tags, TagFromID)

		if createTagsErr := tx.Model(&res).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return createTagsErr
		}
	}

	// Series assignment
	if resourceQuery.SeriesId != 0 {
		var series models.Series
		if err := tx.First(&series, resourceQuery.SeriesId).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("series with id %d not found: %w", resourceQuery.SeriesId, err)
		}
		if err := ctx.AssignResourceToSeries(tx, res, &series, false); err != nil {
			tx.Rollback()
			return err
		}
	} else if resourceQuery.SeriesSlug != "" {
		series, isCreator, err := ctx.GetOrCreateSeriesForResource(tx, resourceQuery.SeriesSlug)
		if err != nil {
			tx.Rollback()
			return err
		}
		if err := ctx.AssignResourceToSeries(tx, res, series, isCreator); err != nil {
			tx.Rollback()
			return err
		}
	}

	// Create version 1 for the new resource
	version := &models.ResourceVersion{
		ResourceID:      res.ID,
		VersionNumber:   1,
		Hash:            res.Hash,
		HashType:        res.HashType,
		FileSize:        res.FileSize,
		ContentType:     res.ContentType,
		Width:           res.Width,
		Height:          res.Height,
		Location:        res.Location,
		StorageLocation: res.StorageLocation,
		Comment:         "Initial version",
	}

	if err := tx.Create(version).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Update resource's current version
	if err := tx.Model(&models.Resource{}).Where("id = ?", res.ID).Updates(map[string]interface{}{"current_version_id": version.ID}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	return nil
}

// mergeIntoExistingResource handles a content-hash collision: the bytes are
// already stored, so the upload turns into an association change on the row
// that holds them rather than a new resource.
//
// It runs before AddResource opens its write transaction, and takes short
// transactions of its own, so that the upload path never opens a transaction
// with a read. See the phase-1 comment in AddResource for why that matters.
// The caller must hold the per-hash idlock.
func (ctx *MahresourcesContext) mergeIntoExistingResource(existingResource *models.Resource, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	if existingResource.OwnerId != nil && resourceQuery.OwnerId == *existingResource.OwnerId {
		return ctx.mergeSameOwnerAssociations(existingResource, resourceQuery)
	}

	if resourceQuery.OwnerId == 0 {
		return nil, &ResourceExistsError{ResourceID: existingResource.ID, Reason: ReasonSameParent}
	}

	for _, group := range existingResource.Groups {
		if resourceQuery.OwnerId == group.ID {
			return nil, &ResourceExistsError{ResourceID: existingResource.ID, Reason: ReasonSameRelation}
		}
	}

	return ctx.attachOwnerToExistingResource(existingResource, resourceQuery)
}

// mergeSameOwnerAssociations appends whatever associations a duplicate upload
// carried to the resource that already holds its bytes, and then reports the
// collision.
//
// The id validation stays INSIDE the transaction, and contention is handled by
// retrying the whole thing rather than by making the first statement a write.
// Hoisting those SELECTs out was tried and is wrong: `Association.Append`
// upserts its target, so handed a group id whose row was deleted between the
// check and the append, GORM recreates it as a blank stub and then inserts the
// join. Validating outside the transaction does not merely fail to protect —
// it manufactures a phantom group, and the foreign key cannot catch it because
// by then the row exists again. Reading and writing under one transaction is
// the guarantee; losing the writer lock is the retry's problem.
func (ctx *MahresourcesContext) mergeSameOwnerAssociations(existingResource *models.Resource, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	// Nothing to append means nothing to write, so no transaction is opened.
	if len(resourceQuery.Groups) == 0 && len(resourceQuery.Tags) == 0 && len(resourceQuery.Notes) == 0 {
		return nil, &ResourceExistsError{ResourceID: existingResource.ID, Reason: ReasonSameParent}
	}

	err := ctx.withUploadTxRetry(func() (err error) {
		tx := ctx.db.Begin()
		// Named return: a bare recover in a function with unnamed results returns
		// nil, so a panic here would roll the associations back and report success.
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				err = fmt.Errorf("panic while merging into the existing resource: %v", r)
			}
		}()

		if lockErr := lockUploadAssociations(tx, resourceQuery.Groups, resourceQuery.Notes, resourceQuery.Tags); lockErr != nil {
			tx.Rollback()
			return lockErr
		}

		if len(resourceQuery.Groups) > 0 {
			groups := BuildAssociationSlice(resourceQuery.Groups, GroupFromID)
			if appendErr := tx.Model(existingResource).Association("Groups").Append(&groups); appendErr != nil {
				tx.Rollback()
				return appendErr
			}
		}

		if len(resourceQuery.Tags) > 0 {
			tags := BuildAssociationSlice(resourceQuery.Tags, TagFromID)
			if appendErr := tx.Model(existingResource).Association("Tags").Append(&tags); appendErr != nil {
				tx.Rollback()
				return appendErr
			}
		}

		if len(resourceQuery.Notes) > 0 {
			notes := BuildAssociationSlice(resourceQuery.Notes, NoteFromID)
			if appendErr := tx.Model(existingResource).Association("Notes").Append(&notes); appendErr != nil {
				tx.Rollback()
				return appendErr
			}
		}

		return tx.Commit().Error
	})
	if err != nil {
		return nil, err
	}

	return nil, &ResourceExistsError{ResourceID: existingResource.ID, Reason: ReasonSameParent}
}

// attachOwnerToExistingResource handles a collision from a *different* owner,
// which is not a refusal: the new owner group is attached to the resource that
// already holds these bytes, and it is returned as a success.
//
// The owner id is validated inside the transaction for the reason spelled out on
// mergeSameOwnerAssociations — Append would otherwise resurrect a group deleted
// since the request was made, as a blank row with the right id and nothing else.
// Master validated nothing here at all.
func (ctx *MahresourcesContext) attachOwnerToExistingResource(existingResource *models.Resource, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	err := ctx.withUploadTxRetry(func() (err error) {
		tx := ctx.db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				err = fmt.Errorf("panic while attaching the owner group: %v", r)
			}
		}()

		if valErr := lockUploadAssociations(tx, []uint{resourceQuery.OwnerId}, nil, nil); valErr != nil {
			tx.Rollback()
			return valErr
		}

		groups := &[]*models.Group{
			{ID: resourceQuery.OwnerId},
		}
		// Appended against a bare handle, not against existingResource: GORM
		// mutates the model's Groups slice in memory as well as the join table,
		// so a retried attempt would return an object listing the owner twice
		// even though the row is written once.
		target := &models.Resource{ID: existingResource.ID}
		if attachToGroupErr := tx.Model(target).Association("Groups").Append(groups); attachToGroupErr != nil {
			tx.Rollback()
			return attachToGroupErr
		}

		return tx.Commit().Error
	})
	if err != nil {
		return nil, err
	}

	// Appending through `target` keeps the caller's model unmutated across
	// retries, which also means it does not know about the group just
	// committed — and this object is what the API serialises, so a successful
	// different-owner collision would describe the resource without its new
	// owner. Re-read rather than patch the slice by hand.
	var refreshed models.Resource
	if reloadErr := ctx.db.Preload("Groups").First(&refreshed, existingResource.ID).Error; reloadErr != nil {
		// The write committed; failing the upload over a read that is only for
		// the response body would be the wrong trade.
		return existingResource, nil
	}
	return &refreshed, nil
}

func (ctx *MahresourcesContext) AddResource(file contracts.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	if err := ValidateEntityName(resourceQuery.Name, "resource"); err != nil {
		return nil, err
	}

	hookData := map[string]any{
		"id":                   float64(0),
		"name":                 resourceQuery.Name,
		"description":          resourceQuery.Description,
		"meta":                 resourceQuery.Meta,
		"resource_category_id": float64(resourceQuery.ResourceCategoryId),
		"owner_id":             float64(resourceQuery.OwnerId),
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_resource_create", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if hName, ok := hookData["name"].(string); ok {
		resourceQuery.Name = hName
	}
	if hDesc, ok := hookData["description"].(string); ok {
		resourceQuery.Description = hDesc
	}
	if hMeta, ok := hookData["meta"].(string); ok {
		resourceQuery.Meta = hMeta
	}

	// The same directory an HLS assembly works in, when one is configured.
	// AddResource copies its whole input through here, so an 8 GiB video
	// assembled onto the media volume was then copied to the root filesystem
	// anyway -- which is exactly the outage -hls-temp-dir exists to prevent,
	// arriving one step later. Empty keeps the system default, so a deployment
	// that sets nothing is unchanged.
	tempFile, err := os.CreateTemp(ctx.Config.HLSTempDir, "upload-")
	if err != nil {
		return nil, err
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	// Copy the contents of the uploaded file to the temporary file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		return nil, err
	}

	fileMime, err := mimetype.DetectFile(tempFile.Name())
	if err != nil {
		return nil, err
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Calculate the SHA1 hash of the uploaded file
	h := sha1.New()
	if _, err = io.Copy(h, tempFile); err != nil {
		return nil, err
	}

	if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	hash := hex.EncodeToString(h.Sum(nil))

	// Pre-compute image dimensions and file size before acquiring lock/transaction,
	// so that resolveResourceCategory can query the DB without conflicting with
	// the transaction's connection.
	preWidth := 0
	preHeight := 0
	if strings.HasPrefix(fileMime.String(), "image/") {
		if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		img, _, decErr := image.Decode(tempFile)
		if decErr != nil {
			// BH-039: Go's stdlib only decodes PNG/JPEG/GIF/BMP/TIFF/WebP
			// natively (plus whatever we register via _ imports). Formats
			// like SVG, ICO, AVIF, HEIC return image.ErrFormat — they are
			// valid images, we just can't extract dimensions here. Accept
			// them and store with Width=0/Height=0; the thumbnail pipeline
			// handles them via ffmpeg/libreoffice downstream.
			if errors.Is(decErr, image.ErrFormat) {
				preWidth = 0
				preHeight = 0
				// An SVG does carry its natural size, in its viewBox. Reading
				// it keeps the resource out of the "unknown aspect" state that
				// the preview pipeline handles worst (findings 72, 73).
				if models.BaseContentType(fileMime.String()) == "image/svg+xml" {
					if _, seekErr := tempFile.Seek(0, io.SeekStart); seekErr == nil {
						if w, h, ok := svgIntrinsicDimensionsFromReader(tempFile); ok {
							preWidth, preHeight = int(w), int(h)
						}
					}
				}
			} else {
				// BH-011 regression guard: genuine decode failure
				// (truncated PNG, corrupted JPEG, etc.) — reject.
				return nil, &InvalidImageError{Cause: decErr}
			}
		} else {
			bounds := img.Bounds()
			if bounds.Dx() == 0 || bounds.Dy() == 0 {
				return nil, &InvalidImageError{}
			}
			preWidth = bounds.Max.X
			preHeight = bounds.Max.Y
		}
	}
	preFileInfo, err := tempFile.Stat()
	if err != nil {
		return nil, err
	}
	preFileSize := preFileInfo.Size()
	resolvedCategoryID := ctx.resolveResourceCategory(resourceQuery.ResourceCategoryId, fileMime.String(), uint(preWidth), uint(preHeight), preFileSize)

	if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// Acquire per-hash lock to prevent race condition where two simultaneous uploads
	// with the same hash both pass the "existing resource" check before either commits.
	ctx.locks.ResourceHashLock.Acquire(hash)
	defer ctx.locks.ResourceHashLock.Release(hash)

	// ---------------------------------------------------------------------
	// Phase 1: the content-hash existence check, deliberately OUTSIDE any
	// transaction.
	//
	// The whole reason AddResource is split into phases is that the write
	// transaction below must open with a WRITE. This used to be the first
	// statement of that transaction, which made it a deferred read: on SQLite
	// in WAL mode, promoting such a snapshot to a write after another
	// connection has committed returns SQLITE_BUSY_SNAPSHOT, and for that
	// extended result code SQLite does not invoke the busy handler — so
	// busy_timeout does not apply and nothing here retries. Concurrent uploads
	// of *different* files reported "database is locked" to the user, on bytes
	// that had already been written to disk.
	//
	// Reading outside a transaction loses nothing. The per-hash idlock above is
	// what actually guarantees the dedup invariant: only another upload of this
	// same hash could change the answer, and it releases the lock only after it
	// has committed, so an autocommit read taken here is guaranteed to see it.
	var existingResource models.Resource

	switch lookupErr := ctx.db.Where("hash = ?", hash).Preload("Groups").First(&existingResource).Error; {
	case lookupErr == nil:
		return ctx.mergeIntoExistingResource(&existingResource, resourceQuery)
	case errors.Is(lookupErr, gorm.ErrRecordNotFound):
		// Genuinely new content: fall through and create it.
	default:
		// Anything else — a lock, a closed pool, a broken schema — is not an
		// answer to "does this hash already exist". Treating it as "no" was the
		// long-standing shape here and it was survivable while uploads were
		// sequential; concurrent uploads make a transient failure on this read
		// realistic, and Resource.Hash carries a plain index rather than a
		// unique one, so falling through would persist a second row for content
		// that already exists and quietly break the dedup invariant.
		return nil, lookupErr
	}

	// ---------------------------------------------------------------------
	// Phase 2: write the file, also OUTSIDE any transaction.
	//
	// This used to run between the transaction's snapshot-taking read and its
	// first write, so a full multi-gigabyte io.Copy sat inside the transaction
	// holding a pooled connection — the window that made the promotion conflict
	// above near-certain rather than rare.
	//
	// A transaction failure after this point leaves a file with no row, but
	// that state already existed and is already handled: the Stat branch below
	// reuses a stale file on the next attempt with the same hash.

	// BH-023: select target filesystem based on PathName (alt-fs key).
	targetFs := ctx.fs
	if resourceQuery.PathName != "" {
		if _, ok := ctx.Config.AltFileSystems[resourceQuery.PathName]; !ok {
			return nil, fmt.Errorf("unknown filesystem: %s", resourceQuery.PathName)
		}
		altFs, altErr := ctx.GetFsForStorageLocation(&resourceQuery.PathName)
		if altErr != nil {
			return nil, altErr
		}
		targetFs = altFs
	}

	folder := "/resources/" + hash[0:2] + "/" + hash[2:4] + "/" + hash[4:6] + "/"

	if err := targetFs.MkdirAll(folder, 0777); err != nil {
		return nil, err
	}

	var savedFile afero.File
	fileExists := false

	filePath := path.Join(folder, hash+fileMime.Extension())
	stat, statError := targetFs.Stat(filePath)

	if statError == nil && stat != nil {
		savedFile, err = targetFs.Open(filePath)
		log.Printf("Reusing stale file at %s", filePath)
		fileExists = true
	} else {
		savedFile, err = targetFs.Create(filePath)
	}

	if err != nil {
		return nil, err
	}

	defer func(savedFile afero.File) { _ = savedFile.Close() }(savedFile)

	if !fileExists {
		_, err = io.Copy(savedFile, tempFile)
		if err != nil {
			return nil, err
		}

		_, err = tempFile.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
	}

	name := fileName

	if resourceQuery.OriginalName == "" {
		resourceQuery.OriginalName = fileName
	}

	if resourceQuery.Name != "" {
		name = resourceQuery.Name
	}

	if resourceQuery.Meta == "" {
		resourceQuery.Meta = "{}"
	}

	if err := ValidateMeta(resourceQuery.Meta); err != nil {
		return nil, err
	}

	// Use pre-computed dimensions and file size (computed before the transaction)
	width := preWidth
	height := preHeight
	fileSize := preFileSize

	// A factory rather than a value, because phase 3 may run more than once and
	// its transaction mutates what it is given: GORM stamps the id and the GUID,
	// AssignResourceToSeries writes SeriesID and OwnMeta (series_context.go:227,
	// 243, 247). Re-running an attempt with a struct the previous one had
	// mutated carries a SeriesID whose row the rollback removed — on SQLite,
	// where foreign keys are ON, the retry then dies with "FOREIGN KEY
	// constraint failed" instead of succeeding. Building it fresh each attempt
	// closes that whole class rather than resetting the fields known today.
	newResource := func() *models.Resource {
		r := &models.Resource{
			Name:               name,
			Hash:               hash,
			HashType:           "SHA1",
			Location:           filePath,
			Meta:               []byte(resourceQuery.Meta),
			OwnMeta:            []byte("{}"),
			Category:           resourceQuery.Category,
			ContentType:        fileMime.String(),
			ContentCategory:    resourceQuery.ContentCategory,
			ResourceCategoryId: resolvedCategoryID,
			FileSize:           fileSize,
			OwnerId:            uintPtrOrNil(resourceQuery.OwnerId),
			Description:        resourceQuery.Description,
			OriginalLocation:   resourceQuery.OriginalLocation,
			OriginalName:       resourceQuery.OriginalName,
			Width:              uint(width),
			Height:             uint(height),
		}
		// BH-023: set StorageLocation when an alt-fs key was provided.
		if resourceQuery.PathName != "" {
			pn := resourceQuery.PathName
			r.StorageLocation = &pn
		}
		return r
	}

	// ---------------------------------------------------------------------
	// ---------------------------------------------------------------------
	// Phase 3: the write transaction, retried on lock contention.
	//
	// The phase ordering above removes the read-snapshot promotion described in
	// phase 1, and with it the great majority of the contention: 6-7 of 8
	// concurrent distinct-file uploads failed before it, none after. A residue
	// survives at roughly one request in a hundred at concurrency 4 over HTTP,
	// and it is worth being precise about what is and is not known about it.
	//
	// Known: those failures return "database is locked" in well under a
	// millisecond, so SQLite's busy handler demonstrably never engages for them
	// and the 10s busy_timeout is not what is being exhausted. Not known: which
	// lock they are losing. Disabling the hash and thumbnail workers does not
	// change the rate, and the transaction's first statement really is the
	// INSERT, so neither of the obvious explanations holds.
	//
	// Retrying is the right answer regardless of which SQLITE_BUSY variant it
	// is: the condition is transient write contention rather than a failure on
	// the statement's own merits, which is exactly what isLockContentionError
	// names. Measured 0 failures in 220 uploads at concurrency 4 with this loop.
	//
	// Retrying is safe because the transaction is the only thing being repeated:
	// a failed attempt rolled back, so nothing partial persisted, and the file
	// is already on disk from phase 2 (content-addressed, so the Stat branch
	// there reuses it rather than writing it twice).
	var res *models.Resource
	if insertErr := ctx.withUploadTxRetry(func() error {
		res = newResource()
		return ctx.insertUploadedResource(res, resourceQuery)
	}); insertErr != nil {
		return nil, insertErr
	}

	ctx.syncMentionsForResource(res)

	ctx.Logger().Info(models.LogActionCreate, "resource", &res.ID, res.Name, "Created resource", nil)

	ctx.RunAfterPluginHooks("after_resource_create", map[string]any{
		"id":                   float64(res.ID),
		"name":                 res.Name,
		"description":          res.Description,
		"meta":                 string(res.Meta),
		"resource_category_id": float64(res.ResourceCategoryId),
		"owner_id":             hookID(res.OwnerId),
	})

	ctx.InvalidateSearchCacheByType(EntityTypeResource)

	// Queue for async hash processing if it's a hashable image type
	if hash_worker.IsHashable(res.ContentType) {
		ctx.QueueForHashing(res.ID)
	}

	// Queue for async thumbnail pre-generation if it's a video
	if strings.HasPrefix(res.ContentType, "video/") {
		ctx.QueueForThumbnailing(res.ID)
	}

	return res, nil
}

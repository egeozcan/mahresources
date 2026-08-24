package application_context

// Defaults for the client-side bulk upload widget on /resource/new.
//
// These are runtime-only settings (see BuildDefaultsFromConfig): they govern
// browser behaviour on one page, so there is no boot flag, no env var and no
// MahresourcesConfig field to fall back to — the literals here are the fallback.
const (
	// defaultUploadConcurrency is how many files the widget sends at once.
	// SQLite has exactly one writer, so higher values mostly buy temp-file and
	// memory pressure there; Postgres tolerates more.
	defaultUploadConcurrency = 3

	// defaultUploadWidgetFileCount: selecting MORE than this many files switches
	// to the widget. At or below it, the browser posts the batch natively.
	defaultUploadWidgetFileCount = 10

	// defaultUploadWidgetSizeBytes: selecting MORE than this many bytes in total
	// switches to the widget, however few files that is. 1 GiB.
	defaultUploadWidgetSizeBytes = 1 << 30
)

// UploadConcurrency is how many files the bulk upload widget uploads at once.
//
// Read through here rather than Settings() directly, for the reason
// DownloadCockpitLimit documents: a context built from a raw
// MahresourcesConfig{} carries no settings service, and publishing 0 to the page
// would mean "no workers" — the widget would start nothing.
func (ctx *MahresourcesContext) UploadConcurrency() int {
	if s := ctx.settings; s != nil {
		if n := s.UploadConcurrency(); n > 0 {
			return n
		}
	}
	return defaultUploadConcurrency
}

// UploadWidgetFileCount is the file count above which /resource/new switches to
// the client-side upload widget. A published 0 would read as "every selection
// crosses the threshold", so the same zero-guard applies.
func (ctx *MahresourcesContext) UploadWidgetFileCount() int {
	if s := ctx.settings; s != nil {
		if n := s.UploadWidgetFileCount(); n > 0 {
			return n
		}
	}
	return defaultUploadWidgetFileCount
}

// UploadWidgetSizeBytes is the total selection size above which /resource/new
// switches to the client-side upload widget.
func (ctx *MahresourcesContext) UploadWidgetSizeBytes() int64 {
	if s := ctx.settings; s != nil {
		if n := s.UploadWidgetSizeBytes(); n > 0 {
			return n
		}
	}
	return defaultUploadWidgetSizeBytes
}

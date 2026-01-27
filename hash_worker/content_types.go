package hash_worker

// HashableContentTypes is the set of content types that can be perceptually hashed.
var HashableContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// hashableContentTypesList is the pre-computed list of hashable content types
// to avoid recreating the slice on every batch.
var hashableContentTypesList = func() []string {
	list := make([]string, 0, len(HashableContentTypes))
	for ct := range HashableContentTypes {
		list = append(list, ct)
	}
	return list
}()

// IsHashable returns true if the content type can be perceptually hashed.
func IsHashable(contentType string) bool {
	return HashableContentTypes[contentType]
}

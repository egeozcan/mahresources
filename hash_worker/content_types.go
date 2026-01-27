package hash_worker

// HashableContentTypes is the set of content types that can be perceptually hashed.
var HashableContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// IsHashable returns true if the content type can be perceptually hashed.
func IsHashable(contentType string) bool {
	return HashableContentTypes[contentType]
}

package archive

import (
	"fmt"
	"strings"
)

// digestHexLen is the length of the lowercase hex digest for each hash_type
// an export carries. SHA1 is the only type mahresources computes; an empty
// hash_type is read as SHA1, the column default, because rows written before
// the type was stamped carry none.
var digestHexLen = map[string]int{"SHA1": 40}

// ValidDigest reports whether hash is the lowercase hex digest that hashType
// produces. The importer builds a storage path from the hash, so anything
// else ("../_job_replay_key", an uppercase or truncated digest) must be
// refused before it gets there.
func ValidDigest(hashType, hash string) bool {
	if hashType == "" {
		hashType = "SHA1"
	}
	n, ok := digestHexLen[strings.ToUpper(hashType)]
	return ok && len(hash) == n && isLowerHex(hash)
}

// IsDigest reports whether hash is the lowercase hex digest of some known
// hash type. A blob entry is named by its hash alone, with no type beside it.
func IsDigest(hash string) bool {
	for _, n := range digestHexLen {
		if len(hash) == n && isLowerHex(hash) {
			return true
		}
	}
	return false
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// checkResourceDigests refuses a resource payload whose own hash or a
// version's hash is not a digest of its hash_type. An empty hash names no
// blob and is left to the importer.
func checkResourceDigests(entry string, p *ResourcePayload) error {
	if p.Hash != "" && !ValidDigest(p.HashType, p.Hash) {
		return &FormatError{Err: fmt.Errorf("archive: %s: hash %q is not a lowercase hex %s digest", entry, p.Hash, digestTypeName(p.HashType))}
	}
	for _, v := range p.Versions {
		if v.Hash != "" && !ValidDigest(v.HashType, v.Hash) {
			return &FormatError{Err: fmt.Errorf("archive: %s: version %s hash %q is not a lowercase hex %s digest", entry, v.VersionExportID, v.Hash, digestTypeName(v.HashType))}
		}
	}
	return nil
}

func digestTypeName(hashType string) string {
	if hashType == "" {
		return "SHA1"
	}
	return hashType
}

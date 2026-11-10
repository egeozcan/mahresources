package template_context_providers

import (
	"os"
	"path/filepath"
)

// MissingBuiltAssets returns the build outputs every page loads that are
// absent under root, the server's working directory. They are gitignored and
// written by `npm run build`, so a checkout that only ran `go build` has
// neither, and its pages render with no stylesheet and no script.
func MissingBuiltAssets(root string) []string {
	var missing []string
	for _, asset := range []string{"public/tailwind.css", "public/dist/main.js"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(asset))); err != nil {
			missing = append(missing, asset)
		}
	}
	return missing
}

package application_context

import (
	"fmt"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"mahresources/models/query_models"
)

// A caller that reuses one creator for several files must not see the first
// file's name become the second's OriginalName. AddResource used to write the
// default back into the creator it was handed, so the second call found
// OriginalName already set. The multipart handler copies the creator per file,
// which hid this; any other caller that loops with one creator did not.
func TestAddResource_ReusedCreatorDoesNotInheritOriginalName(t *testing.T) {
	ctx := createTestContext(t)
	creator := &query_models.ResourceCreator{}
	stamp := time.Now().UnixNano()

	first, err := ctx.AddResource(newBytesFile([]byte(fmt.Sprintf("first %d", stamp))), "first.txt", creator)
	require.NoError(t, err)
	second, err := ctx.AddResource(newBytesFile([]byte(fmt.Sprintf("second %d", stamp))), "second.txt", creator)
	require.NoError(t, err)

	require.Equal(t, "first.txt", first.OriginalName)
	require.Equal(t, "second.txt", second.OriginalName)
	require.Empty(t, creator.OriginalName, "the caller's creator must not be mutated")
}

func TestAddLocalResource_ReusedQueryDoesNotInheritOriginalName(t *testing.T) {
	ctx := createTestContext(t)
	stamp := time.Now().UnixNano()
	query := &query_models.ResourceFromLocalCreator{}

	for _, base := range []string{"one", "two"} {
		p := fmt.Sprintf("/incoming/origname-%s-%d.txt", base, stamp)
		purgeResourcesAt(t, ctx, p)
		require.NoError(t, afero.WriteFile(ctx.fs, p, []byte(fmt.Sprintf("%s %d", base, stamp)), 0644))
		query.LocalPath = p
		res, err := ctx.AddLocalResource("", query)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("origname-%s-%d.txt", base, stamp), res.OriginalName)
	}
	require.Empty(t, query.OriginalName, "the caller's query must not be mutated")
}

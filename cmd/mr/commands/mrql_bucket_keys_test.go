package commands

import "testing"

// The bucketed half of the /mrql column-ordering finding (docs/todo.md, Batch 13).
//
// `mr mrql run` prints one header line per bucket, and it built that line by
// ranging the key map and calling sort.Strings on the result — so
// `GROUP BY width, height` and `GROUP BY height, width` printed
// `--- height=50, width=100 ---` for both. The web table and the CSV export
// follow the author's order, which is the "the app disagrees with itself"
// argument that made the change worth taking on the other two surfaces.
func TestBucketKeyPartsFollowsTheAuthoredGroupBy(t *testing.T) {
	key := map[string]any{"contentType": "image/png", "height": 50, "width": 100}

	got := bucketKeyParts([]string{"width", "height", "contentType"}, key)
	want := []string{"width=100", "height=50", "contentType=image/png"}
	if !equalSlices(got, want) {
		t.Errorf("bucketKeyParts = %v, want %v", got, want)
	}

	// The same key object, the other query. Before the fix both printed the
	// alphabetical order and were indistinguishable.
	got = bucketKeyParts([]string{"contentType", "height", "width"}, key)
	want = []string{"contentType=image/png", "height=50", "width=100"}
	if !equalSlices(got, want) {
		t.Errorf("reversed: bucketKeyParts = %v, want %v", got, want)
	}
}

// A relation bucket carries a "<field>_id" the server adds so two same-named
// groups stay distinguishable. It is not a group-by column, so keyColumns does
// not name it — and it must still be printed.
func TestBucketKeyPartsKeepsUnnamedKeys(t *testing.T) {
	key := map[string]any{"owner": "Photos", "owner_id": 12}

	got := bucketKeyParts([]string{"owner"}, key)
	want := []string{"owner=Photos", "owner_id=12"}
	if !equalSlices(got, want) {
		t.Errorf("bucketKeyParts = %v, want %v", got, want)
	}
}

// An older server sends no keyColumns. The header then prints exactly what it
// used to, rather than nothing.
func TestBucketKeyPartsFallsBackToSortedOrder(t *testing.T) {
	key := map[string]any{"width": 100, "contentType": "image/png"}

	got := bucketKeyParts(nil, key)
	want := []string{"contentType=image/png", "width=100"}
	if !equalSlices(got, want) {
		t.Errorf("bucketKeyParts = %v, want %v", got, want)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

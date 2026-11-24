package application_context

import (
	"bytes"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

var allWinnerCriteria = []string{
	models.WinnerCriterionPixelsDesc, models.WinnerCriterionPixelsAsc,
	models.WinnerCriterionSizeDesc, models.WinnerCriterionSizeAsc,
	models.WinnerCriterionCreatedAsc, models.WinnerCriterionCreatedDesc,
	models.WinnerCriterionUpdatedAsc, models.WinnerCriterionUpdatedDesc,
	models.WinnerCriterionNameAsc, models.WinnerCriterionNameDesc,
	models.WinnerCriterionContentTypeAsc, models.WinnerCriterionContentTypeDesc,
	models.WinnerCriterionHasDescription,
	models.WinnerCriterionAssociationsDesc, models.WinnerCriterionAssociationsAsc,
}

var keyTestTimes = []time.Time{
	{}, // zero: missing
	time.Date(1, 1, 1, 0, 0, 0, 1, time.UTC),
	time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
	time.Date(1969, 12, 31, 23, 59, 59, 999999999, time.UTC),
	time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC),
	time.Date(2024, 5, 6, 7, 8, 9, 1, time.UTC),
	time.Date(2024, 5, 6, 7, 8, 9, 0, time.FixedZone("x", 3600)), // another instant, same wall clock
	time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC),
}

var keyTestStrings = []string{
	"", "a", "ab", "b", "a\x00", "a\x00b", "\x00", "\x00\x00", "a\xff", "\xff", " ", "　x", "é",
	strings.Repeat("x", 700), strings.Repeat("x", 700) + "y", strings.Repeat("x", 699),
}

func randomKeyCandidate(rng *rand.Rand, id uint) winnerKeyCandidate {
	pick := func() time.Time { return keyTestTimes[rng.Intn(len(keyTestTimes))] }
	str := func() string { return keyTestStrings[rng.Intn(len(keyTestStrings))] }
	sizes := []int64{-5, 0, 1, 2, 1 << 40, -1 << 62}
	desc := []string{"", " ", "　\t", "x", " x "}[rng.Intn(5)]
	res := &models.Resource{
		Width: uint(rng.Intn(4) * 100), Height: uint(rng.Intn(3) * 50), FileSize: sizes[rng.Intn(len(sizes))],
		CreatedAt: pick(), UpdatedAt: pick(), Name: str(), ContentType: str(), Description: desc,
	}
	res.ID = id
	return winnerKeyCandidate{
		WinnerCandidate: models.WinnerCandidate{Resource: res, Associations: rng.Intn(4)},
		HasDescription:  strings.TrimSpace(desc) != "",
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// bytes.Compare of two keys has the sign of CompareByWinnerRule, for every
// criterion alone and for random multi-criterion rules, in both argument orders.
func TestWinnerSortKeyOrdersAsTheWinnerRuleDoes(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var rules [][]string
	for _, c := range allWinnerCriteria {
		rules = append(rules, []string{c})
	}
	for i := 0; i < 300; i++ {
		var rule []string
		for n := 1 + rng.Intn(5); n > 0; n-- {
			rule = append(rule, allWinnerCriteria[rng.Intn(len(allWinnerCriteria))])
		}
		rules = append(rules, rule)
	}
	rules = append(rules, models.DefaultWinnerRule(), nil, []string{"not-a-criterion", models.WinnerCriterionSizeAsc})
	for _, rule := range rules {
		for i := 0; i < 400; i++ {
			a, b := randomKeyCandidate(rng, uint(1+rng.Intn(5))), randomKeyCandidate(rng, uint(1+rng.Intn(5)))
			want := sign(models.CompareByWinnerRule(rule, a.WinnerCandidate, b.WinnerCandidate))
			require.Equal(t, want, sign(bytes.Compare(winnerSortKey(rule, a), winnerSortKey(rule, b))), "rule %v", rule)
			require.Equal(t, -want, sign(bytes.Compare(winnerSortKey(rule, b), winnerSortKey(rule, a))), "rule %v reversed", rule)
		}
	}
}

// Two candidates with nothing but a negative file size tie on size and fall to the
// next criterion, as Go's comparator does.
func TestWinnerSortKeyMissingValuesFallThrough(t *testing.T) {
	mk := func(size int64, name string, id uint) winnerKeyCandidate {
		r := &models.Resource{FileSize: size, Name: name}
		r.ID = id
		return winnerKeyCandidate{WinnerCandidate: models.WinnerCandidate{Resource: r}}
	}
	rule := []string{models.WinnerCriterionSizeDesc, models.WinnerCriterionNameAsc}
	a, b := mk(-9, "b", 1), mk(0, "a", 2)
	require.Equal(t, 1, sign(bytes.Compare(winnerSortKey(rule, a), winnerSortKey(rule, b))), "both sizes are missing, so the name decides")
	require.Equal(t, 1, sign(models.CompareByWinnerRule(rule, a.WinnerCandidate, b.WinnerCandidate)))
}

func TestWinnerKeyPrefixIsTheFirst512Bytes(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 1000)
	require.Len(t, winnerKeyPrefix(key), 512)
	require.Equal(t, []byte{1, 2}, winnerKeyPrefix([]byte{1, 2}))
}

// The database orders the seed table's BLOB keys exactly as bytes.Compare does,
// NUL bytes, 0xFF bytes, prefixes and long shared prefixes included.
func testSeedKeysOrderAsBytesDo(t *testing.T, ctx *MahresourcesContext) {
	rng := rand.New(rand.NewSource(11))
	var keys [][]byte
	alphabet := []byte{0x00, 0x01, 0x7f, 0x80, 0xfe, 0xff}
	for i := 0; i < 400; i++ {
		k := make([]byte, 1+rng.Intn(12))
		for j := range k {
			k[j] = alphabet[rng.Intn(len(alphabet))]
		}
		keys = append(keys, k)
	}
	for i := 0; i < 20; i++ {
		keys = append(keys, append(bytes.Repeat([]byte{0xab}, 600), byte(i)), bytes.Repeat([]byte{0xab}, 500+i))
	}
	var rows []models.ReductionSeed
	for i, k := range keys {
		rows = append(rows, models.ReductionSeed{ReductionID: 1, Generation: "g", ResourceID: uint(i + 1), KeyPrefix: winnerKeyPrefix(k), SortKey: k})
	}
	require.NoError(t, ctx.db.CreateInBatches(rows, 100).Error)

	var got []models.ReductionSeed
	require.NoError(t, ctx.db.Where("reduction_id = 1 AND generation = 'g'").Order("key_prefix, sort_key, resource_id").Find(&got).Error)
	want := append([]models.ReductionSeed(nil), rows...)
	sort.SliceStable(want, func(i, j int) bool {
		if c := bytes.Compare(want[i].KeyPrefix, want[j].KeyPrefix); c != 0 {
			return c < 0
		}
		if c := bytes.Compare(want[i].SortKey, want[j].SortKey); c != 0 {
			return c < 0
		}
		return want[i].ResourceID < want[j].ResourceID
	})
	require.Len(t, got, len(want))
	for i := range want {
		require.Equal(t, want[i].ResourceID, got[i].ResourceID, "position %d", i)
	}
}

func TestSeedKeysOrderAsBytesDoSQLite(t *testing.T) {
	testSeedKeysOrderAsBytesDo(t, newReductionSQLiteContext(t))
}

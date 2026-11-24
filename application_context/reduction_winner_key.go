package application_context

import (
	"encoding/binary"

	"mahresources/models"
)

// winnerKeyCandidate is a Winner Rule candidate with the one fact the key cannot
// derive from the row: whether the description holds anything but whitespace.
// The streaming compute computes it in SQL (design D5), so no description enters Go.
type winnerKeyCandidate struct {
	models.WinnerCandidate
	HasDescription bool
}

// winnerKeyPrefixLen is how much of a key the seed table's indexed column holds
// (a Postgres btree row must stay under about 2.7 KB).
const winnerKeyPrefixLen = 512

// winnerKeyPrefix is the indexed prefix of a key.
func winnerKeyPrefix(key []byte) []byte {
	if len(key) > winnerKeyPrefixLen {
		return key[:winnerKeyPrefixLen]
	}
	return key
}

// winnerSortKey encodes a candidate so that bytes.Compare of two keys has the
// sign of models.CompareByWinnerRule on the same two candidates (design D7). Each
// criterion writes a presence byte, 0x00 present and 0x01 missing, which is never
// complemented so a missing value sorts last in both directions, then a value
// whose bytes are complemented for a descending criterion. A missing value writes
// fixed zero value bytes, so two missing candidates tie on the criterion and fall
// through to the next, as Go's comparators do. The Resource id closes the key.
func winnerSortKey(rule []string, c winnerKeyCandidate) []byte {
	var key []byte
	r := c.Resource
	for _, criterion := range rule {
		switch criterion {
		case models.WinnerCriterionPixelsDesc, models.WinnerCriterionPixelsAsc:
			pixels := uint64(r.Width) * uint64(r.Height)
			key = appendNumber(key, pixels > 0, pixels, criterion == models.WinnerCriterionPixelsDesc)
		case models.WinnerCriterionSizeDesc, models.WinnerCriterionSizeAsc:
			key = appendNumber(key, r.FileSize > 0, uint64(r.FileSize)^(1<<63), criterion == models.WinnerCriterionSizeDesc)
		case models.WinnerCriterionCreatedAsc, models.WinnerCriterionCreatedDesc:
			key = appendTime(key, r.CreatedAt.IsZero(), r.CreatedAt.Unix(), r.CreatedAt.Nanosecond(), criterion == models.WinnerCriterionCreatedDesc)
		case models.WinnerCriterionUpdatedAsc, models.WinnerCriterionUpdatedDesc:
			key = appendTime(key, r.UpdatedAt.IsZero(), r.UpdatedAt.Unix(), r.UpdatedAt.Nanosecond(), criterion == models.WinnerCriterionUpdatedDesc)
		case models.WinnerCriterionNameAsc, models.WinnerCriterionNameDesc:
			key = appendString(key, r.Name, criterion == models.WinnerCriterionNameDesc)
		case models.WinnerCriterionContentTypeAsc, models.WinnerCriterionContentTypeDesc:
			key = appendString(key, r.ContentType, criterion == models.WinnerCriterionContentTypeDesc)
		case models.WinnerCriterionHasDescription:
			// No missing state: better first.
			if c.HasDescription {
				key = append(key, 0x00)
			} else {
				key = append(key, 0x01)
			}
		case models.WinnerCriterionAssociationsDesc, models.WinnerCriterionAssociationsAsc:
			// No presence byte: zero is a real count.
			key = appendValue(key, binary.BigEndian.AppendUint64(nil, uint64(c.Associations)), criterion == models.WinnerCriterionAssociationsDesc)
		}
	}
	return binary.BigEndian.AppendUint64(key, uint64(r.ID))
}

func appendValue(key, value []byte, descending bool) []byte {
	if descending {
		for _, b := range value {
			key = append(key, ^b)
		}
		return key
	}
	return append(key, value...)
}

func appendNumber(key []byte, present bool, value uint64, descending bool) []byte {
	if !present {
		return append(key, 0x01, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	return appendValue(append(key, 0x00), binary.BigEndian.AppendUint64(nil, value), descending)
}

func appendTime(key []byte, missing bool, seconds int64, nanos int, descending bool) []byte {
	if missing {
		return append(key, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	value := binary.BigEndian.AppendUint64(nil, uint64(seconds)^(1<<63))
	value = binary.BigEndian.AppendUint32(value, uint32(nanos))
	return appendValue(append(key, 0x00), value, descending)
}

// appendString escapes NUL as 0x00 0xFF and ends the value with 0x00 0x00, which no
// escaped byte pair equals, so a prefix sorts before its extensions.
func appendString(key []byte, s string, descending bool) []byte {
	if s == "" {
		return append(key, 0x01, 0x00, 0x00)
	}
	value := make([]byte, 0, len(s)+2)
	for i := 0; i < len(s); i++ {
		value = append(value, s[i])
		if s[i] == 0x00 {
			value = append(value, 0xFF)
		}
	}
	value = append(value, 0x00, 0x00)
	return appendValue(append(key, 0x00), value, descending)
}

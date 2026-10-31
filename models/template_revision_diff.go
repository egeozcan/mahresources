package models

import "strings"

// Line diff operations.
const (
	DiffOpEqual  = "equal"
	DiffOpInsert = "insert"
	DiffOpDelete = "delete"
)

// DiffHunk is a run of consecutive lines that share one operation.
type DiffHunk struct {
	Op    string   `json:"op"`
	Lines []string `json:"lines"`
}

// FieldDiff is the line diff of one changed field.
type FieldDiff struct {
	Field string     `json:"field"`
	Hunks []DiffHunk `json:"hunks"`
}

// SlotDiff groups the changed fields of one slot.
type SlotDiff struct {
	Slot   string      `json:"slot"`
	Fields []FieldDiff `json:"fields"`
}

// maxDiffCells bounds the LCS table; beyond it the differing middle is
// reported as one delete followed by one insert instead of a minimal diff.
const maxDiffCells = 25_000_000

// DiffTemplateSnapshots diffs two snapshots under the canonical view. Slots
// and fields with no change are left out.
func DiffTemplateSnapshots(carrierType string, from, to TemplateSnapshot) []SlotDiff {
	out := []SlotDiff{}
	for _, slot := range TemplateSlots(carrierType) {
		sd := SlotDiff{Slot: slot.Name, Fields: []FieldDiff{}}
		for _, f := range slot.Fields {
			a := CanonicalTemplateField(carrierType, f, from)
			b := CanonicalTemplateField(carrierType, f, to)
			if a == b {
				continue
			}
			sd.Fields = append(sd.Fields, FieldDiff{Field: f.Name, Hunks: DiffLines(a, b)})
		}
		if len(sd.Fields) > 0 {
			out = append(out, sd)
		}
	}
	return out
}

func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// DiffLines returns a line diff of a against b as hunks.
func DiffLines(a, b string) []DiffHunk {
	x, y := splitDiffLines(a), splitDiffLines(b)

	prefix := 0
	for prefix < len(x) && prefix < len(y) && x[prefix] == y[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(x)-prefix && suffix < len(y)-prefix && x[len(x)-1-suffix] == y[len(y)-1-suffix] {
		suffix++
	}

	var hunks []DiffHunk
	add := func(op string, lines []string) {
		if len(lines) == 0 {
			return
		}
		if n := len(hunks); n > 0 && hunks[n-1].Op == op {
			hunks[n-1].Lines = append(hunks[n-1].Lines, lines...)
			return
		}
		hunks = append(hunks, DiffHunk{Op: op, Lines: append([]string(nil), lines...)})
	}

	add(DiffOpEqual, x[:prefix])
	mx, my := x[prefix:len(x)-suffix], y[prefix:len(y)-suffix]
	if len(mx)*len(my) > maxDiffCells {
		add(DiffOpDelete, mx)
		add(DiffOpInsert, my)
	} else {
		// lcs[i][j] is the LCS length of mx[i:] and my[j:].
		lcs := make([][]int32, len(mx)+1)
		for i := range lcs {
			lcs[i] = make([]int32, len(my)+1)
		}
		for i := len(mx) - 1; i >= 0; i-- {
			for j := len(my) - 1; j >= 0; j-- {
				switch {
				case mx[i] == my[j]:
					lcs[i][j] = lcs[i+1][j+1] + 1
				case lcs[i+1][j] >= lcs[i][j+1]:
					lcs[i][j] = lcs[i+1][j]
				default:
					lcs[i][j] = lcs[i][j+1]
				}
			}
		}
		i, j := 0, 0
		for i < len(mx) && j < len(my) {
			switch {
			case mx[i] == my[j]:
				add(DiffOpEqual, mx[i:i+1])
				i++
				j++
			case lcs[i+1][j] >= lcs[i][j+1]:
				add(DiffOpDelete, mx[i:i+1])
				i++
			default:
				add(DiffOpInsert, my[j:j+1])
				j++
			}
		}
		add(DiffOpDelete, mx[i:])
		add(DiffOpInsert, my[j:])
	}
	add(DiffOpEqual, x[len(x)-suffix:])
	return hunks
}

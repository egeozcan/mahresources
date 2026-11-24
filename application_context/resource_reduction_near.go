package application_context

// similarPair is one stored perceptual edge as the clustering reads it: the
// neighbour and the distance to the Resource it was probed from.
type similarPair struct {
	ResourceID uint
	Distance   uint8
}

// similarityThreshold is the read threshold the rest of the app uses, from live
// settings. MaxStoredPDistance is a hard write-time ceiling, so a threshold above
// it would widen the net over a pair table that never held those edges.
func (ctx *MahresourcesContext) similarityThreshold() int {
	threshold := ctx.Config.HashSimilarityThreshold
	if settings := ctx.Settings(); settings != nil {
		if v := settings.HashSimilarityThreshold(); v > 0 {
			threshold = v
		}
	}
	if threshold <= 0 {
		threshold = 10
	}
	return threshold
}

package application_context

import "mahresources/models"

// selectionPreviewForTest answers the review's selection preview through the
// production path (selectionSummary): the explicit Resources the principal can see,
// counted, and the first names among them.
func selectionPreviewForTest(s *reductionStore, r *models.ResourceReduction) (int64, []string, error) {
	sum, err := s.selectionSummary(r)
	if err != nil {
		return 0, nil, err
	}
	names := make([]string, 0, len(sum.resourceRefs))
	for _, ref := range sum.resourceRefs {
		names = append(names, ref.Name)
	}
	return int64(sum.resources), names, nil
}

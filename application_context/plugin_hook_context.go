package application_context

func hookID(id *uint) float64 {
	if id == nil {
		return 0
	}
	return float64(*id)
}

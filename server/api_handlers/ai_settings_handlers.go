package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/constants"
	"mahresources/contracts"
)

func GetAIProfilesHandler(ctx interface {
	AIGenerationProfiles() contracts.AIGenerationProfiles
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", constants.JSON)
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(ctx.AIGenerationProfiles())
	}
}

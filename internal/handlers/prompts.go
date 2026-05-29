package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/LogicShao/novel-dehydrator/internal/services/prompts"
)

func HandleGetDefaultPrompts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"system":          prompts.DehydrateSystem,
			"user":            prompts.DehydrateUser,
			"position_early":  prompts.PositionHintEarly,
			"position_normal": prompts.PositionHintNormal,
		})
	}
}

package dashboard

import (
	"encoding/json"
	"net/http"

	"github.com/ggai/k8ops/internal/cost"
)

// handleCostSummary returns a cluster-wide cost breakdown by namespace.
// GET /api/cost/summary
func (s *Server) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	est := cost.NewEstimator(s.clientset, cost.PricingFromEnv())
	summary, err := est.Summary(r.Context())
	if err != nil {
		s.log.Error("cost summary failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to calculate cost summary")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}

// handleCostRecommendations returns right-sizing suggestions to reduce spend.
// GET /api/cost/recommendations
func (s *Server) handleCostRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	est := cost.NewEstimator(s.clientset, cost.PricingFromEnv())
	recs, err := est.Recommendations(r.Context())
	if err != nil {
		s.log.Error("cost recommendations failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate cost recommendations")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recs)
}

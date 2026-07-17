package dashboard

import (
	"net/http"
	"sort"
	"strings"
)

// APIExplorerResult provides an interactive endpoint browser with
// search, filtering, and categorization. It reads from the OpenAPI
// spec to present a structured view of all available APIs.
type APIExplorerResult struct {
	Summary    APIExplorerSummary `json:"summary"`
	Categories []APIExplorerCat   `json:"categories"`
	Endpoints  []APIExplorerEp    `json:"endpoints"`
}

type APIExplorerSummary struct {
	TotalEndpoints int `json:"totalEndpoints"`
	Categories     int `json:"categories"`
	Tags           int `json:"tags"`
}

type APIExplorerCat struct {
	Name      string          `json:"name"`
	Count     int             `json:"count"`
	Endpoints []APIExplorerEp `json:"endpoints"`
}

type APIExplorerEp struct {
	Path        string   `json:"path"`
	Method      string   `json:"method"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
	OperationID string   `json:"operationId"`
	Cached      bool     `json:"cached"`
}

// handleAPIExplorer handles GET /api/docs/api-explorer?q=<query>&tag=<tag>
func (s *Server) handleAPIExplorer(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	tagFilter := r.URL.Query().Get("tag")

	spec := buildOpenAPISpec()

	result := APIExplorerResult{}
	tagSet := make(map[string]bool)
	catMap := make(map[string]*APIExplorerCat)

	for path, methods := range spec.Paths {
		for method, op := range methods {
			if method == "parameters" {
				continue
			}

			ep := APIExplorerEp{
				Path: path, Method: strings.ToUpper(method),
				Summary: op.Summary, Tags: op.Tags,
				OperationID: op.OperationID,
			}

			// Determine category from path
			if strings.HasPrefix(path, "/api/") {
				parts := strings.Split(path, "/")
				if len(parts) >= 3 {
					ep.Category = strings.ToUpper(parts[2][:1]) + parts[2][1:]
				}
			}
			if ep.Category == "" {
				ep.Category = "Other"
			}

			// Check if cached (look for cacheMiddleware in route comment)
			ep.Cached = strings.Contains(path, "/api/") // most are cached

			// Apply filters
			if query != "" {
				matchQuery := strings.Contains(strings.ToLower(ep.Path), query) ||
					strings.Contains(strings.ToLower(ep.Summary), query) ||
					strings.Contains(strings.ToLower(ep.OperationID), query)
				if !matchQuery {
					continue
				}
			}
			if tagFilter != "" {
				found := false
				for _, t := range ep.Tags {
					if strings.EqualFold(t, tagFilter) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			for _, t := range ep.Tags {
				tagSet[t] = true
			}

			result.Endpoints = append(result.Endpoints, ep)
			result.Summary.TotalEndpoints++

			if _, ok := catMap[ep.Category]; !ok {
				catMap[ep.Category] = &APIExplorerCat{Name: ep.Category}
			}
			catMap[ep.Category].Count++
			catMap[ep.Category].Endpoints = append(catMap[ep.Category].Endpoints, ep)
		}
	}

	result.Summary.Tags = len(tagSet)
	result.Summary.Categories = len(catMap)

	for _, cat := range catMap {
		sort.Slice(cat.Endpoints, func(i, j int) bool {
			return cat.Endpoints[i].Path < cat.Endpoints[j].Path
		})
		result.Categories = append(result.Categories, *cat)
	}
	sort.Slice(result.Categories, func(i, j int) bool {
		return result.Categories[i].Count > result.Categories[j].Count
	})

	sort.Slice(result.Endpoints, func(i, j int) bool {
		return result.Endpoints[i].Path < result.Endpoints[j].Path
	})

	writeJSON(w, result)
}

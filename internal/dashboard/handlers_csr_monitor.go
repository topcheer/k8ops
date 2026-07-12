package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	certv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CSRResult is the Certificate Signing Request analysis.
type CSRResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         CSRSummary `json:"summary"`
	PendingCSRs     []CSREntry `json:"pendingCSRs"`
	ApprovedCSRs    []CSREntry `json:"approvedCSRs"`
	DeniedCSRs      []CSREntry `json:"deniedCSRs"`
	ExpiredCSRs     []CSREntry `json:"expiredCSRs"`
	StaleCSRs       []CSREntry `json:"staleCSRs"` // pending > 1h
	Recommendations []string   `json:"recommendations"`
}

// CSRSummary aggregates CSR statistics.
type CSRSummary struct {
	Total        int            `json:"total"`
	Pending      int            `json:"pending"`
	Approved     int            `json:"approved"`
	Denied       int            `json:"denied"`
	Expired      int            `json:"expired"`
	StalePending int            `json:"stalePending"` // pending > 1h
	BySigner     map[string]int `json:"bySigner"`
	ByRequester  map[string]int `json:"byRequester"`
	HealthScore  int            `json:"healthScore"`
}

// CSREntry describes one CSR.
type CSREntry struct {
	Name        string    `json:"name"`
	SignerName  string    `json:"signerName"`
	Requester   string    `json:"requester"`
	Username    string    `json:"username"`
	Status      string    `json:"status"` // Pending, Approved, Denied
	CreatedAt   time.Time `json:"createdAt"`
	AgeDuration string    `json:"age"`
	Expiration  string    `json:"expiration,omitempty"`
	DenyReason  string    `json:"denyReason,omitempty"`
	IsStale     bool      `json:"isStale"`
	Severity    string    `json:"severity,omitempty"`
}

// handleCSRMonitor monitors Certificate Signing Requests and node bootstrap certificates.
// GET /api/operations/csr-monitor
func (s *Server) handleCSRMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	csrs, err := rc.clientset.CertificatesV1().CertificateSigningRequests().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	now := time.Now()
	result := CSRResult{
		ScannedAt: now,
		Summary: CSRSummary{
			BySigner:    map[string]int{},
			ByRequester: map[string]int{},
		},
	}
	result.Summary.Total = len(csrs.Items)

	for _, csr := range csrs.Items {
		entry := CSREntry{
			Name:        csr.Name,
			SignerName:  csr.Spec.SignerName,
			Username:    csr.Spec.Username,
			CreatedAt:   csr.CreationTimestamp.Time,
			AgeDuration: formatDuration(now.Sub(csr.CreationTimestamp.Time)),
		}

		// Extract requester from annotations or groups
		if len(csr.Spec.Groups) > 0 {
			entry.Requester = csr.Spec.Groups[0]
		}

		// Determine status from conditions
		status := "Unknown"
		denyReason := ""
		var certExpiration time.Time

		for _, cond := range csr.Status.Conditions {
			switch cond.Type {
			case certv1.CertificateApproved:
				status = "Approved"
			case certv1.CertificateDenied:
				status = "Denied"
				denyReason = cond.Reason
				if denyReason == "" {
					denyReason = cond.Message
				}
			}
		}

		// If no explicit condition, check if certificate is issued
		if status == "Unknown" {
			if len(csr.Status.Certificate) > 0 {
				status = "Approved"
			} else {
				status = "Pending"
			}
		}

		// Check expiration from the certificate (if present)
		if len(csr.Status.Certificate) > 0 {
			// We can't easily parse the cert without crypto/x509 in this handler,
			// but we can note that it's been issued
			entry.Expiration = "issued"
		}

		entry.Status = status
		entry.DenyReason = denyReason

		// Check staleness (pending > 1h)
		age := now.Sub(csr.CreationTimestamp.Time)
		if status == "Pending" && age > time.Hour {
			entry.IsStale = true
			entry.Severity = "high"
			if age > 24*time.Hour {
				entry.Severity = "critical"
			}
		}

		// Update summary
		result.Summary.BySigner[entry.SignerName]++
		result.Summary.ByRequester[entry.Requester]++

		switch status {
		case "Pending":
			result.Summary.Pending++
			result.PendingCSRs = append(result.PendingCSRs, entry)
			if entry.IsStale {
				result.Summary.StalePending++
				result.StaleCSRs = append(result.StaleCSRs, entry)
			}
		case "Approved":
			result.Summary.Approved++
			if len(result.ApprovedCSRs) < 20 {
				result.ApprovedCSRs = append(result.ApprovedCSRs, entry)
			}
		case "Denied":
			result.Summary.Denied++
			result.DeniedCSRs = append(result.DeniedCSRs, entry)
		}

		// Check expired (old approved CSRs > 7 days)
		if status == "Approved" && age > 7*24*time.Hour {
			result.Summary.Expired++
			result.ExpiredCSRs = append(result.ExpiredCSRs, entry)
		}

		_ = certExpiration
	}

	// Sort pending by age (oldest first)
	sort.Slice(result.PendingCSRs, func(i, j int) bool {
		return result.PendingCSRs[i].CreatedAt.Before(result.PendingCSRs[j].CreatedAt)
	})
	sort.Slice(result.StaleCSRs, func(i, j int) bool {
		return result.StaleCSRs[i].CreatedAt.Before(result.StaleCSRs[j].CreatedAt)
	})

	result.Summary.HealthScore = csrScore(result.Summary)
	result.Recommendations = csrRecommendations(&result)

	writeJSON(w, result)
}

// csrScore computes a 0-100 health score.
func csrScore(s CSRSummary) int {
	score := 100

	if s.Pending > 0 {
		score -= min(30, s.Pending*10)
	}

	if s.StalePending > 0 {
		score -= min(40, s.StalePending*20)
	}

	if s.Denied > 5 {
		score -= min(10, (s.Denied-5)*2)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// csrRecommendations generates actionable recommendations.
func csrRecommendations(r *CSRResult) []string {
	var recs []string

	if r.Summary.Pending > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d CSR(s) are pending approval — review and approve legitimate requests to unblock node bootstrap or service TLS",
			r.Summary.Pending,
		))
	}

	if r.Summary.StalePending > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d CSR(s) have been pending for over 1 hour — these may be blocking critical operations, investigate immediately",
			r.Summary.StalePending,
		))
	}

	if r.Summary.Denied > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d CSR(s) were denied — review deny reasons and ensure legitimate requests use correct signer and groups",
			r.Summary.Denied,
		))
	}

	if r.Summary.Total > 100 {
		recs = append(recs, fmt.Sprintf(
			"%d total CSRs — consider setting up automatic CSR approval (e.g., kubelet-serving cert approver) for bootstrap certificates",
			r.Summary.Total,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "CSR lifecycle is healthy — no pending or stale requests")
	}

	return recs
}

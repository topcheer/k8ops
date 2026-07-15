package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestWebhookHealth_NoWebhooks(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/operations/webhook-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleWebhookHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result WhHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalWebhooks != 0 {
		t.Errorf("expected 0 webhooks, got %d", result.Summary.TotalWebhooks)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestWebhookHealth_FailOpen(t *testing.T) {
	failOpen := admissionv1.Ignore
	clientset := k8sfake.NewSimpleClientset(
		&admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "my-validator"},
			Webhooks: []admissionv1.ValidatingWebhook{{
				Name:          "validator.example.com",
				FailurePolicy: &failOpen,
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Namespace: "webhook-system",
						Name:      "validator-svc",
					},
				},
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"pods"},
					},
				}},
			}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/webhook-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleWebhookHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result WhHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.FailOpenCount != 1 {
		t.Errorf("expected 1 fail-open, got %d", result.Summary.FailOpenCount)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestWebhookHealth_LongTimeout(t *testing.T) {
	timeout := int32(60)
	fail := admissionv1.Fail
	clientset := k8sfake.NewSimpleClientset(
		&admissionv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "my-mutator"},
			Webhooks: []admissionv1.MutatingWebhook{{
				Name:           "mutator.example.com",
				FailurePolicy:  &fail,
				TimeoutSeconds: &timeout,
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Namespace: "webhook-system",
						Name:      "mutator-svc",
					},
				},
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
					Rule: admissionv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"pods"},
					},
				}},
			}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/webhook-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleWebhookHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result WhHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.LongTimeout != 1 {
		t.Errorf("expected 1 long timeout, got %d", result.Summary.LongTimeout)
	}
}

func TestWebhookHealth_MatchAll(t *testing.T) {
	fail := admissionv1.Fail
	clientset := k8sfake.NewSimpleClientset(
		&admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "catch-all"},
			Webhooks: []admissionv1.ValidatingWebhook{{
				Name:          "catchall.example.com",
				FailurePolicy: &fail,
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{Namespace: "ns", Name: "svc"},
				},
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{admissionv1.Create},
					Rule: admissionv1.Rule{
						APIGroups:   []string{"*"},
						APIVersions: []string{"*"},
						Resources:   []string{"*"},
					},
				}},
			}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/webhook-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleWebhookHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result WhHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.MatchAllResources != 1 {
		t.Errorf("expected 1 match-all, got %d", result.Summary.MatchAllResources)
	}
}

// Package collector collects events from the Kubernetes cluster
// and triggers diagnostics automatically.
package collector

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EventCollector watches Kubernetes events and creates DiagnosticReports.
type EventCollector struct {
	k8sClient   client.Client
	clientset   *kubernetes.Clientset
	log         *slog.Logger
	triggerChan chan *aiv1alpha1.DiagnosticReport

	mu          sync.Mutex
	processed   map[string]time.Time // dedup key → last processed time
	cooldown    time.Duration
	dedupWindow time.Duration
}

// NewEventCollector creates a new event collector.
func NewEventCollector(k8sClient client.Client, config *rest.Config, log *slog.Logger) (*EventCollector, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &EventCollector{
		k8sClient:   k8sClient,
		clientset:   clientset,
		log:         log,
		triggerChan: make(chan *aiv1alpha1.DiagnosticReport, 100),
		processed:   make(map[string]time.Time),
		cooldown:    5 * time.Minute,
		dedupWindow: 10 * time.Minute,
	}, nil
}

// TriggerChannel returns the channel where new DiagnosticReports are sent.
func (c *EventCollector) TriggerChannel() <-chan *aiv1alpha1.DiagnosticReport {
	return c.triggerChan
}

// Start begins watching events.
func (c *EventCollector) Start(ctx context.Context) {
	go c.watchEvents(ctx)
	c.log.Info("event collector started")
}

// Event patterns that should trigger diagnostics.
var triggerPatterns = []string{
	"BackOff",
	"CrashLoopBackOff",
	"Failed",
	"Unhealthy",
	"FailedScheduling",
	"ImagePullBackOff",
	"ErrImagePull",
	"OOMKilled",
	"Evicted",
	"NodeNotReady",
	"SystemOOM",
	"ContainerCreating",
}

// shouldTrigger checks if an event should trigger a diagnostic.
func (c *EventCollector) shouldTrigger(reason string) bool {
	for _, pattern := range triggerPatterns {
		if strings.Contains(reason, pattern) {
			return true
		}
	}
	return false
}

func (c *EventCollector) watchEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.doWatch(ctx)
		if err != nil {
			c.log.Error("event watch error", "error", err)
			time.Sleep(5 * time.Second) // retry delay
		}
	}
}

func (c *EventCollector) doWatch(ctx context.Context) error {
	watcher, err := c.clientset.CoreV1().Events("").Watch(ctx, metav1.ListOptions{
		TimeoutSeconds: ptr(int64(300)), // 5-minute watch window
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return nil // watch closed, will reconnect
			}
			if event.Type != watch.Added && event.Type != watch.Modified {
				continue
			}

			obj, ok := event.Object.(*corev1.Event)
			if !ok {
				continue
			}

			if !c.shouldTrigger(obj.Reason) {
				continue
			}

			// Build dedup key
			key := fmt.Sprintf("%s/%s/%s", obj.InvolvedObject.Kind, obj.InvolvedObject.Namespace, obj.InvolvedObject.Name)

			c.mu.Lock()
			if lastTime, exists := c.processed[key]; exists {
				if time.Since(lastTime) < c.cooldown {
					c.mu.Unlock()
					continue
				}
			}
			c.processed[key] = time.Now()
			// Clean old entries
			for k, t := range c.processed {
				if time.Since(t) > c.dedupWindow {
					delete(c.processed, k)
				}
			}
			c.mu.Unlock()

			// Create diagnostic report
			report := &aiv1alpha1.DiagnosticReport{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf("auto-%s-%s-", strings.ToLower(obj.InvolvedObject.Kind), strings.ToLower(obj.Reason)),
					Namespace:    obj.InvolvedObject.Namespace,
					Labels: map[string]string{
						"aiops.ggai.dev/auto":   "true",
						"aiops.ggai.dev/reason": obj.Reason,
					},
				},
				Spec: aiv1alpha1.DiagnosticReportSpec{
					Trigger: aiv1alpha1.DiagnosticTrigger{
						Type:         aiv1alpha1.TriggerEvent,
						EventMessage: obj.Message,
						Reason:       obj.Reason,
						ResourceRef: &aiv1alpha1.ResourceRef{
							APIVersion: obj.InvolvedObject.APIVersion,
							Kind:       obj.InvolvedObject.Kind,
							Namespace:  obj.InvolvedObject.Namespace,
							Name:       obj.InvolvedObject.Name,
						},
					},
				},
			}

			report.Status.Phase = "Pending"

			if err := c.k8sClient.Create(ctx, report); err != nil {
				c.log.Error("failed to create diagnostic report", "error", err, "key", key)
				continue
			}

			c.log.Info("triggered diagnostic",
				"report", report.Name,
				"reason", obj.Reason,
				"resource", fmt.Sprintf("%s/%s/%s", obj.InvolvedObject.Kind, obj.InvolvedObject.Namespace, obj.InvolvedObject.Name),
			)
		}
	}
}

// RegisterScheme registers required schemes.
func RegisterScheme(scheme *runtime.Scheme) error {
	_ = corev1.AddToScheme(scheme)
	return nil
}

func ptr[T any](v T) *T { return &v }

// SetupWithManager sets up the collector with a controller manager.
func (c *EventCollector) SetupWithManager(mgr ctrl.Manager) error {
	return nil
}

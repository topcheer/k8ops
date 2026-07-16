// Package k8s — Cluster audit tool that exposes all dashboard audit endpoints to the LLM agent.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ggai/k8ops/internal/tools"
)

// auditEndpoint maps an audit name to its dashboard API path.
type auditEndpoint struct {
	name string
	path string
	desc string
}

// auditRegistry is the complete list of all audit/analysis endpoints available to the agent.
// When adding a new audit endpoint in server.go, also add it here so the agent can use it.
var auditRegistry = []auditEndpoint{
	// --- Product ---
	{"product:staleness", "/api/product/staleness", "Workload staleness & release cadence"},
	{"product:ingress-health", "/api/product/ingress-health", "Ingress traffic routing health"},
	{"product:namespace-lifecycle", "/api/product/namespaces/lifecycle", "Namespace governance & lifecycle"},
	{"product:dns-health", "/api/product/dns-health", "DNS resolution health checker"},
	{"product:config-audit", "/api/product/config-audit", "ConfigMap & Secret configuration audit"},
	{"product:network-policy", "/api/product/network-policy", "Network policy compliance & traffic isolation"},
	{"product:label-hygiene", "/api/product/label-hygiene", "Label & annotation hygiene auditor"},
	{"product:orphaned-resources", "/api/product/orphaned-resources", "Orphaned resource detector"},
	{"product:pvc-health", "/api/product/pvc-health", "PV/PVC storage health & capacity"},
	{"product:statefulset-audit", "/api/product/statefulset-audit", "StatefulSet health & ordered rollout audit"},
	{"product:affinity-conflict", "/api/product/affinity-conflict", "Affinity & anti-affinity conflict detector"},
	{"product:taint-toleration", "/api/product/taint-toleration", "Node taint & pod toleration impact analyzer"},
	{"product:configmap-size", "/api/product/configmap-size", "ConfigMap/Secret size & memory pressure auditor"},
	{"product:job-health", "/api/product/job-health", "Batch job execution health & completion analyzer"},
	{"product:hpa-health", "/api/product/hpa-health", "HPA health & scaling activity analyzer"},
	{"product:api-deprecation", "/api/product/api-deprecation", "Deprecated API version & upgrade readiness checker"},
	{"product:qos-priority", "/api/product/qos-priority", "Pod QoS & priority class distribution auditor"},
	{"product:service-connectivity", "/api/product/service-connectivity", "Service endpoint & connectivity health auditor"},
	{"product:topology-spread", "/api/product/topology-spread", "Topology spread constraint validator"},
	{"product:backup-compliance", "/api/product/backup-compliance", "Volume snapshot & PVC backup compliance auditor"},
	{"product:init-container-audit", "/api/product/init-container-audit", "Init container reliability & startup dependency auditor"},
	{"product:hpa-gap", "/api/product/hpa-gap", "HPA target utilization gap & scaling behavior auditor"},
	{"product:mesh-health", "/api/product/mesh-health", "Service mesh sidecar health & mTLS coverage auditor"},

	// --- Deployment ---
	{"deployment:image-hygiene", "/api/deployment/image-hygiene", "Container image deployment hygiene analyzer"},
	{"deployment:revision-history", "/api/deployment/revision-history", "Deployment revision history & rollback readiness"},
	{"deployment:disruption-impact", "/api/deployment/disruption-impact", "Deployment PDB disruption & maintenance impact"},
	{"deployment:workload-maturity", "/api/deployment/workload-maturity", "Workload maturity & best practices scorer"},
	{"deployment:ephemeral-storage", "/api/deployment/ephemeral-storage", "Ephemeral storage & emptyDir limit compliance"},
	{"deployment:config-sync", "/api/deployment/config-sync", "ConfigMap/Secret config sync & staleness detector"},
	{"deployment:sidecar-audit", "/api/deployment/sidecar-audit", "Sidecar container overhead & injection auditor"},
	{"deployment:restart-policy", "/api/deployment/restart-policy", "Restart policy & lifecycle hook auditor"},
	{"deployment:scale-readiness", "/api/deployment/scale-readiness", "Deployment scale readiness & autoscaling gap detector"},
	{"deployment:rollout-health", "/api/deployment/rollout-health", "Deployment rollout strategy & health analyzer"},
	{"deployment:probe-compliance", "/api/deployment/probe-compliance", "Health probe compliance auditor"},
	{"deployment:resource-limits", "/api/deployment/resource-limits", "Resource limit & enforcement gap audit"},
	{"deployment:graceful-shutdown", "/api/deployment/graceful-shutdown", "Graceful shutdown & termination compliance"},
	{"deployment:update-strategy", "/api/deployment/update-strategy", "Deployment update strategy & rollback readiness"},
	{"deployment:ref-integrity", "/api/deployment/ref-integrity", "Secret/ConfigMap reference integrity checker"},
	{"deployment:image-drift", "/api/deployment/image-drift", "Deployment image drift & version consistency detector"},
	{"deployment:replica-availability", "/api/deployment/replica-availability", "Deployment replica availability & ready pod ratio monitor"},
	{"deployment:helm-health", "/api/deployment/helm-health", "Helm release health & GitOps drift detector"},
	{"deployment:surge-risk", "/api/deployment/surge-risk", "Rolling update risk & surge configuration analyzer"},

	// --- Operations ---
	{"operations:cronjobs-health", "/api/operations/cronjobs/health", "CronJob execution health"},
	{"operations:slo", "/api/operations/slo", "SLO/SLA error budget"},
	{"operations:event-storm", "/api/operations/event-storm", "Event storm & cascade detection"},
	{"operations:probes", "/api/operations/probes", "Health probe effectiveness audit"},
	{"operations:health-score", "/api/operations/health-score", "Cluster health score aggregator"},
	{"operations:node-pressure", "/api/operations/node-pressure", "Node condition & resource pressure"},
	{"operations:oom-tracker", "/api/operations/oom-tracker", "Container OOM kill tracker"},
	{"operations:crashloop", "/api/operations/crashloop", "CrashLoopBackOff detector & crash pattern analyzer"},
	{"operations:pdb-audit", "/api/operations/pdb-audit", "PDB compliance & voluntary disruption risk"},
	{"operations:topology-distribution", "/api/operations/topology-distribution", "Topology spread & pod distribution audit"},
	{"operations:image-pull-failures", "/api/operations/image-pull-failures", "Image pull & container start failure tracker"},
	{"operations:restart-reasons", "/api/operations/restart-reasons", "Pod restart reason analyzer"},
	{"operations:scheduling-latency", "/api/operations/scheduling-latency", "Pod scheduling latency analyzer"},
	{"operations:resource-contention", "/api/operations/resource-contention", "Resource contention & throttling detector"},
	{"operations:node-lease", "/api/operations/node-lease", "Node lease & heartbeat health monitor"},
	{"operations:control-plane", "/api/operations/control-plane", "Control plane health checker"},
	{"operations:pod-evictions", "/api/operations/pod-evictions", "Pod eviction & node pressure history tracker"},
	{"operations:api-latency", "/api/operations/api-latency", "API server responsiveness & pod start latency monitor"},
	{"operations:volume-mount-errors", "/api/operations/volume-mount-errors", "Volume mount & attach error tracker"},
	{"operations:pod-startup", "/api/operations/pod-startup", "Pod startup lifecycle & bottleneck analyzer"},
	{"operations:kubelet-health", "/api/operations/kubelet-health", "Kubelet & container runtime health monitor"},
	{"operations:dns-health", "/api/operations/dns-health", "DNS resolution health & CoreDNS monitor"},
	{"operations:csr-monitor", "/api/operations/csr-monitor", "Certificate signing request & node bootstrap cert monitor"},
	{"operations:etcd-health", "/api/operations/etcd-health", "etcd health & database pressure monitor"},
	{"operations:api-load", "/api/operations/api-load", "API server request throughput & load pressure monitor"},
	{"operations:prom-health", "/api/operations/prom-health", "Prometheus rule health & alert coverage auditor"},
	{"operations:alertmanager-health", "/api/operations/alertmanager-health", "Alertmanager config & alert routing health auditor"},

	// --- Security ---
	{"security:audit", "/api/security/audit", "Cluster-wide security scan"},
	{"security:secrets", "/api/security/secrets", "Secret data exposure & credential leak scanner"},
	{"security:network-policies", "/api/security/network-policies", "NetworkPolicy audit"},
	{"security:health", "/api/security/health", "Platform security health check"},
	{"security:compliance", "/api/security/compliance", "CIS benchmark compliance scan"},
	{"security:pods", "/api/security/pods", "Pod security posture scan"},
	{"security:secrets-rotation", "/api/security/secrets/rotation", "Secret lifecycle & rotation audit"},
	{"security:images", "/api/security/images", "Image supply chain security"},
	{"security:containers", "/api/security/containers", "Container security context audit"},
	{"security:rbac-risk", "/api/security/rbac-risk", "RBAC permission risk analysis"},
	{"security:service-accounts", "/api/security/service-accounts", "ServiceAccount security audit"},
	{"security:rbac-effective", "/api/security/rbac-effective", "RBAC effective permissions & escalation"},
	{"security:admission-audit", "/api/security/admission-audit", "Admission webhook configuration audit"},
	{"security:audit-policy", "/api/security/audit-policy", "API server audit logging configuration checker"},
	{"security:encryption-at-rest", "/api/security/encryption-at-rest", "Secret encryption at rest configuration checker"},
	{"security:host-namespace", "/api/security/host-namespace", "Container host namespace & privilege exposure auditor"},
	{"security:cert-expiry", "/api/security/cert-expiry", "Certificate & TLS expiry monitor"},
	{"security:volume-mounts", "/api/security/volume-mounts", "Volume & mount risk security audit"},
	{"security:endpoint-exposure", "/api/security/endpoint-exposure", "Service endpoint exposure & attack surface audit"},
	{"security:seccomp-audit", "/api/security/seccomp-audit", "Seccomp profile & PSS restricted compliance"},
	{"security:batch-audit", "/api/security/batch-audit", "CronJob & batch job security audit"},
	{"security:psa-audit", "/api/security/psa-audit", "Pod security admission enforcement auditor"},
	{"security:mac-audit", "/api/security/mac-audit", "AppArmor & SELinux MAC compliance auditor"},
	{"security:forensics", "/api/security/forensics", "Pod security forensics & incident evidence collector"},
	{"security:rbac-audit", "/api/security/rbac-audit", "RBAC overprivilege & wildcard permission auditor"},
	{"security:secret-scan", "/api/security/secret-scan", "Secret data exposure & env var credential leak scanner"},
	{"security:sec-drift", "/api/security/sec-drift", "Security context drift & runtime policy compliance auditor"},
	{"security:opa-compliance", "/api/security/opa-compliance", "OPA/Gatekeeper policy compliance & constraint violation auditor"},
	{"security:image-vuln", "/api/security/image-vuln", "Container image vulnerability & patch lag auditor"},
	{"product:cronjob-schedule", "/api/product/cronjob-schedule", "CronJob schedule conflict & resource configuration auditor"},
	{"deployment:startup-latency", "/api/deployment/startup-latency", "Pod startup latency & readiness performance auditor"},
	{"operations:grafana-health", "/api/operations/grafana-health", "Grafana dashboard availability & datasource health auditor"},
	{"security:kyverno-compliance", "/api/security/kyverno-compliance", "Kyverno policy compliance & cluster policy audit"},
	{"scalability:alloc-efficiency", "/api/scalability/alloc-efficiency", "Resource request vs limit allocation efficiency auditor"},
	{"product:external-secret-health", "/api/product/external-secret-health", "External secrets & secret store CSI health auditor"},
	{"deployment:progressive-delivery", "/api/deployment/progressive-delivery", "Progressive delivery & canary rollout health auditor"},
	{"operations:metrics-pipeline", "/api/operations/metrics-pipeline", "Metrics pipeline & kube-state-metrics health auditor"},
	{"security:pss-scorecard", "/api/security/pss-scorecard", "Pod Security Standards compliance scorecard"},
	{"scalability:hpa-performance", "/api/scalability/hpa-performance", "HPA autoscaling performance & scaling event auditor"},
	{"product:endpoint-dns-health", "/api/product/endpoint-dns-health", "Service endpoint & DNS resolution health auditor"},
	{"product:config-mount-risk", "/api/product/config-mount-risk", "ConfigMap & Secret mount injection risk auditor"},
	{"deployment:rs-staleness", "/api/deployment/rs-staleness", "ReplicaSet staleness & rollout history auditor"},
	{"operations:audit-log-health", "/api/operations/audit-log-health", "Audit log pipeline & event export health auditor"},
	{"security:sa-token-audit", "/api/security/sa-token-audit", "SA token rotation & access risk auditor"},
	{"scalability:pv-reclaim", "/api/scalability/pv-reclaim", "PV reclaim policy & storage class waste auditor"},
	{"deployment:gitops-sync", "/api/deployment/gitops-sync", "ArgoCD & Flux GitOps sync status & drift auditor"},
	{"operations:alert-noise", "/api/operations/alert-noise", "Alert noise & fatigue detection auditor"},
	{"security:supply-chain", "/api/security/supply-chain", "Supply chain & SBOM coverage security auditor"},
	{"security:quota-security", "/api/security/quota-security", "Resource quota & limit range security auditor"},
	{"security:policy-drift", "/api/security/policy-drift", "Security policy drift & baseline configuration auditor"},
	{"operations:log-pipeline", "/api/operations/log-pipeline", "Log aggregation & forwarding pipeline health auditor"},
	{"product:runtime-class", "/api/product/runtime-class", "Container runtime class & OCI image compliance auditor"},
	{"deployment:image-pull-audit", "/api/deployment/image-pull-audit", "Image pull policy & secret management auditor"},
	{"scalability:vpa-audit", "/api/scalability/vpa-audit", "VPA configuration & resource recommendation quality auditor"},
	{"product:mesh-traffic", "/api/product/mesh-traffic", "Service mesh traffic management & circuit breaker health auditor"},
	{"deployment:rollout-blocker", "/api/deployment/rollout-blocker", "Deployment rollout blocker & pod condition auditor"},
	{"security:pss-hardening", "/api/security/pss-hardening", "PSS enforcement gap & workload hardening auditor"},
	{"operations:node-trend", "/api/operations/node-trend", "Node condition trend & hardware failure prediction auditor"},
	{"product:endpoint-slice", "/api/product/endpoint-slice", "Endpoint slice health & topology-aware routing auditor"},
	{"deployment:surge-risk", "/api/deployment/surge-risk", "Rolling update risk & surge configuration analyzer"},
	{"scalability:saturation", "/api/scalability/saturation", "Resource saturation & CPU/memory throttling risk predictor"},
	{"operations:registry-rate-limit", "/api/operations/registry-rate-limit", "Container image registry rate limit & pull reliability auditor"},
	{"product:cert-manager", "/api/product/cert-manager", "Cert-manager health & certificate renewal pipeline auditor"},
	{"deployment:quota-impact", "/api/deployment/quota-impact", "Deployment resource quota impact & namespace deployment capacity auditor"},
	{"security:runtime-threat", "/api/security/runtime-threat", "Runtime threat detection & container anomaly auditor"},
	{"security:secret-posture", "/api/security/secret-posture", "Secret management posture & external secret integration auditor"},
	{"security:namespace-posture", "/api/security/namespace-posture", "Namespace security posture & trust boundary auditor"},
	{"security:image-provenance", "/api/security/image-provenance", "Container image provenance & registry trust auditor"},
	{"security:threat-timeline", "/api/security/threat-timeline", "Security event timeline & threat detection pattern auditor"},
	{"operations:cni-health", "/api/operations/cni-health", "CNI plugin health & network stack configuration auditor"},
	{"operations:observability-stack", "/api/operations/observability-stack", "Observability stack integration health auditor"},
	{"operations:operator-health", "/api/operations/operator-health", "Cluster operator & OLM health auditor"},
	{"operations:restart-storm", "/api/operations/restart-storm", "Pod restart pattern & crashloop clustering auditor"},
	{"operations:webhook-health", "/api/operations/webhook-health", "Admission webhook configuration health & performance risk auditor"},
	{"scalability:budget-alert", "/api/scalability/budget-alert", "Cost budget alert & namespace spending limit auditor"},
	{"scalability:node-drain-readiness", "/api/scalability/node-drain-readiness", "Node drain & rotation readiness auditor"},
	{"scalability:scaling-history", "/api/scalability/scaling-history", "Cluster scaling history & autoscaler event timeline auditor"},
	{"scalability:scheduling-fit", "/api/scalability/scheduling-fit", "Pod resource request density & scheduling fit auditor"},
	{"scalability:quota-saturation", "/api/scalability/quota-saturation", "Namespace resource quota saturation & limit exhaustion predictor"},
	{"product:ingress-tls", "/api/product/ingress-tls", "Ingress TLS certificate & HTTPS enforcement auditor"},
	{"product:east-west-traffic", "/api/product/east-west-traffic", "East-west traffic & service-to-service connectivity auditor"},
	{"product:port-exposure", "/api/product/port-exposure", "Container port exposure & named port consistency auditor"},
	{"product:endpoint-mismatch", "/api/product/endpoint-mismatch", "Service endpoint vs pod readiness mismatch auditor"},
	{"deployment:env-config-drift", "/api/deployment/env-config-drift", "Deployment env config drift & ConfigMap/Secret reference auditor"},
	{"deployment:traceability", "/api/deployment/traceability", "Deployment reproducibility & CI/CD traceability auditor"},
	{"deployment:termination-audit", "/api/deployment/termination-audit", "Pod termination message & exit code pattern auditor"},
	{"deployment:readiness-gate", "/api/deployment/readiness-gate", "Pod readiness gate compliance & custom condition auditor"},
	{"product:pv-access", "/api/product/pv-access", "PV access mode & multi-attach risk auditor"},
	{"deployment:dora-metrics", "/api/deployment/dora-metrics", "DORA metrics: deployment frequency, lead time, MTTR, change failure rate"},
	{"operations:apf-audit", "/api/operations/apf-audit", "API Priority & Fairness configuration auditor"},
	{"scalability:spot-readiness", "/api/scalability/spot-readiness", "Spot/preemptible instance readiness & cost optimization auditor"},
	{"product:traffic-policy", "/api/product/traffic-policy", "Service traffic policy & routing configuration auditor"},
	{"product:priority-preemption", "/api/product/priority-preemption", "Pod priority preemption & scheduling starvation risk analyzer"},
	{"deployment:concurrency-guard", "/api/deployment/concurrency-guard", "Deployment concurrency & rolling update collision detector"},
	{"operations:kube-proxy-health", "/api/operations/kube-proxy-health", "Kube-proxy & network routing stability auditor"},
	{"security:secret-age", "/api/security/secret-age", "Secret age & stale credential tracker"},
	{"scalability:ext-resource-health", "/api/scalability/ext-resource-health", "Extended resource & device plugin health auditor"},
	{"product:mesh-injection", "/api/product/mesh-injection", "Service mesh injection coverage & namespace adoption analyzer"},
	{"deployment:revision-diff", "/api/deployment/revision-diff", "Deployment revision diff & pod template change impact analyzer"},
	{"operations:coredns-health", "/api/operations/coredns-health", "CoreDNS configuration & resolution health auditor"},
	{"operations:incident-correlation", "/api/operations/incident-correlation", "Multi-signal incident correlation & root cause suggestion engine"},
	{"security:blast-radius", "/api/security/blast-radius", "Workload attack surface & blast radius analyzer"},
	{"scalability:reservation-audit", "/api/scalability/reservation-audit", "Node resource reservation & allocatable gap analyzer"},
	{"product:replica-distribution", "/api/product/replica-distribution", "Workload replica distribution & anti-affinity coverage analyzer"},
	{"deployment:daemonset-audit", "/api/deployment/daemonset-audit", "DaemonSet rollout & node coverage auditor"},
	{"scalability:capacity-plan", "/api/scalability/capacity-plan", "Capacity planning & growth trend predictor"},
	{"product:service-topology", "/api/product/service-topology", "Cluster-wide service dependency topology & cascade failure risk analyzer"},
	{"deployment:chaos-readiness", "/api/deployment/chaos-readiness", "Chaos engineering readiness assessment & experiment recommender"},
	{"scalability:carbon-footprint", "/api/scalability/carbon-footprint", "Cluster carbon footprint & sustainability metrics analyzer"},
	{"security:admission-policy-audit", "/api/security/admission-policy-audit", "Admission control policy gap & CEL expression auditor"},
	{"operations:pod-anomaly", "/api/operations/pod-anomaly", "Pod performance anomaly & noisy neighbor detector"},
	{"product:exposure-map", "/api/product/exposure-map", "Cluster external exposure surface risk map"},
	{"scalability:scale-simulator", "/api/scalability/scale-simulator", "Workload scaling impact simulator"},
	{"deployment:rollback-risk", "/api/deployment/rollback-risk", "Rollback risk & revision integrity assessor"},
	{"operations:pod-lifecycle", "/api/operations/pod-lifecycle", "Pod lifecycle stage analyzer & dwell-time tracker"},
	{"security:rbac-graph", "/api/security/rbac-graph", "RBAC permission graph & escalation path analyzer"},
	{"product:gateway-audit", "/api/product/gateway-audit", "Gateway API & ingress controller health audit"},
	{"scalability:cost-allocation", "/api/scalability/cost-allocation", "Namespace cost allocation & chargeback report"},
	{"deployment:gitops-audit", "/api/deployment/gitops-audit", "GitOps/CD pipeline health & config drift auditor"},
	{"operations:metrics-pipeline-audit", "/api/operations/metrics-pipeline-audit", "Metrics collection pipeline integrity audit"},
	{"security:compliance-map", "/api/security/compliance-map", "SOC2/PCI-DSS/HIPAA compliance framework mapping"},
	{"product:probe-effectiveness", "/api/product/probe-effectiveness", "Health probe effectiveness & failure detection analyzer"},
	{"scalability:node-upgrade-audit", "/api/scalability/node-upgrade-audit", "Node upgrade readiness & K8s version compatibility auditor"},
	{"operations:predictive-health", "/api/operations/predictive-health", "Cluster predictive health & risk forecast engine"},
	{"deployment:change-readiness", "/api/deployment/change-readiness", "Deployment change readiness pre-flight gate"},
	{"scalability:request-intelligence", "/api/scalability/request-intelligence", "Resource request intelligence & right-sizing engine"},
	{"product:reliability-scorecard", "/api/product/reliability-scorecard", "Per-workload reliability posture scorecard (A-F grading)"},
	{"security:posture-scorecard", "/api/security/posture-scorecard", "Cluster-wide security posture scorecard (A-F grading)"},
	{"operations:triage", "/api/operations/triage", "AIOps incident triage & remediation action plan engine"},
	{"deployment:impact-simulator", "/api/deployment/impact-simulator", "Deployment impact simulator & blast radius predictor"},
	{"scalability:cost-intelligence", "/api/scalability/cost-intelligence", "Cost intelligence, spend forecast & FinOps maturity scorecard"},
	{"product:golden-signals", "/api/product/golden-signals", "SRE four golden signals unified health engine"},
	{"security:remediation-matrix", "/api/security/remediation-matrix", "Security remediation priority & risk-effort matrix"},
	{"operations:mttr", "/api/operations/mttr", "Mean time to recovery & incident lifecycle analytics"},
	{"deployment:rollout-forensics", "/api/deployment/rollout-forensics", "Rollout failure forensics & deployment pattern detector"},
	{"deployment:resource-governance", "/api/deployment/resource-governance", "Resource governance & namespace quota effectiveness"},
	{"product:mesh-readiness", "/api/product/mesh-readiness", "Service mesh readiness & mTLS coverage gap analyzer"},
	{"scalability:idle-waste", "/api/scalability/idle-waste", "Idle resource waste quantification & cost recovery"},
	{"security:policy-governance", "/api/security/policy-governance", "Admission policy governance & enforcement auditor"},
	{"docs:api-quality", "/api/docs/api-quality", "Platform API endpoint quality & coverage gap analyzer"},
	{"operations:obs-cardinality", "/api/operations/obs-cardinality", "Observability data cardinality & volume cost analyzer"},
	{"scalability:autoscaling-intel", "/api/scalability/autoscaling-intel", "Autoscaling intelligence & scaling behavior profiler"},
	{"product:ownership-map", "/api/product/ownership-map", "Workload ownership & accountability governance engine"},
	{"docs:platform-maturity", "/api/docs/platform-maturity", "Platform maturity assessment & capability matrix"},
	{"security:compliance-posture", "/api/security/compliance-posture", "Multi-framework compliance posture & control mapping"},
	{"operations:obs-coverage", "/api/operations/obs-coverage", "Observability coverage & blind spot detector"},
	{"deployment:config-consistency", "/api/deployment/config-consistency", "Configuration consistency & standardization auditor"},
	{"scalability:scheduling-intel", "/api/scalability/scheduling-intel", "Scheduling intelligence & bin-packing efficiency analyzer"},
	{"product:dependency-resilience", "/api/product/dependency-resilience", "Service dependency resilience & cascade failure risk analyzer"},
	{"operations:change-intel", "/api/operations/change-intel", "Change intelligence & blast radius analyzer"},
	{"security:net-policy-effectiveness", "/api/security/net-policy-effectiveness", "Network policy effectiveness & zero-trust isolation scorer"},

	// --- Scalability ---
	{"scalability:overcommit", "/api/scalability/overcommit", "Resource over-commit & pressure"},
	{"scalability:autoscale-recommendations", "/api/scalability/autoscale-recommendations", "HPA/VPA right-sizing"},
	{"scalability:pvc-analysis", "/api/scalability/pvc-analysis", "PVC binding & storage performance"},
	{"scalability:storage-forecast", "/api/scalability/storage-forecast", "Storage capacity exhaustion predictor"},
	{"scalability:pod-density", "/api/scalability/pod-density", "Pod density & scheduling capacity analyzer"},
	{"scalability:ns-consumption", "/api/scalability/ns-consumption", "Namespace resource consumption & cost attribution"},
	{"scalability:capacity-headroom", "/api/scalability/capacity-headroom", "Cluster capacity headroom & scale-out readiness"},
	{"scalability:quota-utilization", "/api/scalability/quota-utilization", "Resource quota utilization & limit compliance"},
	{"scalability:ha-audit", "/api/scalability/ha-audit", "HA & single-point-of-failure detector"},
	{"scalability:node-failure-sim", "/api/scalability/node-failure-sim", "Node failure impact simulator"},
	{"scalability:crd-explosion", "/api/scalability/crd-explosion", "API object count & CRD explosion risk detector"},
	{"scalability:bottleneck-predictor", "/api/scalability/bottleneck-predictor", "K8s scalability bottleneck predictor"},
	{"scalability:namespace-isolation", "/api/scalability/namespace-isolation", "Namespace isolation & multi-tenancy audit"},
	{"scalability:csi-audit", "/api/scalability/csi-audit", "CSI driver & storage capability auditor"},
	{"scalability:scale-limits", "/api/scalability/scale-limits", "Cluster scalability limits & threshold monitor"},
	{"scalability:dr-readiness", "/api/scalability/dr-readiness", "Disaster recovery readiness & backup compliance auditor"},
	{"scalability:fragmentation", "/api/scalability/fragmentation", "Resource fragmentation & bin-packing efficiency analyzer"},
	{"scalability:ip-cidr-utilization", "/api/scalability/ip-cidr-utilization", "IP address & Pod CIDR utilization monitor"},
	{"scalability:node-topology", "/api/scalability/node-topology", "Node topology distribution & multi-AZ fault tolerance analyzer"},
	{"scalability:tenant-pressure", "/api/scalability/tenant-pressure", "Multi-tenant resource pressure & quota competition auditor"},
	{"scalability:node-pool-health", "/api/scalability/node-pool-health", "Node pool & cluster autoscaler health monitor"},
	{"scalability:cost-waste", "/api/scalability/cost-waste", "Idle resource cost waste & namespace cost attribution auditor"},
	{"scalability:node-lifecycle", "/api/scalability/node-lifecycle", "Node OS patch, kernel drift, GPU resources & node rotation auditor"},
	{"scalability:cost-intelligence", "/api/scalability/cost-intelligence", "Cost intelligence, spend forecast & FinOps maturity scorecard"},

	// --- Other audits ---
	{"certificates:expiry", "/api/certificates/expiry", "Certificate & TLS expiry monitor"},
	{"compatibility", "/api/compatibility", "Cluster compatibility & upgrade readiness"},
	{"pdbs", "/api/pdbs", "Pod Disruption Budget list"},
	{"images", "/api/images", "Container image inventory"},
	{"addons:health", "/api/addons/health", "Add-on health scan"},
	{"deployments:rollout", "/api/deployments/rollout", "Deployment rollout health"},
	{"resources:waste", "/api/resources/waste", "Resource waste detection"},
	{"resources:quota", "/api/resources/quota", "ResourceQuota & LimitRange monitor"},
	{"scaling:bottlenecks", "/api/scaling/bottlenecks", "Scaling bottleneck detection"},
	{"dependencies", "/api/dependencies", "Resource dependency graph & blast radius"},
	{"topology:spread", "/api/topology/spread", "Topology spread compliance"},
	{"networking:health", "/api/networking/health", "Service & endpoint health"},
	{"storage:health", "/api/storage/health", "PV/PVC storage health"},
	{"scheduling:health", "/api/scheduling/health", "Scheduling health & fragmentation"},
	{"deployments:audit", "/api/deployments/audit", "Deployment config audit"},
	{"efficiency", "/api/efficiency", "Resource efficiency analysis"},
	{"cost:summary", "/api/cost/summary", "Cost summary"},
	{"cost:recommendations", "/api/cost/recommendations", "Cost optimization recommendations"},

	// --- Cluster overview & diagnostics ---
	{"cluster:overview", "/api/cluster/overview", "Cluster overview: nodes, pods, resources, health summary"},
	{"cluster:diagnostics", "/api/diagnostics", "Run cluster diagnostics (restart analysis, common issues)"},
	{"cluster:diagnostics-restarts", "/api/diagnostics/restarts", "Pod restart diagnosis"},
	{"cluster:events", "/api/events", "Recent cluster events"},
	{"cluster:events-summary", "/api/events/summary", "Event summary & statistics"},
	{"cluster:audit-log", "/api/audit", "Audit log entries"},
	{"cluster:audit-stats", "/api/audit/stats", "Audit log statistics"},
	{"cluster:audit-events", "/api/audit/events", "Audit log events"},
	{"cluster:remediations", "/api/remediations", "Remediation actions list"},
	{"cluster:optimizations", "/api/optimizations", "Optimization suggestions list"},

	// --- Resource browser ---
	{"cluster:resources", "/api/resources", "List all resources across the cluster"},
	{"cluster:crds", "/api/crds", "List Custom Resource Definitions"},
	{"cluster:images-inventory", "/api/images", "Container image inventory (reuse if distinct from security:images)"},

	// --- Infrastructure ---
	{"infra:storage-capacity", "/api/storage/capacity", "Storage capacity & usage"},
	{"infra:capacity-planning", "/api/capacity/planning", "Capacity planning & recommendations"},
	{"infra:capacity-forecast", "/api/capacity/forecast", "Capacity forecast & exhaustion prediction"},
	{"infra:system-info", "/api/system/info", "System information (Go version, memory, goroutines)"},
	{"infra:api-performance", "/api/system/performance", "API performance metrics (p50/p95/p99 latency)"},
	{"infra:namespaces-ranking", "/api/namespaces/ranking", "Namespace ranking by resource usage"},
	{"infra:hpas", "/api/hpa", "Horizontal Pod Autoscaler list"},
}

// AuditTool lets the LLM agent run any registered cluster audit/analysis endpoint.
type AuditTool struct {
	DashboardAddr string // e.g. "localhost:9090"
}

func (t *AuditTool) Name() string { return "k8s_run_audit" }

func (t *AuditTool) Description() string {
	var names []string
	for _, a := range auditRegistry {
		names = append(names, a.name)
	}
	return fmt.Sprintf("Run a cluster audit or analysis tool. Returns structured JSON with findings, scores, and recommendations. "+
		"Available audit_name values: %s. "+
		"Use this to quickly get health scores, compliance status, risk analysis, and operational insights without manually inspecting individual resources.",
		strings.Join(names, ", "))
}

func (t *AuditTool) Parameters() map[string]any {
	names := make([]string, len(auditRegistry))
	for i, a := range auditRegistry {
		names[i] = a.name
	}
	return tools.Schema(map[string]tools.Property{
		"audit_name": {
			Type:        "string",
			Description: "The audit to run (see available values above)",
			Enum:        names,
		},
	}, []string{"audit_name"})
}

func (t *AuditTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	auditName, err := tools.GetString(args, "audit_name")
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Find the audit endpoint
	var endpoint string
	var desc string
	found := false
	for _, a := range auditRegistry {
		if a.name == auditName {
			endpoint = a.path
			desc = a.desc
			found = true
			break
		}
	}
	if !found {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown audit: %s. Use k8s_list_audits to see available options.", auditName),
		}, nil
	}

	// Build URL
	addr := t.DashboardAddr
	if addr == "" {
		addr = "localhost:9090"
	}
	url := fmt.Sprintf("http://%s%s", addr, endpoint)

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to create request: %v", err)}, nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to call audit endpoint: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: fmt.Sprintf("failed to read response: %v", err)}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &tools.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("audit returned HTTP %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	// Try to pretty-print if not already
	var pretty map[string]any
	if json.Unmarshal(body, &pretty) == nil {
		prettyBytes, _ := json.MarshalIndent(pretty, "", "  ")
		return &tools.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Audit: %s (%s)\n\n%s", auditName, desc, string(prettyBytes)),
		}, nil
	}

	return &tools.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Audit: %s (%s)\n\n%s", auditName, desc, string(body)),
	}, nil
}

// --- ListAuditsTool: list all available audits ---

type ListAuditsTool struct{}

func (t *ListAuditsTool) Name() string { return "k8s_list_audits" }
func (t *ListAuditsTool) Description() string {
	return "List all available cluster audit and analysis tools that can be run via k8s_run_audit. " +
		"Returns audit names, descriptions, and categories."
}
func (t *ListAuditsTool) Parameters() map[string]any {
	return tools.Schema(map[string]tools.Property{}, []string{})
}
func (t *ListAuditsTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	type auditInfo struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Desc     string `json:"description"`
	}

	audits := make([]auditInfo, 0, len(auditRegistry))
	for _, a := range auditRegistry {
		cat := "other"
		if idx := strings.Index(a.name, ":"); idx > 0 {
			cat = a.name[:idx]
		}
		audits = append(audits, auditInfo{Name: a.name, Category: cat, Desc: a.desc})
	}

	data, _ := json.MarshalIndent(map[string]any{
		"count":  len(audits),
		"audits": audits,
	}, "", "  ")
	return &tools.ToolResult{Success: true, Output: string(data)}, nil
}

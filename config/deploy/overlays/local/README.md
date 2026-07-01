# Local network overlay — actual environment values
#
# These values are for deploying k8ops to the local K8s cluster at 192.168.31.13.
# The local registry (registry.iot2.win) serves images built and pushed from this machine.
#
# Domain: k8ops.iot2.win (resolved via local DNS at 192.168.31.3)
# Registry: registry.iot2.win (192.168.31.50:5000)
# Ingress: traefik (pre-installed on k3s)
# TLS: cert-manager + letsencrypt (pre-installed)
#
# Usage:
#   # 1. Build and push image
#   make docker-build IMAGE_REPO=registry.iot2.win/k8ops IMAGE_TAG=v12.0
#   make docker-push  IMAGE_REPO=registry.iot2.win/k8ops IMAGE_TAG=v12.0
#
#   # 2. Create secrets
#   kubectl create namespace k8ops-system
#   kubectl create secret generic k8ops-auth \
#     --from-literal=jwt-secret="$(openssl rand -base64 32)" -n k8ops-system
#   kubectl create secret generic k8ops-credentials \
#     --from-literal=api-key="your-api-key" -n k8ops-system
#
#   # 3. Deploy
#   kubectl apply -k config/deploy/overlays/local
#
#   # 4. Verify
#   kubectl get pods -n k8ops-system
#   kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
#   # Open: https://k8ops.iot2.win  or  http://localhost:9090

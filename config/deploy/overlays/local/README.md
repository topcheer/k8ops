# Local Development Overlay
#
# Copy this directory to create custom overlays:
#   cp -r config/deploy/overlays/local config/deploy/overlays/myenv
#
# Quick start:
#   1. kubectl create namespace k8ops-system
#   2. kubectl create secret generic k8ops-auth \
#        --from-literal=jwt-secret="$(openssl rand -base64 32)" \
#        -n k8ops-system
#   3. kubectl create secret generic k8ops-credentials \
#        --from-literal=api-key="your-api-key" \
#        -n k8ops-system
#   4. kubectl apply -k config/deploy/overlays/local/

# Customize kustomization.yaml:
#   - k8ops.local → your domain (add to /etc/hosts or DNS)
#   - image tag (latest → specific version)
#   - AI provider settings (PROVIDER_TYPE/MODEL/ENDPOINT)

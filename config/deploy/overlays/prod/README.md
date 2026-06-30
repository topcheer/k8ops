# Example: Multi-domain overlay
# Supports multiple domains pointing to the same dashboard.
# Copy this directory and modify values as needed.
#
# To add extra TLS domains, patch the tls section:
# patches:
# - target:
#     kind: Ingress
#     name: k8ops-dashboard
#   patch: |-
#     - op: add
#       path: /spec/tls/0/hosts/-
#       value: k8ops-alt.example.com
#     - op: add
#       path: /spec/rules/-
#       value:
#         host: k8ops-alt.example.com
#         http:
#           paths:
#           - path: /
#             pathType: Prefix
#             backend:
#               service:
#                 name: k8ops-dashboard
#                 port:
#                   number: 9090

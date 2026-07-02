#!/bin/bash
# k8ops 备份脚本
# 用法: ./scripts/backup.sh [输出目录]
# 默认输出: ./k8ops-backup-YYYYMMDD/

set -euo pipefail

BACKUP_DIR="${1:-./k8ops-backup-$(date +%Y%m%d)}"
mkdir -p "$BACKUP_DIR"

echo "=== k8ops 备份开始 ==="
echo "输出目录: $BACKUP_DIR"
echo ""

# 1. 备份 DaemonSet 配置
echo "[1/6] 备份 DaemonSet..."
kubectl get daemonset k8ops -n k8ops-system -o yaml > "$BACKUP_DIR/daemonset.yaml"

# 2. 备份 ConfigMap 和 Secret
echo "[2/6] 备份 ConfigMap 和 Secret..."
kubectl get cm,secret -n k8ops-system -o yaml > "$BACKUP_DIR/configs-secrets.yaml"

# 3. 备份 RBAC
echo "[3/6] 备份 RBAC..."
kubectl get clusterrole,clusterrolebinding -o yaml 2>/dev/null | \
  awk '/^---/{found=0} /k8ops/{found=1} found{print}' > "$BACKUP_DIR/rbac.yaml" || true

# 4. 备份 CRD 数据
echo "[4/6] 备份 CRD 数据..."
kubectl get diagnostics -A -o yaml > "$BACKUP_DIR/diagnostics.yaml" 2>/dev/null || echo "  (no diagnostics)"
kubectl get remediations -A -o yaml > "$BACKUP_DIR/remediations.yaml" 2>/dev/null || echo "  (no remediations)"
kubectl get optimizations -A -o yaml > "$BACKUP_DIR/optimizations.yaml" 2>/dev/null || echo "  (no optimizations)"

# 5. 备份 Provider 配置
echo "[5/6] 备份 Provider 配置..."
kubectl get providers -A -o yaml > "$BACKUP_DIR/providers.yaml" 2>/dev/null || echo "  (no providers)"

# 6. 备份命名空间和 Service
echo "[6/6] 备份命名空间和 Service..."
kubectl get ns k8ops-system -o yaml > "$BACKUP_DIR/namespace.yaml"
kubectl get svc -n k8ops-system -o yaml > "$BACKUP_DIR/services.yaml"

echo ""
echo "=== 备份完成 ==="
echo "文件列表:"
ls -lh "$BACKUP_DIR/"
echo ""
echo "备份大小: $(du -sh "$BACKUP_DIR/" | cut -f1)"
echo ""
echo "恢复命令: kubectl apply -f $BACKUP_DIR/"

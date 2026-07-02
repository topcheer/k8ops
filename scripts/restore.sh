#!/bin/bash
# k8ops 恢复脚本
# 用法: ./scripts/restore.sh <备份目录>
# 例如: ./scripts/restore.sh ./k8ops-backup-20260702/

set -euo pipefail

BACKUP_DIR="${1:-}"

if [ -z "$BACKUP_DIR" ]; then
  echo "用法: $0 <备份目录>"
  echo "例如: $0 ./k8ops-backup-20260702/"
  exit 1
fi

if [ ! -d "$BACKUP_DIR" ]; then
  echo "错误: 备份目录不存在: $BACKUP_DIR"
  exit 1
fi

echo "=== k8ops 恢复开始 ==="
echo "备份目录: $BACKUP_DIR"
echo ""

# 按正确顺序恢复
RESTORE_ORDER=(
  "namespace.yaml:命名空间"
  "rbac.yaml:RBAC 权限"
  "configs-secrets.yaml:ConfigMap 和 Secret"
  "services.yaml:Service"
  "daemonset.yaml:DaemonSet"
  "providers.yaml:Provider 配置"
  "diagnostics.yaml:诊断数据"
  "remediations.yaml:修复数据"
  "optimizations.yaml:优化数据"
)

for entry in "${RESTORE_ORDER[@]}"; do
  file="${entry%%:*}"
  label="${entry##*:}"
  if [ -f "$BACKUP_DIR/$file" ]; then
    echo "[$label] 恢复 $file..."
    kubectl apply -f "$BACKUP_DIR/$file" 2>/dev/null || echo "  (跳过，可能已存在)"
  else
    echo "[$label] 跳过 (文件不存在)"
  fi
done

echo ""
echo "=== 恢复完成 ==="
echo "验证:"
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# 变更日志

所有重要变更记录在此文件中。版本号遵循语义化版本规范。

---

## v15.20 (2026-07-05)

### 新增
- **Network Policy 合规与流量隔离审计器** (`GET /api/product/network-policy`)
  - 按命名空间分析 NetworkPolicy 覆盖率和流量隔离
  - 未保护 Pod 检测、宽松出站检测 (0.0.0.0/0)
  - 集群隔离评分 (0-100)
  - 7 个单元测试

## v15.19 (2026-07-05)

### 新增
- **Deployment 滚动更新策略与健康分析器** (`GET /api/deployment/rollout-health`)
  - 滚动更新策略分析 (RollingUpdate vs Recreate)
  - 卡住部署检测 (Progressing=False / ReplicaFailure / 超时)
  - 回滚就绪评估 (revisionHistoryLimit)
  - 集群滚动更新健康评分 (0-100)
  - 7 个单元测试

## v15.18 (2026-07-05)

### 新增
- **命名空间资源消耗与成本归属** (`GET /api/scalability/ns-consumption`)
  - 按命名空间聚合 CPU/内存/存储消耗
  - 月成本估算 ($28/核 CPU, $3.8/GB 内存, $0.10/GB 存储)
  - 浪费分析：过度配置、空闲命名空间、浪费评分
  - Top 10 消费者排行
  - 5 个单元测试

## v15.17 (2026-07-05)

### 新增
- **PDB 合规与自愿中断风险分析器** (`GET /api/operations/pdb-audit`)
  - PDB 状态分类：healthy / blocked / impossible
  - 未保护多副本部署检测
  - 节点排空模拟（逐节点 PDB 阻塞分析）
  - 集群 PDB 覆盖评分 (0-100)
  - 8 个单元测试

## v15.16 (2026-07-05)

### 新增
- **证书与 TLS 过期监控器** (`GET /api/security/cert-expiry`)
  - PEM 证书解析 (CN, SANs, 颁发者, 有效期, 密钥大小)
  - 自签名检测、Pod 引用追踪
  - 过期分级：critical (<30d) / high (<60d) / medium (<90d) / low (>90d)
  - 集群证书健康评分 (0-100)
  - 9 个单元测试

## v15.15 (2026-07-05)

### 新增
- **多语言文档** — 7 种语言，76 个文件
  - English, 中文, 日本語, 한국어, Español, Français, Deutsch
  - 每语言 10 篇文档 + README
  - 母语级 Review 完成

## v15.14 (2026-07-05)

### 新增
- **ConfigMap & Secret 配置审计器** (`GET /api/product/config-audit`)
  - ConfigMap：超大检测 (>1MB)、未引用检测、空数据键、不可变标志
  - Secret：过期凭证 (>180d)、未引用、明文凭证键检测、轮换建议
  - 交叉引用引擎 (Pod volumes, env, envFrom, projected sources)
  - 集群配置审计健康评分 (0-100)
  - 7 个单元测试

## v15.13 (2026-07-05)

### 新增
- **容器镜像部署规范分析器** (`GET /api/deployment/image-hygiene`)
  - 镜像标签策略分析 (版本号 / :latest / @sha256 摘要锁定)
  - 重复镜像检测、仓库信任分级
  - 集群镜像规范评分 (0-100)
  - 8 个单元测试

---

## 统计信息

| 指标 | 数值 |
|------|------|
| OpenAPI 端点 | 108 |
| 单元测试 | 692 |
| 文档 | 12 篇 (7 种语言) |
| i18n 文件 | 76 个 |
| Release Assets | 17 个 |
| 镜像大小 | 28.6MB (distroless) |
| Go 版本 | 1.26+ |

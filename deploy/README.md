# Online Boutique + MySQL 中间件集成

基于 Google [microservices-demo](https://github.com/GoogleCloudPlatform/microservices-demo) 的扩展版本，增加了 MySQL 中间件、一键部署脚本、Prometheus 监控和 Chaos Mesh 故障注入能力。

## 架构概览

```
┌──────────────┐     gRPC      ┌──────────────────────────┐     SQL      ┌───────────────┐
│   frontend   │──────────────▶│  productcatalogservice   │────────────▶│               │
│   (Go)       │               │  (Go, MySQL-enabled)     │             │  MySQL 8.0    │
└──────────────┘               └──────────────────────────┘             │  (boutique)   │
                                                                        │               │
┌──────────────┐     HTTP      ┌──────────────────────────┐     SQL     │  12 tables    │
│ order-loadgen│──────────────▶│     orderservice         │────────────▶│  ~600K rows   │
│ (Python)     │               │  (Python Flask)          │             │               │
└──────────────┘               └──────────────────────────┘             └───────┬───────┘
                                                                                │
                               ┌──────────────────────────┐                     │
                               │   mysqld-exporter        │─────────────────────┘
                               │   (Prometheus metrics)   │
                               └────────────┬─────────────┘
                                            │ /metrics
                               ┌────────────▼─────────────┐
                               │      Prometheus          │
                               │  (kube-prometheus-stack) │
                               └────────────┬─────────────┘
                                            │
                               ┌────────────▼─────────────┐
                               │        Grafana           │
                               └──────────────────────────┘
```

### 原有服务 (11个)

frontend, cartservice, checkoutservice, currencyservice, emailservice, paymentservice, productcatalogservice, recommendationservice, shippingservice, adservice, loadgenerator

### 新增组件

| 组件 | 说明 |
|------|------|
| **mysql-boutique** | MySQL 8.0, 12张表, ~60万行数据, 慢查询日志开启 |
| **productcatalogservice** (改造) | 实时查询 MySQL 获取商品数据, MySQL 不可用时回退到 JSON |
| **orderservice** | Python Flask, 订单CRUD + 慢查询分析接口 |
| **order-loadgenerator** | 持续创建订单, 定期触发慢查询 |
| **mysqld-exporter** | Prometheus 指标采集, ServiceMonitor 自动发现 |

## 前置条件

- Docker Desktop / OrbStack (本地 K8s)
- kubectl
- Helm 3
- Go 1.25+ (构建镜像时需要)

## 快速部署

```bash
# 一键启动 (含微服务、监控、MySQL、Chaos Mesh)
cd deploy
./up.sh

# 一键停止并清理
./down.sh
```

启动后可访问:

| 服务 | 地址 | 说明 |
|------|------|------|
| Frontend | http://localhost:9000 | 商城前端 |
| Grafana | http://localhost:9001 | 监控面板 (admin/admin) |
| Chaos Mesh | http://localhost:9002 | 故障注入控制台 |
| MySQL | mysql-boutique:3306 (集群内) | boutique/boutique123 |
| OrderService | orderservice:8080 (集群内) | 订单服务 |

## 目录结构

```
.
├── src/
│   └── productcatalogservice/    # Go 源码 (已集成 MySQL 查询)
│       ├── server.go             # MySQL 连接池初始化, 重试逻辑
│       ├── product_catalog.go    # ListProducts/GetProduct/SearchProducts MySQL 实现
│       └── catalog_loader.go     # MySQL/AlloyDB/JSON 三级加载策略
├── deploy/
│   ├── up.sh                     # 一键启动
│   ├── down.sh                   # 一键停止
│   ├── build_images.sh           # 构建 productcatalogservice:mysql-v1 镜像
│   ├── health_check.sh           # 前端健康检查
│   ├── monitoring_install.sh     # kube-prometheus-stack 安装
│   ├── monitoring_port_forward.sh
│   ├── monitoring_values.yaml    # Prometheus/Grafana 配置 (含 OrbStack 兼容修复)
│   ├── chaos_mesh_install.sh     # Chaos Mesh 安装
│   ├── chaos_mesh_port_forward.sh
│   ├── chaos_mesh_uninstall.sh
│   └── mysql/                    # MySQL 相关 K8s 资源
│       ├── mysql-init-schema.yaml    # 12张表 DDL
│       ├── mysql-init-data.yaml      # 种子数据 (9个商品, 9个分类)
│       ├── mysql-init-generate.yaml  # 批量数据生成 (~60万行)
│       ├── mysql-deployment.yaml     # MySQL Deployment + Service + Secret
│       ├── mysql-exporter.yaml       # mysqld-exporter + ServiceMonitor
│       ├── order-service.yaml        # orderservice Deployment + ConfigMap
│       ├── order-loadgen.yaml        # 流量生成器
│       └── productcatalog-override.yaml  # 替换为 MySQL 版本的 Deployment
└── release/
    └── kubernetes-manifests.yaml # 原始微服务清单
```

## 数据库设计

### 表清单 (12张)

| 表名 | 行数 | 用途 |
|------|------|------|
| products | 9 | 商品信息 (与 products.json 对应) |
| categories | 9 | 商品分类 |
| product_categories | 12 | 商品-分类多对多关系 |
| users | 1,000 | 用户 |
| user_addresses | 2,000 | 用户地址 |
| orders | 50,000 | 订单 |
| order_items | 200,000 | 订单明细 |
| payments | 50,000 | 支付记录 |
| shipping_records | 50,000 | 物流记录 |
| coupons | 200 | 优惠券 |
| product_reviews | 50,000 | 商品评价 |
| audit_logs | 200,000 | 审计日志 |

### 慢查询设计

`/api/analytics/product-sales` 接口执行 11 个关联子查询, 跨越 order_items(20万行)、orders(5万行)、product_reviews(5万行)、audit_logs(20万行)、shipping_records(5万行)、payments(5万行) 等表。

关键设计: **故意不在外键列 (user_id, product_id, order_id, entity_id, status) 上建索引**, 使每个子查询都需要全表扫描, 自然达到 >2s 的执行时间。

MySQL 慢查询日志已开启 (`long_query_time=1`), 可通过以下方式查看:

```bash
kubectl exec deploy/mysql-boutique -n boutique -- cat /var/log/mysql/slow.log
```

## productcatalogservice 改造说明

### MySQL 连接

通过环境变量控制 (`MYSQL_ADDR`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`):
- 设置 `MYSQL_ADDR` 时启用 MySQL 模式, 启动时带 60 次重试 (2s 间隔)
- 未设置或连接失败时回退到原始 JSON 目录

### 查询链路

```
gRPC ListProducts/GetProduct/SearchProducts
  → mysqlDB != nil?
    → Yes: 查询 MySQL products + categories 表
      → 失败: 回退到 JSON catalog
    → No: 使用 JSON catalog (products.json)
```

### 本地镜像构建

```bash
cd deploy
./build_images.sh
# 产出: productcatalogservice:mysql-v1 (基于 distroless/static, ~26MB)
```

productcatalog-override.yaml 使用 `imagePullPolicy: IfNotPresent` 引用本地镜像。

## 监控

### Prometheus 指标

mysqld-exporter 通过 ServiceMonitor 被 Prometheus 自动采集, 可用指标包括:

- `mysql_global_status_threads_connected` - 当前连接数
- `mysql_global_status_slow_queries` - 慢查询计数
- `mysql_global_status_questions` - 总查询数
- `mysql_global_status_innodb_buffer_pool_reads` - InnoDB 缓冲池读取
- 更多指标见 `curl <exporter-pod>:9104/metrics`

### 验证采集

```bash
# 检查 Prometheus target 状态
kubectl port-forward svc/kube-prometheus-stack-prometheus 9090:9090 -n monitoring
# 访问 http://localhost:9090/targets, 查看 mysql-exporter job 状态为 UP
```

### OrbStack 兼容性

`monitoring_values.yaml` 包含 OrbStack 环境下 cAdvisor 空标签的修复:
- `image=""` → 替换为 `orbstack` 占位符
- `container=""` → 从 cgroup id 提取容器名

## 部署流程详解

`up.sh` 执行顺序:

1. 创建 `boutique` namespace
2. 部署原始 11 个微服务 (`kubernetes-manifests.yaml`)
3. 等待 frontend pod ready, 设置 port-forward (9000)
4. 安装 kube-prometheus-stack (Helm), 设置 Grafana port-forward (9001)
5. 构建 `productcatalogservice:mysql-v1` Docker 镜像
6. 部署 MySQL (ConfigMap → Deployment, 等待 ready, 含数据初始化)
7. 用 MySQL 版本替换 productcatalogservice
8. 部署 orderservice + order-loadgenerator + mysqld-exporter
9. 安装 Chaos Mesh (Helm), 设置 Dashboard port-forward (9002)

`down.sh` 按相反顺序清理所有资源和 namespace。

## 常用运维命令

```bash
# 查看所有 pod 状态
kubectl get pods -n boutique

# 查看 productcatalogservice 是否连接 MySQL
kubectl logs deploy/productcatalogservice -n boutique | head -10

# 手动触发慢查询
kubectl port-forward svc/orderservice 8080:8080 -n boutique
curl http://localhost:8080/api/analytics/product-sales

# 查看 MySQL 表行数
kubectl exec deploy/mysql-boutique -n boutique -- \
  mysql -uboutique -pboutique123 boutique -e \
  "SELECT TABLE_NAME, TABLE_ROWS FROM information_schema.TABLES WHERE TABLE_SCHEMA='boutique';"

# 查看 loadgen 日志
kubectl logs deploy/order-loadgenerator -n boutique --tail=20

# 查看 MySQL 慢查询日志
kubectl exec deploy/mysql-boutique -n boutique -- tail -50 /var/log/mysql/slow.log
```

## 扩展方向

- **Redis 中间件**: 为 productcatalogservice 增加 Redis 缓存层
- **消息队列**: 引入 Kafka/RabbitMQ 处理订单异步流程
- **更多故障场景**: 利用 Chaos Mesh 注入 MySQL 网络延迟、Pod 故障等
- **Grafana Dashboard**: 导入 MySQL Overview dashboard (ID: 7362) 可视化监控
- **数据规模调整**: 修改 `mysql-init-generate.yaml` 中的 WHERE 条件调整数据量

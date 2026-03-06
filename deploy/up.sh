#!/bin/bash

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="boutique"

echo "Creating namespace ${NAMESPACE}..."
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

echo "Deploying microservices-demo to Kubernetes (namespace: ${NAMESPACE})..."
kubectl apply -f https://raw.githubusercontent.com/yriiolik/microservices-demo/main/release/kubernetes-manifests.yaml -n ${NAMESPACE}
echo "Waiting for deployments to be created..."
sleep 5
echo "Waiting for all pods to be ready..."
kubectl wait --for=condition=ready pod -l app=frontend -n ${NAMESPACE} --timeout=300s

echo "Exposing frontend service on port 9000..."
# 检查端口是否被占用，如果有残留的 port-forward 进程，先干掉
if [ -f "${SCRIPT_DIR}/.port_forward.pid" ]; then
    kill $(cat "${SCRIPT_DIR}/.port_forward.pid") 2>/dev/null || true
fi
kubectl port-forward svc/frontend-external 9000:80 -n ${NAMESPACE} > /dev/null 2>&1 &
echo $! > "${SCRIPT_DIR}/.port_forward.pid"

echo "Deployment completed successfully!"
echo "Frontend is accessible at http://localhost:9000"

# One-click monitoring setup
echo "Starting monitoring setup..."
"${SCRIPT_DIR}/monitoring_install.sh"
"${SCRIPT_DIR}/monitoring_port_forward.sh"

# ============================================================
# MySQL stack deployment (after monitoring so ServiceMonitor CRD exists)
# ============================================================
echo ""
echo "Starting MySQL stack deployment..."

# Build productcatalogservice with MySQL support
echo "Building productcatalogservice with MySQL support..."
bash "${SCRIPT_DIR}/build_images.sh"

# Deploy MySQL and related services
echo "Deploying MySQL stack to namespace ${NAMESPACE}..."
kubectl apply -f "${SCRIPT_DIR}/mysql/mysql-init-schema.yaml" -n ${NAMESPACE}
kubectl apply -f "${SCRIPT_DIR}/mysql/mysql-init-data.yaml" -n ${NAMESPACE}
kubectl apply -f "${SCRIPT_DIR}/mysql/mysql-init-generate.yaml" -n ${NAMESPACE}
kubectl apply -f "${SCRIPT_DIR}/mysql/mysql-deployment.yaml" -n ${NAMESPACE}

echo "Waiting for MySQL to be ready (this may take a few minutes for data initialization)..."
kubectl wait --for=condition=ready pod -l app=mysql-boutique -n ${NAMESPACE} --timeout=600s

# Override productcatalogservice with MySQL-enabled version
echo "Deploying MySQL-enabled productcatalogservice..."
kubectl apply -f "${SCRIPT_DIR}/mysql/productcatalog-override.yaml" -n ${NAMESPACE}
echo "Waiting for productcatalogservice to restart..."
kubectl rollout status deployment/productcatalogservice -n ${NAMESPACE} --timeout=120s

# Deploy order service, load generator, and MySQL exporter
echo "Deploying orderservice, load generator, and MySQL exporter..."
kubectl apply -f "${SCRIPT_DIR}/mysql/order-service.yaml" -n ${NAMESPACE}
kubectl apply -f "${SCRIPT_DIR}/mysql/order-loadgen.yaml" -n ${NAMESPACE}
kubectl apply -f "${SCRIPT_DIR}/mysql/mysql-exporter.yaml" -n ${NAMESPACE}

echo "MySQL stack deployed successfully!"

# One-click Chaos Mesh setup
echo "Starting Chaos Mesh setup..."
"${SCRIPT_DIR}/chaos_mesh_install.sh"
"${SCRIPT_DIR}/chaos_mesh_port_forward.sh"

echo ""
echo "========== All services ready =========="
echo "Frontend:             http://localhost:9000"
echo "Grafana:              http://localhost:9001  (admin / admin)"
echo "Chaos Mesh Dashboard: http://localhost:9002"
echo "MySQL:                mysql-boutique:3306 (internal, boutique/boutique123)"
echo "OrderService:         orderservice:8080 (internal)"
echo "========================================"

#!/bin/bash

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="boutique"

echo "Stopping microservices-demo environment..."

if [ -f "${SCRIPT_DIR}/.port_forward.pid" ]; then
    echo "Stopping port-forward..."
    kill $(cat "${SCRIPT_DIR}/.port_forward.pid") 2>/dev/null || true
    rm -f "${SCRIPT_DIR}/.port_forward.pid"
fi

if [ -f "${SCRIPT_DIR}/.grafana_port_forward.pid" ]; then
    echo "Stopping Grafana port-forward..."
    kill $(cat "${SCRIPT_DIR}/.grafana_port_forward.pid") 2>/dev/null || true
    rm -f "${SCRIPT_DIR}/.grafana_port_forward.pid"
fi

if [ -f "${SCRIPT_DIR}/.chaos_dashboard_port_forward.pid" ]; then
    echo "Stopping Chaos Mesh Dashboard port-forward..."
    kill $(cat "${SCRIPT_DIR}/.chaos_dashboard_port_forward.pid") 2>/dev/null || true
    rm -f "${SCRIPT_DIR}/.chaos_dashboard_port_forward.pid"
fi

# Cleanup Chaos Mesh if installed
if kubectl get namespace chaos-mesh >/dev/null 2>&1; then
    echo "Cleaning up Chaos Mesh..."
    "${SCRIPT_DIR}/chaos_mesh_uninstall.sh"
fi

# Delete the kubernetes manifests
echo "Deleting microservices-demo resources..."
kubectl delete -f https://raw.githubusercontent.com/yriiolik/microservices-demo/main/release/kubernetes-manifests.yaml -n ${NAMESPACE} --ignore-not-found

# Cleanup MySQL stack
echo "Cleaning up MySQL stack..."
kubectl delete -f "${SCRIPT_DIR}/mysql/" -n ${NAMESPACE} --ignore-not-found 2>/dev/null || true

# Delete boutique namespace
echo "Deleting namespace ${NAMESPACE}..."
kubectl delete namespace ${NAMESPACE} --ignore-not-found

# Cleanup monitoring if installed
if kubectl get namespace monitoring >/dev/null 2>&1; then
    echo "Cleaning up monitoring stack..."
    helm uninstall kube-prometheus-stack -n monitoring || true
    kubectl delete namespace monitoring --ignore-not-found
fi

echo "Environment stopped and cleaned up."

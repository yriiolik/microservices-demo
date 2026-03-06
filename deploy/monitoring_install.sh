#!/bin/bash
set -e

NAMESPACE="monitoring"
RELEASE_NAME="kube-prometheus-stack"

echo "Redirecting Helm home to a directory with write permissions..."
export HELM_CACHE_HOME="/tmp/helm-cache"
export HELM_CONFIG_HOME="/tmp/helm-config"
export HELM_DATA_HOME="/tmp/helm-data"
mkdir -p $HELM_CACHE_HOME $HELM_CONFIG_HOME $HELM_DATA_HOME

echo "Creating namespace $NAMESPACE..."
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Skipping helm repo update due to local environment network/permission constraints

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

echo "Installing kube-prometheus-stack via Helm..."
# Using local values file to fix label mismatches and ensure metrics collection
helm upgrade --install $RELEASE_NAME https://github.com/prometheus-community/helm-charts/releases/download/kube-prometheus-stack-69.8.2/kube-prometheus-stack-69.8.2.tgz \
  --namespace $NAMESPACE \
  --values "${SCRIPT_DIR}/monitoring_values.yaml"

echo "Waiting for monitoring components to be ready..."
kubectl wait --for=condition=ready pod -l release=$RELEASE_NAME -n $NAMESPACE --timeout=600s

echo "Monitoring stack installed successfully! ✅"
echo "Grafana credentials: admin / admin"

#!/bin/bash
set -e

NAMESPACE="chaos-mesh"
RELEASE_NAME="chaos-mesh"
CHART_VERSION="2.8.1"

echo "Redirecting Helm home to a directory with write permissions..."
export HELM_CACHE_HOME="/tmp/helm-cache"
export HELM_CONFIG_HOME="/tmp/helm-config"
export HELM_DATA_HOME="/tmp/helm-data"
mkdir -p $HELM_CACHE_HOME $HELM_CONFIG_HOME $HELM_DATA_HOME

echo "Creating namespace $NAMESPACE..."
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

echo "Adding Chaos Mesh Helm repo..."
helm repo add chaos-mesh https://charts.chaos-mesh.org 2>/dev/null || true

echo "Installing Chaos Mesh via Helm..."
helm upgrade --install $RELEASE_NAME chaos-mesh/chaos-mesh \
  --namespace $NAMESPACE \
  --set chaosDaemon.runtime=docker \
  --set chaosDaemon.socketPath=/var/run/docker.sock \
  --set dashboard.securityMode=false \
  --version $CHART_VERSION

echo "Waiting for Chaos Mesh components to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE --timeout=300s

echo "Chaos Mesh installed successfully!"
echo "Components:"
kubectl get pods -n $NAMESPACE

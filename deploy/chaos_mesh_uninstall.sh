#!/bin/bash

NAMESPACE="chaos-mesh"
RELEASE_NAME="chaos-mesh"

export HELM_CACHE_HOME="/tmp/helm-cache"
export HELM_CONFIG_HOME="/tmp/helm-config"
export HELM_DATA_HOME="/tmp/helm-data"

echo "Uninstalling Chaos Mesh..."

# Step 1: Delete all Chaos experiments across all namespaces
echo "Step 1: Cleaning up Chaos experiments..."
for crd in $(kubectl get crd -o name 2>/dev/null | grep chaos-mesh.org); do
    crd_name=$(echo "$crd" | cut -d/ -f2)
    kubectl get "$crd_name" --all-namespaces -o custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name --no-headers 2>/dev/null | while read ns name; do
        if [ -n "$ns" ] && [ -n "$name" ]; then
            echo "  Deleting $crd_name/$name in namespace $ns..."
            kubectl delete "$crd_name" "$name" -n "$ns" --ignore-not-found 2>/dev/null
        fi
    done
done

# Step 2: Uninstall Helm release
echo "Step 2: Uninstalling Helm release..."
helm uninstall $RELEASE_NAME -n $NAMESPACE 2>/dev/null || true

# Step 3: Delete CRDs
echo "Step 3: Deleting Chaos Mesh CRDs..."
kubectl get crd -o name 2>/dev/null | grep chaos-mesh.org | xargs -r kubectl delete 2>/dev/null || true

# Step 4: Delete namespace
echo "Step 4: Deleting namespace $NAMESPACE..."
kubectl delete namespace $NAMESPACE --ignore-not-found

echo "Chaos Mesh uninstalled successfully!"

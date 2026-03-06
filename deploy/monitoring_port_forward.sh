#!/bin/bash

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="monitoring"
PORT=9001

echo "Exposing Grafana on port $PORT..."

# Kill existing port-forward if running
if [ -f "${SCRIPT_DIR}/.grafana_port_forward.pid" ]; then
    kill $(cat "${SCRIPT_DIR}/.grafana_port_forward.pid") 2>/dev/null || true
fi

kubectl port-forward svc/kube-prometheus-stack-grafana -n $NAMESPACE $PORT:80 > /dev/null 2>&1 &
echo $! > "${SCRIPT_DIR}/.grafana_port_forward.pid"

echo "Grafana is accessible at http://localhost:$PORT"
echo "Username: admin"
echo "Password: admin"

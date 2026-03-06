#!/bin/bash

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
NAMESPACE="chaos-mesh"
PORT=9002

echo "Exposing Chaos Mesh Dashboard on port $PORT..."

# Kill existing port-forward if running
if [ -f "${SCRIPT_DIR}/.chaos_dashboard_port_forward.pid" ]; then
    kill $(cat "${SCRIPT_DIR}/.chaos_dashboard_port_forward.pid") 2>/dev/null || true
fi

kubectl port-forward svc/chaos-dashboard -n $NAMESPACE $PORT:2333 > /dev/null 2>&1 &
echo $! > "${SCRIPT_DIR}/.chaos_dashboard_port_forward.pid"

echo "Chaos Mesh Dashboard is accessible at http://localhost:$PORT"

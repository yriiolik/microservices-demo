#!/bin/bash

echo "Starting health check for microservices-demo on port 9000..."

# Wait up to 60 seconds for the frontend to return 200 OK (it might return 500 initially if backends are still starting)
MAX_RETRIES=30
RETRY_INTERVAL=5

echo "Checking frontend interface..."

SUCCESS=0
for i in $(seq 1 $MAX_RETRIES); do
    HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 http://localhost:9000/)
    
    if [ "$HTTP_STATUS" == "200" ]; then
        echo "✅ Health check passed: Frontend responded with 200 OK."
        SUCCESS=1
        break
    else
        echo "⏳ Frontend returned HTTP Status $HTTP_STATUS. Waiting for backend services... ($i/$MAX_RETRIES)"
        sleep $RETRY_INTERVAL
    fi
done

if [ "$SUCCESS" -eq 1 ]; then
    exit 0
else
    echo "❌ Health check failed after maximum retries."
    exit 1
fi

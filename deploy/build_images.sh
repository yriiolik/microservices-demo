#!/bin/bash
set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_DIR=$(cd "${SCRIPT_DIR}/.." && pwd)
IMAGE_NAME="productcatalogservice:mysql-v1"

echo "=========================================="
echo " Building productcatalogservice with MySQL"
echo "=========================================="

SRC_DIR="${REPO_DIR}/src/productcatalogservice"

if [ ! -d "${SRC_DIR}" ]; then
    echo "ERROR: source directory not found at ${SRC_DIR}"
    exit 1
fi

# Ensure MySQL driver dependency exists
echo "Checking MySQL driver dependency..."
cd "${SRC_DIR}"
if ! grep -q "go-sql-driver/mysql" go.mod; then
    go get github.com/go-sql-driver/mysql@v1.8.1
    go mod tidy
fi

# Build Docker image
echo "Building Docker image: ${IMAGE_NAME}..."
docker build -t "${IMAGE_NAME}" .

# Verify
echo "Verifying image..."
docker image inspect "${IMAGE_NAME}" > /dev/null 2>&1
echo ""
echo "Successfully built ${IMAGE_NAME}"
echo "Image details:"
docker images "${IMAGE_NAME}" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}"

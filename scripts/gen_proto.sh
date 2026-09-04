#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROTO_DIR="${ROOT_DIR}/proto"
GO_OUT_DIR="${ROOT_DIR}/go-services/shared/pb"
PY_OUT_DIR="${ROOT_DIR}/python-ml/shared/pb"

echo "=== Aegis Protobuf Generator ==="
echo "Proto dir: ${PROTO_DIR}"
echo "Go output: ${GO_OUT_DIR}"
echo "Python output: ${PY_OUT_DIR}"

mkdir -p "${GO_OUT_DIR}"
mkdir -p "${PY_OUT_DIR}"

# Check for protoc
if command -v protoc >/dev/null 2>&1; then
    echo "Generating Go stubs..."
    protoc -I="${PROTO_DIR}" \
        --go_out="${ROOT_DIR}/go-services" \
        --go_opt=module=github.com/aegis/go-services \
        --go-grpc_out="${ROOT_DIR}/go-services" \
        --go-grpc_opt=module=github.com/aegis/go-services \
        "${PROTO_DIR}"/*.proto || echo "Warning: protoc go generation had warnings or failed"
fi

# Check for python grpc_tools
if python -m grpc_tools.protoc --version >/dev/null 2>&1; then
    echo "Generating Python stubs..."
    python -m grpc_tools.protoc \
        -I="${PROTO_DIR}" \
        --python_out="${PY_OUT_DIR}" \
        --grpc_python_out="${PY_OUT_DIR}" \
        "${PROTO_DIR}"/*.proto || echo "Warning: python grpc_tools generation had warnings or failed"
    touch "${PY_OUT_DIR}/__init__.py"
else
    echo "Notice: python grpc_tools not installed, skipping Python stub generation."
fi

echo "Proto generation script completed."

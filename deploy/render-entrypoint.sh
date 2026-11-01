#!/bin/sh
set -eu
python /app/python-ml/serving/grpc_server.py &
exec /usr/local/bin/aegis-gateway

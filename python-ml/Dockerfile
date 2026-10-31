FROM python:3.11-slim AS proto-builder

WORKDIR /build
RUN python -m venv /opt/proto-venv
ENV PATH="/opt/proto-venv/bin:$PATH"
COPY python-ml/requirements-serve.txt python-ml/requirements-train.txt /tmp/
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --disable-pip-version-check --no-warn-script-location \
    grpcio-tools==1.68.1
COPY proto /build/proto
RUN mkdir -p /build/generated && \
    python -m grpc_tools.protoc \
    -I /build/proto \
    --python_out=/build/generated \
    --grpc_python_out=/build/generated \
    /build/proto/*.proto && \
    touch /build/generated/__init__.py

FROM python:3.11-slim AS serving

WORKDIR /app
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

COPY python-ml/requirements-serve.txt /tmp/requirements-serve.txt
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-warn-script-location -r /tmp/requirements-serve.txt

COPY proto /app/proto
COPY configs /app/configs
COPY python-ml /app/python-ml
COPY python-ml/artifacts /app/models
COPY --from=proto-builder /build/generated/ /app/python-ml/shared/pb/

ENV PYTHONPATH=/app/python-ml:/app/python-ml/shared/pb \
    PORT=50051 \
    AEGIS_ML_MODEL_PATH=/app/models/incident_models.joblib \
    OMP_NUM_THREADS=2

RUN useradd --create-home --uid 10001 aegis && \
    chown -R aegis:aegis /app
USER aegis

EXPOSE 50051
WORKDIR /app/python-ml
CMD ["python", "serving/grpc_server.py"]

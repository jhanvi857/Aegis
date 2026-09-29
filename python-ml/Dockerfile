FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY python-ml/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

COPY proto /app/proto
COPY configs /app/configs
COPY python-ml /app/python-ml

ENV PYTHONPATH=/app/python-ml:/app/python-ml/shared/pb
ENV PORT=50051

EXPOSE 50051

WORKDIR /app/python-ml
CMD ["python", "serving/grpc_server.py"]

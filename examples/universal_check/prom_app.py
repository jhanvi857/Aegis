import os
from prometheus_client import Counter, start_http_server
from time import sleep

service_name = os.getenv("SERVICE_NAME", "svc-prom")
requests = Counter("http_requests_total", "Requests", ["service"])
start_http_server(int(os.getenv("PORT", "9090")))
while True:
    requests.labels(service_name).inc()
    sleep(1)

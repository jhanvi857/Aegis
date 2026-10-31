import os
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

from opentelemetry import trace
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter

service_name = os.getenv("OTEL_SERVICE_NAME", "svc-otel")
provider = TracerProvider(resource=Resource.create({SERVICE_NAME: service_name}))
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
trace.set_tracer_provider(provider)
tracer = trace.get_tracer(__name__)

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        with tracer.start_as_current_span("call-prometheus-service") as span:
            span.set_attribute("peer.service", "svc-prom")
            if os.getenv("HIGH_ERRORS") == "true":
                span.set_status(trace.Status(trace.StatusCode.ERROR, "verification fault"))
            time.sleep(float(os.getenv("LATENCY_SECONDS", "0.01")))
        self.send_response(500 if os.getenv("HIGH_ERRORS") == "true" else 200)
        self.end_headers()
        self.wfile.write(b"ok")

    def log_message(self, *_args):
        return

HTTPServer(("", int(os.getenv("PORT", "8080"))), Handler).serve_forever()

"""
Kafka consumer for live telemetry streams (Phase 2)
Consumes TelemetryBatch messages from Kafka and provides stream decoding
with a built-in simulation fallback for local testing.
"""

import json
import logging
import time
import random
from typing import Generator, Optional, Dict, Any, List

logger = logging.getLogger(__name__)


class TelemetryConsumer:
    """
    Consumes live telemetry streams from Kafka.
    Falls back gracefully to synthetic simulation if Kafka is unavailable,
    ensuring downstream graph builders and algorithms can be thoroughly tested.
    """

    def __init__(
        self,
        bootstrap_servers: str = "localhost:9092",
        topic: str = "aegis-telemetry",
        group_id: str = "aegis-graph-builder",
        auto_offset_reset: str = "latest",
    ):
        self.bootstrap_servers = bootstrap_servers
        self.topic = topic
        self.group_id = group_id
        self.auto_offset_reset = auto_offset_reset
        self.consumer = None
        self.is_running = False

    def start(self) -> bool:
        """
        Attempts to connect to Kafka broker.
        Returns True if connected, False if falling back to simulation mode.
        """
        self.is_running = True
        try:
            from kafka import KafkaConsumer  # type: ignore

            self.consumer = KafkaConsumer(
                self.topic,
                bootstrap_servers=self.bootstrap_servers.split(","),
                group_id=self.group_id,
                auto_offset_reset=self.auto_offset_reset,
                enable_auto_commit=True,
                consumer_timeout_ms=1000,
                value_deserializer=lambda m: self._deserialize_payload(m),
            )
            logger.info(f"Connected to Kafka broker at {self.bootstrap_servers}, topic: {self.topic}")
            return True
        except Exception as e:
            logger.warning(
                f"Could not connect to Kafka at {self.bootstrap_servers} ({e}). "
                "Operating in simulation/standby mode."
            )
            self.consumer = None
            return False

    def _deserialize_payload(self, raw_bytes: bytes) -> Dict[str, Any]:
        """Deserializes message bytes from JSON or Protobuf."""
        try:
            return json.loads(raw_bytes.decode("utf-8"))
        except Exception:
            # Fallback for binary / protobuf inspection
            return {"raw_bytes": raw_bytes}

    def stream_batches(self) -> Generator[Dict[str, Any], None, None]:
        """
        Yields telemetry batches.
        Reads from live Kafka if connected; otherwise yields simulated batches.
        """
        if self.consumer:
            while self.is_running:
                try:
                    for message in self.consumer:
                        if not self.is_running:
                            break
                        if isinstance(message.value, dict):
                            yield message.value
                        else:
                            yield {"raw": message.value}
                except Exception as e:
                    logger.error(f"Error reading from Kafka: {e}")
                    time.sleep(1.0)
        else:
            logger.info("Starting synthetic telemetry stream...")
            for batch in self.simulate_stream():
                if not self.is_running:
                    break
                yield batch

    def generate_simulated_batch(
        self,
        node_ids: Optional[List[str]] = None,
        fault_node: Optional[str] = None,
        fault_type: Optional[str] = None,
    ) -> Dict[str, Any]:
        """
        Generates a realistic telemetry batch for testing Phase 2 graph annotations.
        """
        if node_ids is None:
            node_ids = ["gateway", "node-a", "node-b", "node-c", "node-d"]

        now_ns = int(time.time() * 1e9)
        metrics = []
        logs = []

        for node in node_ids:
            is_faulty = node == fault_node

            if is_faulty and fault_type == "cpu_stress":
                cpu = random.uniform(92.0, 99.5)
                latency = random.uniform(150.0, 350.0)
                error_rate = random.uniform(0.05, 0.15)
            elif is_faulty and fault_type == "latency":
                cpu = random.uniform(25.0, 45.0)
                latency = random.uniform(600.0, 1500.0)
                error_rate = random.uniform(0.08, 0.25)
            elif is_faulty and fault_type == "memory_leak":
                cpu = random.uniform(40.0, 70.0)
                latency = random.uniform(200.0, 400.0)
                error_rate = random.uniform(0.1, 0.4)
            elif is_faulty and fault_type == "kill_service":
                cpu = 0.0
                latency = 5000.0
                error_rate = 1.0
            else:
                cpu = random.uniform(15.0, 45.0)
                latency = random.uniform(10.0, 40.0)
                error_rate = random.uniform(0.001, 0.01)

            metrics.extend([
                {"service_id": node, "metric_name": "cpu_usage", "value": cpu, "unit": "%", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "memory_usage", "value": random.uniform(30.0, 60.0), "unit": "%", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "p95_latency_ms", "value": latency, "unit": "ms", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "error_rate", "value": error_rate, "unit": "ratio", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "rps", "value": random.uniform(100.0, 500.0), "unit": "req/s", "timestamp_unix_nano": now_ns},
            ])

            if is_faulty:
                logs.append({
                    "timestamp_unix_nano": now_ns,
                    "service_id": node,
                    "level": "ERROR",
                    "message": f"High degradation detected on {node}: {fault_type}",
                    "trace_id": f"trace-{random.randint(1000, 9999)}",
                })

        return {
            "timestamp_unix_nano": now_ns,
            "metrics": metrics,
            "logs": logs,
            "traces": [],
        }

    def simulate_stream(
        self,
        interval_sec: float = 1.0,
        max_batches: Optional[int] = None,
        fault_node: Optional[str] = None,
        fault_type: Optional[str] = None,
    ) -> Generator[Dict[str, Any], None, None]:
        """Yields synthetic telemetry batches at a fixed interval."""
        count = 0
        while self.is_running:
            yield self.generate_simulated_batch(
                fault_node=fault_node,
                fault_type=fault_type,
            )
            count += 1
            if max_batches is not None and count >= max_batches:
                break
            time.sleep(interval_sec)

    def close(self):
        """Stops streaming and closes Kafka consumer."""
        self.is_running = False
        if self.consumer:
            try:
                self.consumer.close()
            except Exception as e:
                logger.warning(f"Error closing Kafka consumer: {e}")
            self.consumer = None
        logger.info("TelemetryConsumer stopped")

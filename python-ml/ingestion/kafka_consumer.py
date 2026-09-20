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
        precursor_severity: float = 0.0,
    ) -> Dict[str, Any]:
        """
        Generates a realistic telemetry batch for testing Phase 2 graph annotations
        and Phase 3 predictive lead-time modeling.
        """
        if node_ids is None:
            node_ids = ["gateway", "node-a", "node-b", "node-c", "node-d"]

        now_ns = int(time.time() * 1e9)
        metrics = []
        logs = []

        for node in node_ids:
            is_faulty = node == fault_node

            if is_faulty and precursor_severity > 0.0:
                # Pre-injection precursor signal (subtle drift before threshold breach)
                s = min(0.75, max(0.05, precursor_severity))
                mem = random.uniform(30.0, 50.0)
                queue_depth = random.randint(1, 4)
                db_wait = random.uniform(8.0, 25.0) + (10.0 * s)
                cache_miss = random.uniform(0.05, 0.18)

                if fault_type == "cpu_stress":
                    cpu = random.uniform(25.0, 42.0) + (38.0 * s) + random.gauss(0, 2.0)
                    latency = random.uniform(15.0, 32.0) + (35.0 * s)
                    error_rate = random.uniform(0.002, 0.008) + (0.015 * s)
                elif fault_type == "memory_leak":
                    cpu = random.uniform(22.0, 40.0)
                    mem = random.uniform(32.0, 45.0) + (42.0 * s)
                    latency = random.uniform(15.0, 30.0) + (25.0 * s)
                    error_rate = random.uniform(0.001, 0.008)
                elif fault_type in ("db_lock", "slow_query"):
                    cpu = random.uniform(20.0, 40.0)
                    latency = random.uniform(18.0, 35.0) + (85.0 * s) + random.gauss(0, 4.0)
                    error_rate = random.uniform(0.002, 0.008) + (0.020 * s)
                    db_wait = random.uniform(25.0, 60.0) + (550.0 * s) + random.gauss(0, 10.0)
                elif fault_type == "cache_down":
                    cpu = random.uniform(22.0, 42.0)
                    latency = random.uniform(18.0, 38.0) + (65.0 * s)
                    error_rate = random.uniform(0.002, 0.008) + (0.018 * s)
                    cache_miss = min(0.95, 0.15 + (0.75 * s) + random.gauss(0, 0.03))
                elif fault_type == "latency":
                    cpu = random.uniform(20.0, 40.0)
                    latency = random.uniform(18.0, 35.0) + (85.0 * s) + random.gauss(0, 4.0)
                    error_rate = random.uniform(0.002, 0.008) + (0.020 * s)
                elif fault_type == "mq_lag":
                    cpu = random.uniform(20.0, 38.0)
                    latency = random.uniform(15.0, 32.0) + (40.0 * s)
                    error_rate = random.uniform(0.001, 0.006)
                    queue_depth = max(1, int(2 + 20.0 * s + random.randint(0, 2)))
                elif fault_type == "kill_service":
                    cpu = max(8.0, random.uniform(22.0, 38.0) - (16.0 * s))
                    latency = random.uniform(15.0, 32.0) + (35.0 * s)
                    error_rate = random.uniform(0.002, 0.008) + (0.035 * s)
                else:
                    cpu = random.uniform(20.0, 40.0) + (20.0 * s)
                    latency = random.uniform(15.0, 35.0) + (55.0 * s)
                    error_rate = random.uniform(0.002, 0.008) + (0.025 * s)

            elif is_faulty:
                # Active high-severity failure state
                mem = random.uniform(30.0, 60.0)
                queue_depth = random.randint(5, 20)
                db_wait = random.uniform(15.0, 45.0)
                cache_miss = random.uniform(0.08, 0.22)

                if fault_type == "cpu_stress":
                    cpu = random.uniform(92.0, 99.5)
                    latency = random.uniform(150.0, 350.0)
                    error_rate = random.uniform(0.05, 0.15)
                elif fault_type == "latency":
                    cpu = random.uniform(25.0, 45.0)
                    latency = random.uniform(1050.0, 1800.0)
                    error_rate = random.uniform(0.08, 0.25)
                elif fault_type == "memory_leak":
                    cpu = random.uniform(40.0, 70.0)
                    mem = random.uniform(88.0, 98.0)
                    latency = random.uniform(200.0, 400.0)
                    error_rate = random.uniform(0.1, 0.4)
                elif fault_type in ("db_lock", "slow_query"):
                    cpu = random.uniform(30.0, 55.0)
                    latency = random.uniform(250.0, 750.0)
                    error_rate = random.uniform(0.05, 0.20)
                    db_wait = random.uniform(550.0, 950.0)  # Exceeds 500ms threshold
                elif fault_type == "cache_down":
                    cpu = random.uniform(35.0, 65.0)
                    latency = random.uniform(250.0, 600.0)
                    error_rate = random.uniform(0.03, 0.15)
                    cache_miss = random.uniform(0.85, 0.98)  # Exceeds 0.80 threshold
                elif fault_type == "kill_service":
                    cpu = 0.0
                    latency = 5000.0
                    error_rate = 1.0
                else:
                    cpu = random.uniform(50.0, 85.0)
                    latency = random.uniform(300.0, 700.0)
                    error_rate = random.uniform(0.05, 0.20)
            else:
                # Nominal background operation with natural variance
                cpu = max(10.0, min(58.0, random.uniform(18.0, 46.0) + random.gauss(0, 3.0)))
                mem = random.uniform(28.0, 58.0)
                latency = max(8.0, min(48.0, random.uniform(12.0, 38.0) + random.gauss(0, 3.0)))
                error_rate = random.uniform(0.001, 0.012)
                queue_depth = random.randint(1, 3)
                db_wait = max(2.0, min(50.0, random.uniform(5.0, 25.0) + random.gauss(0, 2.0)))
                cache_miss = max(0.01, min(0.30, random.uniform(0.04, 0.18)))

            metrics.extend([
                {"service_id": node, "metric_name": "cpu_usage", "value": round(cpu, 2), "unit": "%", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "memory_usage", "value": round(mem, 2), "unit": "%", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "p95_latency_ms", "value": round(latency, 2), "unit": "ms", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "error_rate", "value": round(error_rate, 4), "unit": "ratio", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "rps", "value": random.uniform(100.0, 500.0), "unit": "req/s", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "queue_depth", "value": queue_depth, "unit": "count", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "db_connection_wait_ms", "value": round(db_wait, 2), "unit": "ms", "timestamp_unix_nano": now_ns},
                {"service_id": node, "metric_name": "cache_miss_rate", "value": round(cache_miss, 4), "unit": "ratio", "timestamp_unix_nano": now_ns},
            ])

            if is_faulty and precursor_severity == 0.0:
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

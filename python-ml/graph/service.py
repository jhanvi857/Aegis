"""
Phase 2 Graph Preprocessing Service
Ties together TelemetryConsumer, GraphBuilder, and GraphRepresentationBuilder into
a cohesive streaming service that continuously updates the live System Graph and
emits ready-to-consume Graph Representations for TGNN models and Dashboard visualization.
"""

import threading
import time
import logging
from typing import Dict, Any, Optional

try:
    from graph.graph_builder import GraphBuilder
    from graph.representation import GraphRepresentationBuilder
    from ingestion.kafka_consumer import TelemetryConsumer
except ImportError:
    try:
        from .graph_builder import GraphBuilder
        from .representation import GraphRepresentationBuilder
        from ..ingestion.kafka_consumer import TelemetryConsumer
    except ImportError:
        from graph_builder import GraphBuilder
        from representation import GraphRepresentationBuilder
        from kafka_consumer import TelemetryConsumer

logger = logging.getLogger(__name__)


class GraphPreprocessingService:
    """
    Authoritative service for Phase 2:
    Streams telemetry -> updates System Graph -> precomputes structural features -> emits GraphRepresentation.
    """

    def __init__(
        self,
        topology_path: str = "configs/topology.yaml",
        kafka_bootstrap: str = "localhost:9092",
        kafka_topic: str = "aegis-telemetry",
        structural_ttl_sec: float = 5.0,
    ):
        self.builder = GraphBuilder(topology_path=topology_path)
        self.rep_builder = GraphRepresentationBuilder(structural_cache_ttl_sec=structural_ttl_sec)
        self.consumer = TelemetryConsumer(bootstrap_servers=kafka_bootstrap, topic=kafka_topic)

        self._lock = threading.Lock()
        self._current_representation: Optional[Dict[str, Any]] = None
        self._is_running = False
        self._worker_thread: Optional[threading.Thread] = None

        # Perform initial build
        self._recompute_representation(force=True)

    def _recompute_representation(self, force: bool = False):
        """Builds and caches latest GraphRepresentation under lock."""
        with self._lock:
            self._current_representation = self.rep_builder.build_representation(
                self.builder.graph,
                force_recompute_structural=force,
            )

    def process_telemetry_batch(self, batch: Dict[str, Any]) -> Dict[str, Any]:
        """
        Processes a single telemetry batch:
        Updates node/edge metrics and generates updated GraphRepresentation.
        """
        with self._lock:
            self.builder.update_with_telemetry(batch)
            self._current_representation = self.rep_builder.build_representation(
                self.builder.graph,
                force_recompute_structural=False,
            )
            return self._current_representation

    def get_latest_representation(self) -> Dict[str, Any]:
        """Returns the latest preprocessed GraphRepresentation."""
        with self._lock:
            if self._current_representation is None:
                self._recompute_representation(force=True)
            return self._current_representation  # type: ignore

    def get_graph(self):
        """Returns reference to the active NetworkX graph."""
        return self.builder.graph

    def start_streaming(self):
        """Starts background consumption thread."""
        if self._is_running:
            return

        self._is_running = True
        self.consumer.start()

        def _consume_loop():
            logger.info("GraphPreprocessingService consumption loop started")
            for batch in self.consumer.stream_batches():
                if not self._is_running:
                    break
                self.process_telemetry_batch(batch)

        self._worker_thread = threading.Thread(target=_consume_loop, daemon=True)
        self._worker_thread.start()

    def stop(self):
        """Stops streaming consumer and cleans up resources."""
        self._is_running = False
        self.consumer.close()
        if self._worker_thread and self._worker_thread.is_alive():
            self._worker_thread.join(timeout=2.0)
        logger.info("GraphPreprocessingService stopped")


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    service = GraphPreprocessingService()
    logger.info("Initializing GraphPreprocessingService Phase 2 demo...")
    rep = service.get_latest_representation()
    print(f"Topological order: {rep['topological_order']}")
    print(f"Centrality scores: {rep['centrality_scores']}")
    print(f"Has cycles: {rep['has_cycles']}")

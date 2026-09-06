"""
gRPC Server serving PredictionService and RecoveryService to Go control plane
"""

import time
import logging
from concurrent import futures

logger = logging.getLogger(__name__)


def serve(port: int = 50051):
    logger.info(f"[grpc-server] Aegis ML gRPC Server starting on port {port}...")
    # Will bind proto generated services in Phase 3 & 4


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    serve()

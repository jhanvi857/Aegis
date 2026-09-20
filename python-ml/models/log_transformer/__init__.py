"""
Log Transformer module
Embeds log lines into dense feature vectors for TGNN node/edge features.
"""

from .embedder import LogTransformerEmbedder

__all__ = ["LogTransformerEmbedder"]

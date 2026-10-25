import os
import sys

# Suppress Protobuf major version runtime mismatch across heterogeneous environments
os.environ.setdefault("TEMPORARILY_DISABLE_PROTOBUF_VERSION_CHECK", "true")

_pb_dir = os.path.dirname(os.path.abspath(__file__))
if _pb_dir not in sys.path:
    sys.path.insert(0, _pb_dir)

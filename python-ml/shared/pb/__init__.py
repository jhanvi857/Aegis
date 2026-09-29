import os
import sys

_pb_dir = os.path.dirname(os.path.abspath(__file__))
if _pb_dir not in sys.path:
    sys.path.insert(0, _pb_dir)

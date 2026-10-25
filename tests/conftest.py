import os

# Suppress Protobuf runtime/gencode major version mismatch across test environments
os.environ["TEMPORARILY_DISABLE_PROTOBUF_VERSION_CHECK"] = "true"

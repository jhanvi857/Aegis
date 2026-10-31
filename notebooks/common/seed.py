"""Central seed policy for every notebook and repeat."""

import os
import random

import numpy as np

SEED = 42
REPEAT_SEEDS = (0, 1, 2, 3, 4)


def set_seed(seed: int = SEED) -> int:
    random.seed(seed)
    np.random.seed(seed)
    os.environ["PYTHONHASHSEED"] = str(seed)
    return seed

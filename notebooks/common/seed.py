"""Central seed policy for every notebook and repeat."""

import os
import random

import numpy as np
import torch


SEED = 42
REPEAT_SEEDS = (0, 1, 2, 3, 4)


def set_seed(seed: int = SEED) -> int:
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)
    os.environ["PYTHONHASHSEED"] = str(seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(seed)
    return seed
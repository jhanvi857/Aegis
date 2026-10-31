"""Execute every notebook from a clean kernel and fail on cell errors."""

from __future__ import annotations

import argparse
import os
from pathlib import Path

import nbformat
from nbclient import NotebookClient


ROOT = Path(__file__).resolve().parents[2]


def main(reduced: bool = False) -> None:
    os.environ["AEGIS_REDUCED"] = "1" if reduced else "0"
    for path in sorted((ROOT / "notebooks").glob("0[1-3]_*.ipynb")):
        notebook = nbformat.read(path, as_version=4)
        NotebookClient(notebook, timeout=600, kernel_name="python3", resources={"metadata": {"path": str(ROOT)}}).execute()
        print(f"PASS {path.name}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--reduced", action="store_true")
    main(parser.parse_args().reduced)

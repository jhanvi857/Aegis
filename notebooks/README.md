# Aegis ML notebook suite

This suite is the professor-facing, reproducible path through the Aegis ML evidence. Each notebook opens with **Question**, **Method**, and **Takeaway**, loads repository code instead of duplicating models, and places a short interpretation after each result cell.

## Run order

1. `01_data_generation.ipynb` exports the tensor dataset and plots one measured episode.
2. `02_data_validity.ipynb` recomputes balance, leakage, shortcut, and split-stability checks.
3. `03_eda_features.ipynb` describes telemetry and topology structure.
4. `04_models.ipynb` runs the compact model comparison. Set `RETRAIN = True` to train the small neural path.
5. `05_evaluation.ipynb` reports held-out metrics and fixed-seed repeats.
6. `06_unseen_scenarios.ipynb` inspects the multi-topology artifact and refuses to invent missing scenario results.
7. `07_conclusion.ipynb` joins the generated evidence and states limitations.

## Reproducibility

`common/seed.py` owns seed 42 and repeat seeds 0–4. `common/data.py`, `common/metrics.py`, and `common/plotting.py` are shared imports. The default path loads `python-ml/models/checkpoints/tgnn_best.pt`; training writes small checkpoints under `notebooks/artifacts/`, and JSON metrics under `notebooks/results/`.

The CSV export is `dataset/export_csv.py`. It writes `data/node_timeseries.csv`, `data/edges.csv`, `data/episodes.csv`, and `data/episodes_flat.csv` from tensors plus complete episode metadata.

## Headless smoke test

From the repository root:

```text
make notebooks-smoke
```

The script uses `nbclient`, a fresh Python kernel per notebook, and `AEGIS_REDUCED=1`. It fails on the first cell error. Each notebook is designed to stay below ten minutes on Kaggle CPU or one GPU; the default cached-checkpoint path is substantially shorter.

## Kaggle

Upload the repository or notebook directory, set the working directory to the repository root, install `notebooks/requirements.txt`, and run the notebooks in order. No absolute paths are used. A clean environment must include PyTorch because the repository tensors and TGNN are PyTorch artifacts.

## Artifact policy

Committed artifacts are intentionally small: source tensors/checkpoints remain the repository's canonical inputs, while notebook JSON results are caches that can be regenerated. No number in notebooks 05–07 is copied from documentation; values are loaded from or recomputed into `notebooks/results/*.json`.

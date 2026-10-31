# Dataset card: RCAEval candidate data

RCAEval is the primary candidate for offline validation. Local files are stored under `dataset/external/rcaeval_re1/` and `dataset/external/rcaeval_sample/`.

## Validation status

The local RE1 schemas and sample coverage have been inspected. The reproducible preparation and incident-level detection evaluator are documented in [datasets.md](datasets.md), but have not yet been run as part of this implementation pass. No benchmark score or RCA performance claim is made. Do not treat the project's prior synthetic simulator outputs or stored evaluation JSON as external benchmark results.

## Required audit

- Inspect each file schema and timestamp resolution.
- Verify whether rows are service-level or system-level and how service identity is encoded.
- Confirm how `inject_time` and root-cause service labels are stored for the selected RE1/RE2-OB subset.
- Check missing values, repeated windows, topology identity, and incident IDs.
- Split by incident before selecting thresholds or fitting any transformation.
- Report limitations and keep Kaggle data separate.

Run the RE1 preparation and evaluation commands in [datasets.md](datasets.md) after implementation is complete. Review the generated EDA manifest, case coverage, metric mappings, unit conversions, and incident split before interpreting the detector report. RE2-OB trace-based RCA still requires its actual trace payloads; local metadata alone is insufficient.

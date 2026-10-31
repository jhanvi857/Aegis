# Validation status

The repository does not currently publish verified external benchmark performance for the production baseline path. Historical simulator results and stored model outputs are not evidence for live application performance.

The production detector uses static thresholds and process-local rolling robust baselines. Validate it with incident-aware RCAEval splits before claiming detection or RCA quality. Preserve incident boundaries across train/validation/test, report missingness and cold start, and keep production ingestion separate from dataset-specific transformations.

Recovery verification must be demonstrated against a configured adapter and health check. Unit tests alone do not establish end-to-end deployment behavior.

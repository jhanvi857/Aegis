# Detection, RCA, and recovery planning

The decision flow separates abnormal behavior from root-cause attribution and recovery:

1. The shared detector evaluates only received signals and includes thresholds, baseline sample counts, and contributing evidence.
2. Incident correlation groups related symptoms only when time and topology evidence support it.
3. RCA ranks candidates and alternatives. Topology alone is insufficient to assert causality; absent trace or temporal evidence must remain explicit uncertainty.
4. BFS describes potential impact. SCC condensation exposes cycle groups and provides a DAG for dependency ordering.
5. The recovery planner recommends actions only when observed metrics match configured rules and environment capabilities.
6. Go policy validates permissions and risk; approvals gate configured actions. Execution, verification, and audit are distinct statuses.

Python does not execute recovery. No command response is represented as verified recovery.

# RCAEval RE1 offline comparison

Cases: 375; windows: 1230790; systems: RE1-OB, RE1-SS, RE1-TT.
Labels: timestamp >= inject_time is faulty; earlier windows are normal. Metrics only; traces are not used.
Detection precision, recall and F1 are fold means ± sample standard deviation. RCA top-k is the fraction of labeled cases whose root_cause_service ranks in the first k services.

## case group kfold

| Model | Precision | Recall | F1 | RCA top-1 | RCA top-3 | Labeled RCA cases |
|---|---:|---:|---:|---:|---:|---:|
| ServiceDetector | 0.647 ± 0.018 | 0.229 ± 0.002 | 0.339 ± 0.004 | 0.133 ± 0.009 | 0.373 ± 0.044 | 375 |
| Isolation Forest | 0.558 ± 0.006 | 0.230 ± 0.004 | 0.325 ± 0.004 | 0.155 ± 0.030 | 0.397 ± 0.033 | 375 |
| Random Forest | 0.602 ± 0.008 | 0.567 ± 0.012 | 0.584 ± 0.003 | 0.192 ± 0.042 | 0.301 ± 0.065 | 375 |
| XGBoost | 0.574 ± 0.002 | 0.413 ± 0.016 | 0.480 ± 0.011 | 0.144 ± 0.049 | 0.264 ± 0.070 | 375 |

## leave one system out

| Model | Precision | Recall | F1 | RCA top-1 | RCA top-3 | Labeled RCA cases |
|---|---:|---:|---:|---:|---:|---:|
| ServiceDetector | 0.707 ± 0.143 | 0.254 ± 0.090 | 0.367 ± 0.099 | 0.133 ± 0.033 | 0.373 ± 0.188 | 375 |
| Isolation Forest | 0.530 ± 0.051 | 0.396 ± 0.218 | 0.421 ± 0.142 | 0.363 ± 0.107 | 0.589 ± 0.180 | 375 |
| Random Forest | 0.524 ± 0.033 | 0.541 ± 0.130 | 0.524 ± 0.052 | 0.112 ± 0.029 | 0.363 ± 0.136 | 375 |
| XGBoost | 0.542 ± 0.022 | 0.370 ± 0.123 | 0.431 ± 0.073 | 0.155 ± 0.115 | 0.285 ± 0.143 | 375 |

## Method and limitations

A case is the grouping unit: all windows from one case stay in one fold. Metric samples are averaged into fixed 10-second service windows, with bins aligned to the injection boundary. Leave-one-system-out evaluates OB, SS and TT separately when each is held out. Features are per-service metric windows with missing-value indicators; model imputation is fit on training data only. Isolation Forest is fit only on normal training windows. Random Forest uses balanced class weights; XGBoost uses a training-fold class ratio.
RCA ranking uses metric anomaly onset (first predicted anomalous service window), then that service's maximum anomaly score as a tie-break. This is a metric-only heuristic, does not use traces or topology, and is not a causal claim. Labels derive from the injection timestamp and do not establish exact per-service fault onset. Results are limited by the small RE1 case set and cannot establish generalization beyond these captures.

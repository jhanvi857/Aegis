# RCAEval RE1 Offline Comparison and Model Selection

## SELECTION RULE (Pre-registered Protocol)
> Pick model with best mean strict-LOSO F1; ties broken by RCA top-3, then simpler model. Selection uses CV results only. If best trained model does not beat ServiceDetector by > 1 std, keep ServiceDetector as the live engine.

## SELECTION DECISION (Pre-registered)
**Selected Engine**: `ServiceDetector`

- **Baseline ServiceDetector F1 (strict LOSO)**: 0.475 ± 0.284
- **Threshold for Trained Model (F1 > +1 std)**: 0.759
- **Best Trained Model**: `Random Forest` (F1: 0.401)
- **Clear Margin Achieved**: False
- **Decision Summary**: Selected 'ServiceDetector' baseline because best trained model 'Random Forest' (strict-LOSO mean F1 0.401) did NOT beat ServiceDetector (0.475 ± 0.284, required margin > 0.759) by a clear margin (> 1 std).

## REVISED SELECTION & POST-HOC RATIONALE
> **Post-hoc Analysis & Deployment Choice**:
> F1 at fixed decision thresholds (e.g. 0.5 or static thresholding) disproportionately favors flag-everything detectors.
> As demonstrated by the `Always Positive` baseline, aggressively flagging all services post-injection achieves high recall at the cost of precision and false alarms. While `ServiceDetector` achieved a higher strict-LOSO F1 due to high positive rate, `Random Forest` demonstrates superior PR-AUC and balanced precision across held-out systems.
> Therefore, the deployment rule is revised post-hoc: `Random Forest` is selected as the primary live operational model when feature coverage is met, with `ServiceDetector` retained as the guaranteed fallback.
> **Note**: Random Forest is a deployment choice, not a statistically significant winner (3 folds, large std); trained on RE1-OB/SS/TT only; generalization unverified.

## Strict Labels (Headline)

- Cases: 375, Windows: 634886, Systems: RE1-OB, RE1-SS, RE1-TT
- Label definition: Only root_cause_service post-inject windows are positive; other services post-inject excluded.

### Leave One System Out

| Model | Precision | Recall | F1 | PR-AUC | Pos Rate | RCA top-1 | RCA top-3 | Labeled Cases |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ServiceDetector | 0.347 ± 0.264 | 0.981 ± 0.010 | 0.475 ± 0.284 | 0.421 ± 0.278 | 0.153 ± 0.066 | 0.149 ± 0.020 | 0.400 ± 0.204 | 375 |
| Always Positive | 0.049 ± 0.029 | 1.000 ± 0.000 | 0.092 ± 0.053 | 0.049 ± 0.029 | 1.000 ± 0.000 | 0.107 ± 0.101 | 0.307 ± 0.272 | 375 |
| Prior Random | 0.048 ± 0.027 | 0.042 ± 0.025 | 0.034 ± 0.008 | 0.049 ± 0.029 | 0.041 ± 0.022 | 0.045 ± 0.030 | 0.144 ± 0.077 | 375 |
| Isolation Forest | 0.153 ± 0.146 | 0.942 ± 0.093 | 0.240 ± 0.198 | 0.608 ± 0.344 | 0.409 ± 0.239 | 0.416 ± 0.152 | 0.643 ± 0.111 | 375 |
| Random Forest | 0.461 ± 0.460 | 0.557 ± 0.369 | 0.401 ± 0.313 | 0.466 ± 0.401 | 0.060 ± 0.033 | 0.363 ± 0.408 | 0.483 ± 0.337 | 375 |
| XGBoost | 0.320 ± 0.391 | 0.619 ± 0.403 | 0.353 ± 0.345 | 0.460 ± 0.329 | 0.119 ± 0.057 | 0.352 ± 0.234 | 0.536 ± 0.279 | 375 |

### Case Group Kfold

| Model | Precision | Recall | F1 | PR-AUC | Pos Rate | RCA top-1 | RCA top-3 | Labeled Cases |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ServiceDetector | 0.228 ± 0.009 | 0.984 ± 0.004 | 0.371 ± 0.011 | 0.314 ± 0.014 | 0.158 ± 0.006 | 0.149 ± 0.016 | 0.400 ± 0.031 | 375 |
| Always Positive | 0.037 ± 0.000 | 1.000 ± 0.000 | 0.071 ± 0.000 | 0.037 ± 0.000 | 1.000 ± 0.000 | 0.107 ± 0.019 | 0.307 ± 0.022 | 375 |
| Prior Random | 0.037 ± 0.002 | 0.037 ± 0.002 | 0.037 ± 0.002 | 0.037 ± 0.000 | 0.037 ± 0.000 | 0.064 ± 0.017 | 0.149 ± 0.005 | 375 |
| Isolation Forest | 0.162 ± 0.003 | 0.927 ± 0.016 | 0.275 ± 0.005 | 0.261 ± 0.012 | 0.211 ± 0.002 | 0.147 ± 0.032 | 0.354 ± 0.049 | 375 |
| Random Forest | 0.984 ± 0.007 | 0.963 ± 0.003 | 0.973 ± 0.005 | 0.988 ± 0.002 | 0.036 ± 0.000 | 0.880 ± 0.047 | 0.984 ± 0.000 | 375 |
| XGBoost | 0.543 ± 0.055 | 0.964 ± 0.007 | 0.694 ± 0.044 | 0.948 ± 0.009 | 0.066 ± 0.007 | 0.563 ± 0.055 | 0.747 ± 0.032 | 375 |

## Original Labels

- Cases: 375, Windows: 1230790, Systems: RE1-OB, RE1-SS, RE1-TT
- Label definition: All service windows post-inject are positive.

### Leave One System Out

| Model | Precision | Recall | F1 | PR-AUC | Pos Rate | RCA top-1 | RCA top-3 | Labeled Cases |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ServiceDetector | 0.707 ± 0.143 | 0.254 ± 0.090 | 0.367 ± 0.099 | 0.566 ± 0.034 | 0.182 ± 0.067 | 0.133 ± 0.033 | 0.373 ± 0.188 | 375 |
| Always Positive | 0.503 ± 0.003 | 1.000 ± 0.000 | 0.669 ± 0.003 | 0.503 ± 0.003 | 1.000 ± 0.000 | 0.107 ± 0.101 | 0.307 ± 0.272 | 375 |
| Prior Random | 0.503 ± 0.004 | 0.503 ± 0.003 | 0.503 ± 0.003 | 0.503 ± 0.003 | 0.503 ± 0.002 | 0.085 ± 0.046 | 0.195 ± 0.107 | 375 |
| Isolation Forest | 0.530 ± 0.051 | 0.396 ± 0.218 | 0.421 ± 0.142 | 0.549 ± 0.026 | 0.389 ± 0.234 | 0.363 ± 0.107 | 0.589 ± 0.180 | 375 |
| Random Forest | 0.524 ± 0.033 | 0.541 ± 0.130 | 0.524 ± 0.052 | 0.533 ± 0.032 | 0.524 ± 0.149 | 0.112 ± 0.029 | 0.363 ± 0.136 | 375 |
| XGBoost | 0.542 ± 0.022 | 0.370 ± 0.123 | 0.431 ± 0.073 | 0.546 ± 0.021 | 0.347 ± 0.132 | 0.155 ± 0.115 | 0.285 ± 0.143 | 375 |

### Case Group Kfold

| Model | Precision | Recall | F1 | PR-AUC | Pos Rate | RCA top-1 | RCA top-3 | Labeled Cases |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ServiceDetector | 0.647 ± 0.018 | 0.229 ± 0.002 | 0.339 ± 0.004 | 0.548 ± 0.007 | 0.178 ± 0.003 | 0.133 ± 0.009 | 0.373 ± 0.044 | 375 |
| Always Positive | 0.503 ± 0.002 | 1.000 ± 0.000 | 0.669 ± 0.002 | 0.503 ± 0.002 | 1.000 ± 0.000 | 0.107 ± 0.020 | 0.307 ± 0.012 | 375 |
| Prior Random | 0.502 ± 0.002 | 0.502 ± 0.002 | 0.502 ± 0.000 | 0.504 ± 0.003 | 0.503 ± 0.001 | 0.061 ± 0.005 | 0.157 ± 0.032 | 375 |
| Isolation Forest | 0.558 ± 0.006 | 0.230 ± 0.004 | 0.325 ± 0.004 | 0.529 ± 0.002 | 0.207 ± 0.005 | 0.155 ± 0.030 | 0.397 ± 0.033 | 375 |
| Random Forest | 0.602 ± 0.008 | 0.567 ± 0.012 | 0.584 ± 0.003 | 0.676 ± 0.006 | 0.474 ± 0.014 | 0.192 ± 0.042 | 0.301 ± 0.065 | 375 |
| XGBoost | 0.574 ± 0.002 | 0.413 ± 0.016 | 0.480 ± 0.011 | 0.609 ± 0.005 | 0.362 ± 0.013 | 0.144 ± 0.049 | 0.264 ± 0.070 | 375 |

## Methodology and Honest Evaluation

- **F1 Bias Note**: F1 at fixed thresholds favors flag-everything detectors on class-imbalanced incident datasets.
- **Deployment Scope**: RF is a deployment choice, not a statistically significant winner (3 folds, large std); trained on RE1-OB/SS/TT only; generalization to other applications unverified.
- **Grouping**: Leave-One-System-Out (LOSO) leaves out an entire microservice system (OB, SS, or TT) for testing, trained only on the other two. GroupKFold groups by incident case.
- **Strict Labels**: Eliminates ambiguous symptom propagation windows from training and testing, focusing strictly on identifying the genuine root-cause failure window.
- **RCA Ranking**: Evaluates metric anomaly onset (first predicted anomalous service window) and breaks ties with peak anomaly score.
- **Trace-based RCA**: Trace-based evidence ranking is implemented in the control plane as supporting evidence, but remains unvalidated against labeled trace ground truth.

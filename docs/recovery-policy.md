# Recovery policy

Aegis treats action recommendation, approval, attempt, and verified recovery as separate states.

- The connected environment must explicitly advertise supported actions and targets.
- The rulebook and risk assessment choose recommendations from observed evidence only.
- Operator permissions and policy are checked before execution. Actions requiring approval remain pending until an authorized approver acts.
- The recovery engine applies timeouts, bounded retries, and idempotency where the adapter supports them.
- A post-action health window decides whether recovery was verified, ineffective, or unresolved.
- Every decision and state transition is recorded in the audit log.
- Dry-run is the default for a new environment binding. Do not enable automatic execution until action capabilities, rollback, and verification have been exercised.

A Python recommendation or successful adapter call is not proof that the incident is resolved.

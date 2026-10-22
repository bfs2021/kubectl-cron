# Findings rules

Rules have stable kebab-case IDs. Findings identify a configuration or calculation issue; they carry no severity and prescribe no fix. Their message wording can change. Scripts should match `rule` rather than parse the message.

| Rule | Evidence | Meaning |
| --- | --- | --- |
| `invalid-schedule` | The controller-compatible parser rejects the expression. | A dated preview cannot be calculated. |
| `embedded-timezone` | An expression contains a `TZ=` or `CRON_TZ=` field. | Embedded timezone declarations are unsupported by the CronJob API. |
| `timezone-not-set` | The CronJob has no `spec.timeZone`. | The controller's timezone is unknown; an explicit assumption, if supplied, remains an assumption. |
| `timezone-unavailable` | The client cannot resolve a portable configured timezone. | This client cannot calculate a preview. This does not establish controller failure. |
| `no-next-occurrence` | The parser returns no next time in its bounded search. | No occurrence was found in that search; mathematical impossibility is not asserted. |
| `relative-anchor-unavailable` | A relative schedule lacks a recorded reference timestamp. | Its dated preview would require inventing a phase. |
| `sub-minute-schedule` | A relative schedule repeats more often than once per minute. | The cadence is finer than the controller's minute-based Job naming. |

`--problems` means rows with Findings, including uncertainty. It does not mean proven unhealthy. `--no-findings` removes Findings from every output format while retaining calculation reasons and timezone provenance. It cannot be combined with `--problems` or `--exit-on-findings`.

Suspension suppresses the upcoming execution display, but does not hide invalid configuration. The Overview and `runs` use configuration checks. `inspect` and `diagnose` additionally evaluate retained runtime evidence:

| Rule | Evidence | Meaning |
| --- | --- | --- |
| `retained-job-failures` | Owned Jobs with terminal `Failed=True`. | Reports a retained failure sample, with the newest Job and terminal observation. Failed retry Pods alone do not trigger it. |
| `concurrent-active-jobs` | More than one retained nonterminal owned Job reports active Pods. | Reports observed concurrency and current policy; manual Jobs are included. It does not assert runaway growth. |
| `duration-exceeds-current-gap` | A recorded completed Job duration is at least the gap between the next two current-spec occurrences. | Gives a dated comparison, without asserting historical policy or permanently skipped executions. |
| `external-job-controller` | An owned Job declares another `managedBy` controller. | Built-in Job status interpretation is unavailable. |
| `failed-history-disabled` | Effective failed history limit is zero. | The setting requests no retained terminal failures; it does not explain any particular deletion. |
| `successful-history-disabled` | Effective successful history limit is zero. | The setting requests no retained successes. |
| `job-already-active-event` | A UID-scoped retained `JobAlreadyActive` Event. | Reports the controller observation, without assigning individual slots. |
| `missed-schedule-event` | A UID-scoped retained `MissSchedule` Event. | Reports the Event's message and last observation, without inventing a slot count. |
| `too-many-missed-times-event` | A UID-scoped retained `TooManyMissedTimes` Event. | Reports the warning; subsequent controller behavior remains unknown. |
| `controller-warning-event` | Another UID-scoped Warning Event. | Reports its reason, message, and observation time. |

Events can expire and retained Jobs can be removed by several mechanisms. Neither their absence nor current history settings establish an exact missed-run count, a failure streak, replacement causes, or historical suspension duration. A `SuccessfulDelete` Event does not establish replacement, even under current `Replace` policy. Unavailable reads appear separately from Findings so empty Findings never imply complete evidence or proven health.

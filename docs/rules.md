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

Suspension suppresses the upcoming execution display, but does not hide invalid configuration. v0.1 collects no Jobs, Pods, or Events, and makes no inference about historical failure streaks, exact missed-run counts, replacement causes, or deleted evidence.

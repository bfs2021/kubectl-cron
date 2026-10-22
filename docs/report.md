# Report contract

JSON and YAML encode the same model with `apiVersion: cron.kubectl.dev/v1alpha1`. The kind is `CronJobReport` for an Overview and `ScheduleExplanation` for `explain`. `generatedAt` is the single evaluation instant, in UTC. `source` is `cluster`, `file`, or `expression`.

An Overview has a `cronJobs` array and, by default, a factual `summary`. Summary-only omits `cronJobs`; an empty normal selection retains `cronJobs: []`. A bare-expression Explanation has an `explanation` object. File-based Explanation has `cronJobs`, each containing the requested occurrences, and no Summary.

Each schedule includes its expression, description, timezone provenance, and next occurrence. Dated previews preserve UTC offsets. Recorded status timestamps are normalized to UTC.

```json
{
  "timezone": {"name": null, "source": "controller-unknown"},
  "next": {"time": null, "reason": "controller-timezone-unknown"}
}
```

Timezone sources are `spec`, `assumed`, `controller-unknown`, `offline-default`, and `argument`. Relative schedules additionally expose `anchor` and `anchorSource` (`creation`, `last-schedule`, or `evaluation`). No relative prediction is made without a reference point.

Absent status timestamps use `time: null` and `reason: not-recorded`. Other unavailable-next reasons are `suspended`, `invalid-schedule`, `embedded-timezone`, `timezone-unavailable`, `no-next-occurrence`, and `relative-anchor-unavailable`. Unknown values never become epoch timestamps or zeros that imply successful observation.

Findings are `{rule, message}` objects. Rule IDs are the stable interface; messages are prose. `--no-findings` omits those objects consistently. Human tables and messages can change without a schema change; consumers should check `apiVersion` and `kind`.

Exit status is 0 for completed inspection, 1 for an operational or input error, and 2 when `--exit-on-findings` is enabled and the selected objects have Findings. Operational and output-write errors take precedence. Summary-only still computes Findings for this explicit exit-code check.

`inspect` returns kind `CronJobInspection` with an `inspection` object: `cronJob`, `runs`, `retainedRuns`, `runsTruncated`, `events`, and `evidence`. A read's state is `available`, `forbidden`, `unavailable`, or `not-requested`. A null retained count means the Jobs read was unavailable; zero means the read returned no owned Jobs. Neither establishes complete historical coverage.

Each Run includes its Job UID, origin, controller, observed state, recorded nominal `slot`, creation, controller start, and finish. `schedulingDelay.seconds` is signed Job-creation-minus-slot; `duration.seconds` is Job-controller-observed elapsed time. Missing measurements have null seconds and a reason. Failed Jobs use the terminal condition's transition timestamp when no completion time exists. Intermediate termination conditions remain distinct from terminal failure or success. External controllers are identified without interpreting their status as built-in Job-controller behavior.

`runs` returns kind `CronJobRunHistory` and a `history` object with the inclusive evaluation window, current CronJob, entries, evidence availability, and truncation flags. Entries are newest first, each with `at`, `basis`, `state`, and an optional observed `run`. Basis is `recorded-slot`, `job-creation`, or `current-spec-projection`. An unmatched projection has `state: unknown`; it is not proof of a deleted or skipped execution. `retainedRuns` counts observed Jobs in the window before the display limit, and is null if the Jobs read failed. Projections are bounded to the newest 10,000 dates before the display limit; both bounds are reported. Historical configuration is unknown, and relative schedules do not project past slots.

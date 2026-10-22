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

`diagnose` returns kind `CronJobDiagnostics` with a `diagnostics` array. Each entry has namespace, name, optional Findings, and evidence-read availability, including entries with no Findings. Empty selection is an empty array. File diagnosis requests no runtime evidence. Findings are observations about the supplied or retained evidence, with no health score, severity, or prescribed fix. Inspection and diagnosis share the same runtime Rules.

`timeline` returns kind `CronJobTimeline` with the same `history` object as `runs`. Human symbols and displayed-state counts are a projection of that evidence; each symbol is an entry, without assuming a uniform interval or complete historical coverage.

`calendar` returns kind `CronJobCalendar` and a `calendar` object containing current CronJobs, the exclusive-start/inclusive-end window, slots, excluded calculations, truncation, and optional calendar Findings. Slots are chronological UTC instants with `count` and complete `cronJobs` membership for the selected scope. Members retain expressions, timezone provenance, and local dates with UTC offsets. Missing zones and suspension are explicit exclusions. The display limit applies to distinct instants, never partial groups. Coincidence Findings describe displayed nominal slots, not observed simultaneous starts.

`--watch` streams complete Overview snapshots, rather than individual Kubernetes events. Kind remains `CronJobReport`; `watch` adds `mode: api`, connection `state` (`connecting`, `resyncing`, `reconnecting`, or `streaming`), and an optional last error. JSON is one compact object per line; YAML uses `---` document separators. Each snapshot has its own evaluation timestamp. Cached objects can remain visible during reconnection. The cache applies deletions and replaces its contents after a relist, without claiming continuous event history across an expired version. Sorting, selectors, Summary projection, and Finding suppression retain their usual meanings. A Finding-triggered exit occurs after writing its snapshot; output errors take precedence.

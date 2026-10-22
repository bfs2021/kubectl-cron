# kubectl-cron

A tool for inspecting recurring Kubernetes workloads and interpreting their schedules.

## Language

**CronJob**:
A recurring workload whose schedule governs automatic Job creation.
_Avoid_: Cron, task

**Schedule**:
A recurrence that determines the intended times for a CronJob's automatic executions.
_Avoid_: Interval, frequency

**Occurrence**:
A time selected by a Schedule in its effective timezone. It expresses scheduling intent rather than observed execution.
_Avoid_: Run, Job start

**Next occurrence**:
The first Occurrence strictly after the evaluation time.
_Avoid_: Next run, predicted start

**Timezone assumption**:
An operator-supplied timezone used when a CronJob leaves its timezone unspecified. It is not a discovered controller setting.
_Avoid_: Detected timezone, cluster timezone

**Timezone provenance**:
The basis for a calculation's timezone: a CronJob's configuration, an operator's assumption, or the stated default for an offline expression.
_Avoid_: Detected timezone

**Offline source**:
CronJob definitions supplied for inspection without consulting a live cluster.
_Avoid_: Live snapshot, hydrated manifest

**Overview**:
The default view, with one row for each selected CronJob and its recorded scheduling information and configuration.
_Avoid_: Summary, dashboard

**Summary**:
Factual counts over the same selected CronJobs as the Overview.
_Avoid_: Findings, health report

**Explanation**:
An offline interpretation of a Schedule, including its upcoming occurrences.
_Avoid_: Inspection, forecast

**Finding**:
An observation produced by a Rule about a CronJob's configuration or reported state. It identifies something to investigate without prescribing an action.
_Avoid_: Warning, fix, recommendation

**Rule**:
A named condition that produces a Finding. Its identity remains stable when the wording of its observations changes.
_Avoid_: Linter, policy

**Report**:
A machine-readable rendering of an Overview or Explanation.
_Avoid_: Dump, summary

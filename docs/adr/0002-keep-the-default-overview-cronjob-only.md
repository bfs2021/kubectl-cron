# Keep the default Overview CronJob-only

The default Overview collects only CronJobs; it excludes retained-run counts and duration statistics. This keeps the common invocation inexpensive and usable with permission to read CronJobs alone. Features requiring Jobs, Pods, or Events belong to explicit richer views, since their additional API cost and incomplete retained history need a separate contract.

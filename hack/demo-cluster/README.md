# Local CronJob demo

Requires Go, kind, Docker, and kubectl. The Makefile uses only the explicit `kind-cron-demo` context for cluster operations.

```sh
make up
make cron
make chaos
make cron
make heal
make down
```

The minute marker creates a small Job each minute. The parked export is suspended. Chaos adds a calendar with no returned date and a CronJob with an unspecified controller timezone. The latter illustrates uncertainty rather than claiming an unhealthy controller.

`make offline` explains the definitions without creating a cluster. Inputs rejected by Kubernetes admission, such as malformed expressions and invalid timezone names, are tested locally rather than submitted as pretend demo workloads.

The demo applies and deletes sample resources when explicitly invoked. The plugin itself reads only CronJobs. Automated tests use fixtures; they do not start Docker or mutate a cluster.

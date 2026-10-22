// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package collect

import (
	"context"
	"sort"
	"sync"

	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	typedbatch "k8s.io/client-go/kubernetes/typed/batch/v1"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"
)

// RelatedMany reads Jobs once per namespace and scopes Events to each UID.
// Separate bounded stages avoid nesting unbounded per-object requests.
func RelatedMany(ctx context.Context, batch typedbatch.BatchV1Interface, core typedcore.CoreV1Interface, cjs []batchv1.CronJob) ([]Evidence, error) {
	namespaces := []string{}
	seen := map[string]bool{}
	for _, cj := range cjs {
		if !seen[cj.Namespace] {
			seen[cj.Namespace] = true
			namespaces = append(namespaces, cj.Namespace)
		}
	}
	sort.Strings(namespaces)
	type bucket struct {
		jobs         []batchv1.Job
		availability model.Availability
	}
	buckets := make([]bucket, len(namespaces))
	bounded(ctx, len(namespaces), 4, func(i int) { buckets[i].jobs, buckets[i].availability = ReadJobs(ctx, batch, namespaces[i]) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	byNS := map[string]bucket{}
	for i, ns := range namespaces {
		byNS[ns] = buckets[i]
	}
	out := make([]Evidence, len(cjs))
	bounded(ctx, len(cjs), 4, func(i int) {
		cj := cjs[i]
		b := byNS[cj.Namespace]
		e := Evidence{CronJob: cj, Jobs: []batchv1.Job{}, Availability: []model.Availability{b.availability}}
		if cj.UID == "" {
			e.Availability = []model.Availability{{Resource: "jobs", State: "unavailable", Reason: "cronjob-uid-missing"}, {Resource: "events", State: "unavailable", Reason: "cronjob-uid-missing"}}
		} else {
			for _, job := range b.jobs {
				if OwnedJob(cj, job) {
					e.Jobs = append(e.Jobs, job)
				}
			}
			var source model.Availability
			e.Events, source = ReadEvents(ctx, core, cj)
			e.Availability = append(e.Availability, source)
		}
		out[i] = e
	})
	return out, ctx.Err()
}

func bounded(ctx context.Context, count, workers int, work func(int)) {
	queue := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers && i < count; i++ {
		wg.Go(func() {
			for n := range queue {
				if ctx.Err() != nil {
					return
				}
				work(n)
			}
		})
	}
send:
	for i := 0; i < count; i++ {
		select {
		case queue <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(queue)
	wg.Wait()
}

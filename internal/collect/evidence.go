// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package collect

import (
	"context"

	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	typedbatch "k8s.io/client-go/kubernetes/typed/batch/v1"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Evidence struct {
	CronJob      batchv1.CronJob
	Jobs         []batchv1.Job
	Events       []corev1.Event
	Availability []model.Availability
}

func OwnedJob(cj batchv1.CronJob, job batchv1.Job) bool {
	owner := metav1.GetControllerOf(&job)
	return cj.UID != "" && job.Namespace == cj.Namespace && owner != nil && owner.Kind == "CronJob" && owner.UID == cj.UID
}

func ReadJobs(ctx context.Context, client typedbatch.BatchV1Interface, namespace string) ([]batchv1.Job, model.Availability) {
	result := []batchv1.Job{}
	query := metav1.ListOptions{Limit: 500}
	for {
		page, err := client.Jobs(namespace).List(ctx, query)
		if err != nil {
			return nil, ReadAvailability("jobs", err)
		}
		result = append(result, page.Items...)
		if page.Continue == "" {
			break
		}
		query.Continue = page.Continue
	}
	return result, ReadAvailability("jobs", nil)
}

func ReadEvents(ctx context.Context, client typedcore.CoreV1Interface, cj batchv1.CronJob) ([]corev1.Event, model.Availability) {
	result := []corev1.Event{}
	query := metav1.ListOptions{Limit: 500, FieldSelector: fields.OneTermEqualSelector("involvedObject.uid", string(cj.UID)).String()}
	for {
		page, err := client.Events(cj.Namespace).List(ctx, query)
		if err != nil {
			return nil, ReadAvailability("events", err)
		}
		for _, event := range page.Items {
			if event.InvolvedObject.UID == cj.UID && event.Namespace == cj.Namespace {
				result = append(result, event)
			}
		}
		if page.Continue == "" {
			break
		}
		query.Continue = page.Continue
	}
	return result, ReadAvailability("events", nil)
}

func Related(ctx context.Context, batch typedbatch.BatchV1Interface, core typedcore.CoreV1Interface, cj batchv1.CronJob, events bool) (Evidence, error) {
	e := Evidence{CronJob: cj, Jobs: []batchv1.Job{}, Events: []corev1.Event{}}
	if cj.UID == "" {
		for _, resource := range []string{"jobs", "events"} {
			e.Availability = append(e.Availability, model.Availability{Resource: resource, State: "unavailable", Reason: "cronjob-uid-missing"})
		}
		return e, nil
	}
	jobs, availability := ReadJobs(ctx, batch, cj.Namespace)
	if err := ctx.Err(); err != nil {
		return e, err
	}
	e.Availability = append(e.Availability, availability)
	for _, job := range jobs {
		if OwnedJob(cj, job) {
			e.Jobs = append(e.Jobs, job)
		}
	}
	if events {
		items, availability := ReadEvents(ctx, core, cj)
		if err := ctx.Err(); err != nil {
			return e, err
		}
		e.Events = items
		e.Availability = append(e.Availability, availability)
	} else {
		e.Availability = append(e.Availability, model.Availability{Resource: "events", State: "not-requested"})
	}
	return e, nil
}

func ReadAvailability(resource string, err error) model.Availability {
	if err == nil {
		return model.Availability{Resource: resource, State: "available"}
	}
	state, reason := "unavailable", "read-failed"
	if apierrors.IsForbidden(err) {
		state = "forbidden"
		reason = "permission-denied"
	}
	if apierrors.IsUnauthorized(err) {
		reason = "unauthorized"
	}
	return model.Availability{Resource: resource, State: state, Reason: reason, Message: err.Error()}
}

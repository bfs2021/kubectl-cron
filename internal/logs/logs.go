// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package logs resolves owned execution logs and bounds concurrent API streams.
package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Selection struct {
	Run                 string
	Offset              int
	Failed, All, Follow bool
}

// Select includes owned manual Jobs. The caller supplies only UID-verified Jobs.
func Select(jobs []batchv1.Job, opts Selection) ([]batchv1.Job, error) {
	if opts.Offset < 0 {
		return nil, fmt.Errorf("run offset must be nonnegative")
	}
	ordered := append([]batchv1.Job(nil), jobs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
			return a.CreationTimestamp.After(b.CreationTimestamp.Time)
		}
		return a.Name < b.Name
	})
	selected := []batchv1.Job{}
	for _, job := range ordered {
		r := analyze.Run(job)
		if opts.Run != "" && job.Name != opts.Run {
			continue
		}
		if opts.Failed && r.State != "failed" {
			continue
		}
		if opts.Follow && opts.Run == "" && !opts.All && !opts.Failed && job.Status.Active == 0 {
			continue
		}
		selected = append(selected, job)
	}
	if opts.All && len(selected) > 0 {
		return selected, nil
	}
	if opts.Offset >= len(selected) {
		return nil, fmt.Errorf("no retained owned Job matches the log selection; history may be incomplete")
	}
	return selected[opts.Offset : opts.Offset+1], nil
}

type Target struct{ Namespace, Job, Pod, Container string }

func (t Target) label() string { return t.Namespace + "/" + t.Job + "/" + t.Pod + "/" + t.Container }

func Resolve(ctx context.Context, client typedcore.CoreV1Interface, jobs []batchv1.Job, container string, allContainers bool) ([]Target, []string, error) {
	targets, notes := []Target{}, []string{}
	for _, job := range jobs {
		if job.UID == "" {
			return nil, nil, fmt.Errorf("Job %s has no UID; Pod ownership cannot be verified", job.Name)
		}
		selector := labels.Set{batchv1.ControllerUidLabel: string(job.UID)}.AsSelector()
		if job.Spec.Selector != nil {
			var err error
			selector, err = metav1.LabelSelectorAsSelector(job.Spec.Selector)
			if err != nil {
				return nil, nil, fmt.Errorf("Job %s selector: %w", job.Name, err)
			}
		}
		query := metav1.ListOptions{Limit: 500, LabelSelector: selector.String()}
		pods := []corev1.Pod{}
		for {
			page, err := client.Pods(job.Namespace).List(ctx, query)
			if err != nil {
				return nil, nil, fmt.Errorf("read Pods for Job %s: %w", job.Name, err)
			}
			for _, pod := range page.Items {
				owner := metav1.GetControllerOf(&pod)
				if owner != nil && owner.Kind == "Job" && owner.UID == job.UID && pod.Namespace == job.Namespace {
					pods = append(pods, pod)
				}
			}
			if page.Continue == "" {
				break
			}
			query.Continue = page.Continue
		}
		if len(pods) == 0 {
			notes = append(notes, fmt.Sprintf("No retained owned Pods for Job %s/%s; deletion or absence cause is not recorded.", job.Namespace, job.Name))
			continue
		}
		sort.Slice(pods, func(i, j int) bool { return pods[i].Name < pods[j].Name })
		for _, pod := range pods {
			names := []string{}
			for _, c := range pod.Spec.Containers {
				names = append(names, c.Name)
			}
			for _, c := range pod.Spec.InitContainers {
				names = append(names, c.Name)
			}
			for _, c := range pod.Spec.EphemeralContainers {
				names = append(names, c.Name)
			}
			chosen := names
			if !allContainers {
				name := container
				if name == "" {
					name = pod.Annotations["kubectl.kubernetes.io/default-container"]
					if !contains(names, name) {
						if len(pod.Spec.Containers) == 0 {
							return nil, nil, fmt.Errorf("Pod %s has no default regular container; specify --container", pod.Name)
						}
						name = pod.Spec.Containers[0].Name
						if len(names) > 1 {
							notes = append(notes, fmt.Sprintf("Pod %s: defaulting to container %s; use --container or --all-containers to select others.", pod.Name, name))
						}
					}
				}
				if !contains(names, name) {
					return nil, nil, fmt.Errorf("container %q not found in Pod %s", name, pod.Name)
				}
				chosen = []string{name}
			}
			for _, name := range chosen {
				targets = append(targets, Target{Namespace: job.Namespace, Job: job.Name, Pod: pod.Name, Container: name})
			}
		}
	}
	if len(targets) == 0 {
		return nil, notes, fmt.Errorf("no retained owned Pods with selected containers; log availability and deletion causes cannot be inferred from history limits")
	}
	return targets, notes, nil
}

func contains(names []string, name string) bool {
	for _, v := range names {
		if v == name {
			return true
		}
	}
	return false
}

type StreamOptions struct {
	Pod         corev1.PodLogOptions
	MaxRequests int
	Prefix      bool
}

func Stream(ctx context.Context, client typedcore.CoreV1Interface, targets []Target, opts StreamOptions, out io.Writer) error {
	if opts.MaxRequests < 1 {
		return fmt.Errorf("maximum log requests must be positive")
	}
	if opts.Pod.Follow && len(targets) > opts.MaxRequests {
		return fmt.Errorf("following %d streams exceeds --max-log-requests=%d; increase it or narrow the selection", len(targets), opts.MaxRequests)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var output sync.Mutex
	queue := make(chan Target)
	var wg sync.WaitGroup
	var first error
	var failed sync.Once
	fail := func(err error) { failed.Do(func() { first = err; cancel() }) }
	for i := 0; i < opts.MaxRequests && i < len(targets); i++ {
		wg.Go(func() {
			for target := range queue {
				if ctx.Err() != nil {
					return
				}
				podOpts := opts.Pod
				podOpts.Container = target.Container
				stream, err := client.Pods(target.Namespace).GetLogs(target.Pod, &podOpts).Stream(ctx)
				if err != nil {
					fail(fmt.Errorf("logs %s: %w", target.label(), err))
					return
				}
				err = copyLines(stream, func(line string) error {
					output.Lock()
					defer output.Unlock()
					if opts.Prefix || len(targets) > 1 {
						if !strings.HasSuffix(line, "\n") {
							line += "\n"
						}
						line = "[" + target.label() + "] " + line
					}
					_, err := io.WriteString(out, line)
					return err
				})
				closeErr := stream.Close()
				if err == nil {
					err = closeErr
				}
				if err != nil {
					fail(fmt.Errorf("logs %s: %w", target.label(), err))
					return
				}
			}
		})
	}
send:
	for _, target := range targets {
		select {
		case queue <- target:
		case <-ctx.Done():
			break send
		}
	}
	close(queue)
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

// Keep buffering bounded and fail explicitly rather than truncate long lines.
func copyLines(in io.Reader, write func(string) error) error {
	r := bufio.NewReaderSize(in, 32*1024)
	var line strings.Builder
	for {
		part, err := r.ReadSlice('\n')
		if len(part) > 0 {
			line.Write(part)
		}
		if line.Len() > 1024*1024 {
			return fmt.Errorf("log line exceeds the 1 MiB limit")
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if line.Len() > 0 {
			if e := write(line.String()); e != nil {
				return e
			}
			line.Reset()
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

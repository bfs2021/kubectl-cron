// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package collect reads CronJobs through either a live API or offline files.
package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	typedbatch "k8s.io/client-go/kubernetes/typed/batch/v1"
)

type Options struct {
	Namespace         string
	NamespaceExplicit bool
	AllNamespaces     bool
	Selector          string
	Name              string
	Filenames         []string
	Stdin             io.Reader
}

func Live(ctx context.Context, client typedbatch.BatchV1Interface, opts Options) ([]batchv1.CronJob, error) {
	ns := opts.Namespace
	if opts.AllNamespaces {
		ns = ""
	}
	selector, err := labels.Parse(opts.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid CronJob selector: %w", err)
	}
	if opts.Name != "" {
		if opts.AllNamespaces {
			return nil, fmt.Errorf("a named CronJob requires a namespace; omit --all-namespaces")
		}
		job, err := client.CronJobs(ns).Get(ctx, opts.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if !selector.Matches(labels.Set(job.Labels)) {
			return []batchv1.CronJob{}, nil
		}
		return []batchv1.CronJob{*job}, nil
	}
	var result []batchv1.CronJob
	query := metav1.ListOptions{LabelSelector: opts.Selector, Limit: 500}
	for {
		page, err := client.CronJobs(ns).List(ctx, query)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if page.Continue == "" {
			break
		}
		query.Continue = page.Continue
	}
	return result, nil
}

func Files(opts Options) ([]batchv1.CronJob, error) {
	selector, err := labels.Parse(opts.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid CronJob selector: %w", err)
	}
	var jobs []batchv1.CronJob
	usedStdin := false
	for _, filename := range opts.Filenames {
		if filename == "-" {
			if usedStdin {
				return nil, fmt.Errorf("stdin can only be read once")
			}
			usedStdin = true
			if opts.Stdin == nil {
				return nil, fmt.Errorf("stdin is unavailable")
			}
			decoded, err := Read(opts.Stdin)
			if err != nil {
				return nil, fmt.Errorf("stdin: %w", err)
			}
			jobs = append(jobs, decoded...)
			continue
		}
		if strings.Contains(filename, "://") {
			return nil, fmt.Errorf("remote input %q is unsupported; file input is offline", filename)
		}
		file, err := os.Open(filename)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", filename, err)
		}
		decoded, readErr := Read(file)
		closeErr := file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %q: %w", filename, readErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		jobs = append(jobs, decoded...)
	}
	selected := make([]batchv1.CronJob, 0, len(jobs))
	for _, job := range jobs {
		if opts.NamespaceExplicit {
			if job.Namespace != "" && job.Namespace != opts.Namespace {
				return nil, fmt.Errorf("CronJob %s declares namespace %q, which conflicts with --namespace=%s", job.Name, job.Namespace, opts.Namespace)
			}
			job.Namespace = opts.Namespace
		}
		if opts.Name != "" && job.Name != opts.Name {
			continue
		}
		if !selector.Matches(labels.Set(job.Labels)) {
			continue
		}
		selected = append(selected, job)
	}
	return selected, nil
}

// Read intentionally avoids a REST mapper or discovery client.
func Read(r io.Reader) ([]batchv1.CronJob, error) {
	decoder := utilyaml.NewYAMLOrJSONDecoder(r, 4096)
	result := []batchv1.CronJob{}
	for document := 1; ; document++ {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("document %d: %w", document, err)
		}
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		items, err := decodeObject(raw, 0)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", document, err)
		}
		result = append(result, items...)
	}
	return result, nil
}

func decodeObject(raw json.RawMessage, depth int) ([]batchv1.CronJob, error) {
	if depth > 16 {
		return nil, fmt.Errorf("nested Lists exceed 16 levels")
	}
	var header struct {
		APIVersion string            `json:"apiVersion"`
		Kind       string            `json:"kind"`
		Items      []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, err
	}
	if (header.Kind == "List" && header.APIVersion == "v1") || (header.Kind == "CronJobList" && header.APIVersion == "batch/v1") {
		result := []batchv1.CronJob{}
		for i, item := range header.Items {
			// Typed API lists omit each item's TypeMeta; only that parent can
			// supply the missing kind and apiVersion without guessing.
			if header.Kind == "CronJobList" {
				var object map[string]json.RawMessage
				if err := json.Unmarshal(item, &object); err != nil {
					return nil, fmt.Errorf("List item %d: %w", i+1, err)
				}
				if object == nil {
					return nil, fmt.Errorf("List item %d is not an object", i+1)
				}
				if len(object["kind"]) == 0 {
					object["kind"] = json.RawMessage(`"CronJob"`)
				}
				if len(object["apiVersion"]) == 0 {
					object["apiVersion"] = json.RawMessage(`"batch/v1"`)
				}
				var err error
				item, err = json.Marshal(object)
				if err != nil {
					return nil, err
				}
			}
			jobs, err := decodeObject(item, depth+1)
			if err != nil {
				return nil, fmt.Errorf("List item %d: %w", i+1, err)
			}
			result = append(result, jobs...)
		}
		return result, nil
	}
	if header.Kind != "CronJob" || header.APIVersion != "batch/v1" {
		return nil, fmt.Errorf("expected batch/v1 CronJob or List, got %q %q", header.APIVersion, header.Kind)
	}
	var job batchv1.CronJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return nil, err
	}
	return []batchv1.CronJob{job}, nil
}

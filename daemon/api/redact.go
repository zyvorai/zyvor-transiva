// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	"github.com/zyvorai/transiva/daemon/models"
)

// redactedValue replaces a secret in API responses.
const redactedValue = "***"

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "private_key"} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

// redactMetadata returns a copy of a job's metadata with credential values replaced. Jobs carry the
// Prism or vCenter credentials the dashboard sent, and they must not come back out of the API.
func redactMetadata(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		switch {
		case isSecretKey(k):
			out[k] = redactedValue
		default:
			if nested, ok := v.(map[string]interface{}); ok {
				out[k] = redactMetadata(nested)
			} else {
				out[k] = v
			}
		}
	}
	return out
}

// redactedJob returns a copy of job that is safe to send to API clients.
func redactedJob(job *models.Job) *models.Job {
	if job == nil {
		return nil
	}
	c := *job
	c.Definition.Metadata = redactMetadata(job.Definition.Metadata)
	return &c
}

func redactedJobs(jobs []*models.Job) []*models.Job {
	out := make([]*models.Job, len(jobs))
	for i, j := range jobs {
		out[i] = redactedJob(j)
	}
	return out
}

/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	justintimev1 "jira-jit-rbac-operator/api/v1"

	corev1 "k8s.io/api/core/v1"

	"jira-jit-rbac-operator/internal/config"
	"os"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Read operator configuration from config file
func ReadConfigFromFile() (*justintimev1.JustInTimeConfigSpec, error) {
	// common lock for concurrent reads
	config.ConfigLock.RLock()
	defer config.ConfigLock.RUnlock()

	data, err := os.ReadFile(fmt.Sprintf("%s/%s", config.ConfigCacheFilePath, config.ConfigFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var config justintimev1.JustInTimeConfigSpec
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	return &config, nil
}

// checks if a string is present in a slice.
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Validate namespace name with regex if provided
func ValidateNamespaceRegex(namespaces []string) (string, error) {
	if config.NamespaceAllowedRegex != nil {
		for _, namespace := range namespaces {
			if !config.NamespaceAllowedRegex.MatchString(namespace) {
				return namespace, field.Invalid(
					field.NewPath("spec").Child("namespace"),
					namespace,
					fmt.Sprintf("namespace does not match the allowed pattern: %s", config.NamespaceAllowedRegex.String()),
				)
			}
		}
	}
	return "", nil
}

// Validate namespace(s) have namespaceLabels
func ValidateNamespaceLabels(ctx context.Context, jitRequest *justintimev1.JitRequest, r client.Client) (string, error) {
	for _, namespace := range jitRequest.Spec.Namespaces {
		ns := &corev1.Namespace{}
		err := r.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		if err != nil {
			return namespace, fmt.Errorf("failed to get namespace %s: %v", namespace, err)
		}

		for key, value := range jitRequest.Spec.NamespaceLabels {
			if ns.Labels[key] != value {
				return namespace, fmt.Errorf("namespace %s does not have the label %s=%s", namespace, key, value)
			}
		}
	}
	return "", nil
}

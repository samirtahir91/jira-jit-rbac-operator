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
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"jira-jit-rbac-operator/internal/config"
	"os"

	jira "github.com/ctreminiom/go-atlassian/v2/jira/v2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReadConfigFromFile Reads operator configuration from config file
func ReadConfigFromFile() (*justintimev1.JustInTimeConfigSpec, error) {
	// common lock for concurrent reads
	config.ConfigLock.RLock()
	defer config.ConfigLock.RUnlock()

	data, err := os.ReadFile(fmt.Sprintf("%s/%s", config.ConfigCacheFilePath, config.ConfigFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var newConfig justintimev1.JustInTimeConfigSpec
	if err := json.Unmarshal(data, &newConfig); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	return &newConfig, nil
}

// Contains checks if a string is present in a slice.
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ValidateNamespaceRegex validates namespace name with regex if provided
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

// ValidateNamespaceLabels validates namespace(s) have namespaceLabels
func ValidateNamespaceLabels(ctx context.Context, jitRequest *justintimev1.JitRequest, k8sClient client.Client) ([]string, error) { //nolint:lll

	// if there are no namespace labels, skip and return
	if len(jitRequest.Spec.NamespaceLabels) == 0 {
		return nil, nil
	}

	// get all namespaces matching labels if defined in JitRequest
	labelSelector := labels.SelectorFromSet(jitRequest.Spec.NamespaceLabels)
	namespaceList := &corev1.NamespaceList{}
	err := k8sClient.List(ctx, namespaceList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	// make map of matching namespaces
	validNamespaces := make(map[string]struct{})
	for _, ns := range namespaceList.Items {
		validNamespaces[ns.Name] = struct{}{}
	}

	// filter out non-matching namespaces
	var invalidNamespaces []string
	for _, namespace := range jitRequest.Spec.Namespaces {
		if _, found := validNamespaces[namespace]; !found {
			invalidNamespaces = append(invalidNamespaces, namespace)
		}
	}

	// fmt label msg for error
	labelPairs := make([]string, 0, len(jitRequest.Spec.NamespaceLabels))
	for key, value := range jitRequest.Spec.NamespaceLabels {
		labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", key, value))
	}
	labelString := strings.Join(labelPairs, ", ")

	// return invalid namespaces if any
	if len(invalidNamespaces) > 0 {
		return invalidNamespaces, fmt.Errorf(
			"the following namespaces do not match the specified labels (%s): %v",
			labelString, invalidNamespaces,
		)
	}

	return nil, nil
}

// GetNameByEmail gets and returns ID for as Jira user by email - gets the 1st result
func GetNameByEmail(email string, jiraClient *jira.Client) (string, error) {

	type User struct {
		Name string `json:"name"`
	}

	// RAW endpoint
	apiEndpoint := fmt.Sprintf("rest/api/2/user/search?username=%s", email)
	request, err := jiraClient.NewRequest(context.Background(), http.MethodGet, apiEndpoint, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to find account name for reporter email: %w", err)
	}

	var users []User
	response, err := jiraClient.Call(request, &users)
	if err != nil {
		if response != nil {
			body := response.Bytes.String()
			return "", fmt.Errorf("failed to find account name for reporter email: %w, response: %s", err, body)
		} else {
			return "", fmt.Errorf("failed to find account name for reporter email: %w, response: nil response", err)
		}
	}

	// check if any users were found
	if len(users) == 0 {
		return "", fmt.Errorf("no users found with email: %s", email)
	}

	// get the account name
	accountId := users[0].Name
	return accountId, nil
}

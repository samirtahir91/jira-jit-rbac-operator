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

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/pkg/configuration"
)

var (
	ConfigCacheFilePath string
	ConfigFile          = "config.json"
	ConfigLock          sync.RWMutex
)

// JustInTimeConfigReconciler reconciles a JustInTimeConfig object
type JustInTimeConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile
func (c *JustInTimeConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("JustInTimeConfig reconciliation started", "request.name", req.Name)

	cfg := configuration.NewJitRbacOperatorConfiguration(ctx, c.Client, req.Name)
	l.Info(
		"JustInTimeConfig",
		"allowed cluster roles",
		cfg.AllowedClusterRoles(),
		"jira workflow approved name",
		cfg.JiraWorkflowApproveStatus(),
		"jira reject transition id",
		cfg.RejectedTransitionID(),
		"jira project",
		cfg.JiraProject(),
		"jira issue type",
		cfg.JiraIssueType(),
		"jira approve transition id",
		cfg.ApprovedTransitionID(),
		"jira custom fields",
		cfg.CustomFields(),
		"jira required fields",
		cfg.RequiredFields(),
		"environment",
		cfg.Environment(),
		"labels",
		cfg.Labels(),
		"additional comments",
		cfg.AdditionalCommentText(),
	)

	// cache config to file
	if err := c.SaveConfigToFile(ctx, cfg, ConfigCacheFilePath, ConfigFile); err != nil {
		l.Error(err, "failed to save configuration to file")
		return ctrl.Result{}, err
	}

	l.Info("JustInTimeConfig reconciliation finished", "request.name", req.Name)

	return ctrl.Result{}, nil
}

// Save configuration to a file
func (c *JustInTimeConfigReconciler) SaveConfigToFile(ctx context.Context, cfg configuration.Configuration, filePath string, fileName string) error {
	l := log.FromContext(ctx)
	// Create dir if does not exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := os.MkdirAll(filePath, 0700); err != nil {
			return fmt.Errorf("failed to create config directory: %v", err)
		}
	}

	// common lock (config file is read by jitrequest controller reconciles)
	ConfigLock.Lock()
	defer ConfigLock.Unlock()

	configData := justintimev1.JustInTimeConfigSpec{
		AllowedClusterRoles:       cfg.AllowedClusterRoles(),
		JiraWorkflowApproveStatus: cfg.JiraWorkflowApproveStatus(),
		RejectedTransitionID:      cfg.RejectedTransitionID(),
		JiraProject:               cfg.JiraProject(),
		JiraIssueType:             cfg.JiraIssueType(),
		ApprovedTransitionID:      cfg.ApprovedTransitionID(),
		CustomFields:              cfg.CustomFields(),
		RequiredFields:            cfg.RequiredFields(),
		Environment:               cfg.Environment(),
		Labels:                    cfg.Labels(),
		AdditionalCommentText:     cfg.AdditionalCommentText(),
	}

	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize configuration: %w", err)
	}

	file, err := os.Create(fmt.Sprintf("%s/%s", filePath, fileName))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	defer func() {
		err := file.Close()
		if err != nil {
			l.Error(err, "error closing config file")
		}
	}()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	return nil
}

// Only use JustInTimeConfig named as per param
func nameMatchPredicate(name string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == name
	})
}

// SetupWithManager sets up the controller with the Manager.
func (c *JustInTimeConfigReconciler) SetupWithManager(mgr ctrl.Manager, configurationName string, configCacheFilePath string) error {
	ConfigCacheFilePath = configCacheFilePath
	return ctrl.NewControllerManagedBy(mgr).
		For(&justintimev1.JustInTimeConfig{},
			builder.WithPredicates(
				predicate.ResourceVersionChangedPredicate{},
				nameMatchPredicate(configurationName),
			)).
		Named("justintimeconfig").
		Complete(c)
}

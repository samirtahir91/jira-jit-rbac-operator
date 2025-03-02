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

package controller

import (
	"context"
	"os"
	"strings"

	jira "github.com/ctreminiom/go-atlassian/v2/jira/v2"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder" // Required for Watching
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/pkg/utils"
)

var OperatorNamespace = os.Getenv("OPERATOR_NAMESPACE")

// JitRequestReconciler reconciles a JitRequest object
type JitRequestReconciler struct {
	JiraClient *jira.Client
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is the main loop for reconciling a JitRequest
func (r *JitRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the JitRequest instance
	jitRequest, err := r.fetchJitRequest(ctx, req.NamespacedName)
	if err != nil {
		return r.handleFetchError(ctx, l, err, jitRequest)
	}

	// Fetch operator config
	operatorConfig, err := utils.ReadConfigFromFile()
	if err != nil {
		return ctrl.Result{}, err
	}
	jiraWorkflowApproveStatus := operatorConfig.JiraWorkflowApproveStatus
	rejectedTransitionID := operatorConfig.RejectedTransitionID
	allowedClusterRoles := operatorConfig.AllowedClusterRoles
	jiraProject := operatorConfig.JiraProject
	jiraIssueType := operatorConfig.JiraIssueType
	completedTransitionID := operatorConfig.CompletedTransitionID
	customFieldsConfig := operatorConfig.CustomFields
	requiredFieldsConfig := operatorConfig.RequiredFields
	ticketLabels := operatorConfig.Labels
	targetEnvironment := operatorConfig.Environment
	additionalComments := operatorConfig.AdditionalCommentText

	l.Info("Got JitRequest", "Requestor", jitRequest.Spec.Reporter, "Role", jitRequest.Spec.ClusterRole, "Namespace", strings.Join(jitRequest.Spec.Namespaces, ", "))

	// Handle JitRequest based on its status
	switch jitRequest.Status.State {
	case StatusRejected:
		return r.handleRejected(ctx, l, jitRequest, rejectedTransitionID)
	case "":
		return r.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)
	case StatusPreApproved:
		return r.handlePreApproved(ctx, l, jitRequest, completedTransitionID, jiraWorkflowApproveStatus)
	case StatusSucceeded:
		return r.handleCleanup(ctx, l, jitRequest)
	default:
		return r.handleCleanup(ctx, l, jitRequest)
	}
}

// jitRequestPredicate filters events for JitRequest objects and ignores is StatusRejected is identical for update events
func jitRequestPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJitRequest := e.ObjectOld.(*justintimev1.JitRequest)
			newJitRequest := e.ObjectNew.(*justintimev1.JitRequest)

			if oldJitRequest.Status.State == StatusRejected &&
				newJitRequest.Status.State == StatusRejected {
				return false
			}

			return newJitRequest.Status.State == StatusRejected
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *JitRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&justintimev1.JitRequest{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, jitRequestPredicate())).
		Named("jitrequest").
		Complete(r)
}

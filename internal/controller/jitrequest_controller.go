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
	"encoding/json"
	"fmt"
	"os"
	"time"

	jira "github.com/ctreminiom/go-atlassian/v2/jira/v2"
	"github.com/ctreminiom/go-atlassian/v2/pkg/infra/models"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder" // Required for Watching
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	v1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/internal/config"
)

var (
	OperatorNamespace = os.Getenv("OPERATOR_NAMESPACE")
)

const (
	StatusRejected    = "Rejected"
	StatusPreApproved = "Pre-Approved"
	StatusSucceeded   = "Succeeded"
)

// JitRequestReconciler reconciles a JitRequest object
type JitRequestReconciler struct {
	JiraClient *jira.Client
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	ConfigurationName string
}

// Reconcile loop
func (r *JitRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the JitRequest instance
	jitRequest, err := r.fetchJitRequest(ctx, req.NamespacedName)
	if err != nil {
		return r.handleFetchError(ctx, l, err, jitRequest)
	}

	// Fetch operator config
	operatorConfig, err := r.readConfigFromFile(config.ConfigCacheFilePath, config.ConfigFile)
	if err != nil {
		return ctrl.Result{}, err
	}
	jiraWorkflowApproveStatus := operatorConfig.JiraWorkflowApproveStatus
	rejectedTransitionID := operatorConfig.RejectedTransitionID
	allowedClusterRoles := operatorConfig.AllowedClusterRoles
	jiraProject := operatorConfig.JiraProject
	jiraIssueType := operatorConfig.JiraIssueType
	approvedTransitionID := operatorConfig.ApprovedTransitionID
	customFieldsConfig := operatorConfig.CustomFields
	requiredFieldsConfig := operatorConfig.RequiredFields

	l.Info("Got JitRequest", "Requestor", jitRequest.Spec.Reporter, "Role", jitRequest.Spec.ClusterRole, "Namespace", jitRequest.Spec.Namespace)

	// Handle JitRequest based on its status
	switch jitRequest.Status.State {
	case StatusRejected:
		return r.handleRejected(ctx, l, jitRequest, rejectedTransitionID)
	case "":
		return r.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig)
	case StatusPreApproved:
		return r.handlePreApproved(ctx, l, jitRequest, approvedTransitionID, jiraWorkflowApproveStatus)
	case StatusSucceeded:
		return r.handleCleaup(ctx, l, jitRequest)
	default:
		return r.handleCleaup(ctx, l, jitRequest)
	}
}

// Fetch a JitRequest
func (r *JitRequestReconciler) fetchJitRequest(ctx context.Context, namespacedName types.NamespacedName) (*justintimev1.JitRequest, error) {
	jitRequest := &justintimev1.JitRequest{}
	err := r.Get(ctx, namespacedName, jitRequest)
	return jitRequest, err
}

// Cleanup owned objects (rolebindings) on deleted JitRequests
func (r *JitRequestReconciler) handleFetchError(
	ctx context.Context,
	l logr.Logger,
	err error,
	jitRequest *justintimev1.JitRequest,
) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		l.Info("JitRequest resource not found. Deleting managed objects.")
		if err := r.deleteOwnedObjects(ctx, jitRequest); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	l.Error(err, "failed to get JitRequest")
	return ctrl.Result{}, err
}

// Read operator configuration from config file
func (r *JitRequestReconciler) readConfigFromFile(filePath string, fileName string) (*v1.JustInTimeConfigSpec, error) {
	// common lock for concurrent reads
	config.ConfigLock.RLock()
	defer config.ConfigLock.RUnlock()

	data, err := os.ReadFile(fmt.Sprintf("%s/%s", filePath, fileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var config v1.JustInTimeConfigSpec
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	return &config, nil
}

// Reject Jira ticket and delete JitRequest
func (r *JitRequestReconciler) handleRejected(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	rejectedTransitionID string,
) (ctrl.Result, error) {
	// Reject jira ticket
	if jitRequest.Status.JiraTicket != "Skipped" {
		if err := r.rejectJiraTicket(ctx, jitRequest, rejectedTransitionID); err != nil {
			l.Error(err, "failed to reject jira ticket")
			return ctrl.Result{}, err
		}
	}
	// Delete JitRequest
	if err := r.deleteJitRequest(ctx, jitRequest); err != nil {
		l.Error(err, "failed to delete JitRequest")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Create new Jira ticket for new JitRequests, validate config
func (r *JitRequestReconciler) handleNewRequest(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	allowedClusterRoles []string,
	jiraProject,
	jiraIssueType string,
	customFieldsConfig map[string]v1.CustomFieldSettings,
	requiredFieldsConfig *v1.RequiredFieldsSpec,
) (ctrl.Result, error) {
	jiraIssueKey, err := r.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig)
	if err != nil {
		l.Error(err, "failed to createJiraTicket")
		return ctrl.Result{}, err
	}

	// Return if missing jira field
	if jiraIssueKey == "Skipped" {
		return ctrl.Result{}, nil
	}

	// check cluster role is allowed
	if !contains(allowedClusterRoles, jitRequest.Spec.ClusterRole) {
		return r.rejectInvalidRole(ctx, l, jitRequest, jiraIssueKey)
	}

	return r.preApproveRequest(ctx, l, jitRequest, jiraIssueKey)
}

// Reject an invalid cluster role
func (r *JitRequestReconciler) rejectInvalidRole(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	jiraIssueKey string,
) (ctrl.Result, error) {
	errorMsg := fmt.Sprintf("ClusterRole '%s' is not allowed", jitRequest.Spec.ClusterRole)
	r.raiseEvent(jitRequest, "Warning", "ValidationFailed", errorMsg)
	if err := r.updateStatus(ctx, jitRequest, StatusRejected, errorMsg, jiraIssueKey, 3); err != nil {
		l.Error(err, "failed to update status to Rejected")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Pre-approve the JitRequest, update the Jira ticke and queue for start time
func (r *JitRequestReconciler) preApproveRequest(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	jiraIssueKey string,
) (ctrl.Result, error) {
	startTime := jitRequest.Spec.StartTime.Time
	if startTime.After(time.Now()) {
		// update status and event
		r.raiseEvent(jitRequest, "Normal", StatusPreApproved, fmt.Sprintf("ClusterRole '%s' is allowed", jitRequest.Spec.ClusterRole))
		if err := r.updateStatus(ctx, jitRequest, StatusPreApproved, "Pre-approval - Access will be granted at start time pending human approval(s)", jiraIssueKey, 3); err != nil {
			l.Error(err, "failed to update status to Pre-Approved")
			return ctrl.Result{}, err
		}

		// update jira with comment
		jiraTicket := jitRequest.Status.JiraTicket
		comment := (jitRequest.Status.Message + "\nNamespace: " + jitRequest.Spec.Namespace)
		if err := r.updateJiraTicket(ctx, jiraTicket, comment); err != nil {
			return ctrl.Result{}, err
		}

		// requeue for start time
		delay := time.Until(startTime)
		l.Info("Start time not reached, requeuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// invalid start time, reject
	errMsg := fmt.Errorf("start time %s must be after current time", jitRequest.Spec.StartTime.Time)
	if err := r.updateStatus(ctx, jitRequest, StatusRejected, errMsg.Error(), jiraIssueKey, 3); err != nil {
		l.Error(err, "failed to update status to Rejected")
		return ctrl.Result{}, err
	}

	l.Error(errMsg, "start time validation failed")
	return ctrl.Result{}, nil
}

// Create the rolebinding for approved JitRequests if the Jira is approved
func (r *JitRequestReconciler) handlePreApproved(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	approvedTransitionID, jiraWorkflowApproveStatus string,
) (ctrl.Result, error) {
	// check if needs to be re-queued
	startTime := jitRequest.Spec.StartTime.Time
	if startTime.After(time.Now()) {
		// requeue for start time
		delay := time.Until(startTime)
		l.Info("Start time not reached, requeuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	jiraTicket := jitRequest.Status.JiraTicket
	if err := r.getJiraApproval(ctx, jitRequest, jiraWorkflowApproveStatus); err != nil {
		l.Error(err, StatusRejected, "jira ticket", jiraTicket)
		if err := r.updateStatus(ctx, jitRequest, StatusRejected, "Jira ticket has not been approved", jiraTicket, 3); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	l.Info("Creating role binding")
	if err := r.createRbac(ctx, jitRequest); err != nil {
		l.Error(err, "failed to create rbac for JIT request")
		r.raiseEvent(jitRequest, "Warning", "FailedRBAC", fmt.Sprintf("Error: %s", err))
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, jitRequest, StatusSucceeded, "Access granted until end time", jiraTicket, 3); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.completeJiraTicket(ctx, jitRequest, approvedTransitionID); err != nil {
		return ctrl.Result{}, err
	}

	// Queue for deletion at end time
	return r.handleCleaup(ctx, l, jitRequest)
}

// Handle and queue succeeded and unknown JitRequests for deletion
func (r *JitRequestReconciler) handleCleaup(ctx context.Context, l logr.Logger, jitRequest *justintimev1.JitRequest) (ctrl.Result, error) {
	endTime := jitRequest.Spec.EndTime.Time
	if endTime.After(time.Now()) {
		delay := time.Until(endTime)
		l.Info("End time not reached, requeuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	l.Info("End time reached, deleting JitRequest")
	if err := r.deleteJitRequest(ctx, jitRequest); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Update jira with comment
func (r *JitRequestReconciler) updateJiraTicket(ctx context.Context, jiraTicket, comment string) error {
	l := log.FromContext(ctx)

	l.Info("Updating Jira ticket", "jiraTicket", jiraTicket)

	// Add a comment to the Jira issue
	payload := &models.CommentPayloadSchemeV2{
		Body: comment,
	}
	_, _, err := r.JiraClient.Issue.Comment.Add(context.Background(), jiraTicket, payload, nil)
	if err != nil {
		l.Error(err, "failed to add comment to jira ticket", "jiraTicket", jiraTicket)
		return err
	}

	return nil
}

// Complete jira with comment
func (r *JitRequestReconciler) completeJiraTicket(ctx context.Context, jitRequest *justintimev1.JitRequest, approvedTransitionID string) error {
	l := log.FromContext(ctx)

	// Add a comment to the Jira issue
	jiraTicket := jitRequest.Status.JiraTicket
	comment := fmt.Sprintf("Completed - %s", jitRequest.Status.Message)
	l.Info("Compelting Jira ticket", "jiraTicket", jiraTicket)
	if err := r.updateJiraTicket(ctx, jiraTicket, comment); err != nil {
		return err
	}

	// Complete ticket
	options := &models.IssueMoveOptionsV2{
		Fields: &models.IssueSchemeV2{
			Fields: &models.IssueFieldsSchemeV2{
				Resolution: &models.ResolutionScheme{},
			},
		},
	}
	_, err := r.JiraClient.Issue.Move(context.Background(), jiraTicket, approvedTransitionID, options)
	if err != nil {
		l.Error(err, "failed to transition jira ticket to completed")
		return err
	}

	return nil
}

// Check Jira ticket is approved
func (r *JitRequestReconciler) getJiraApproval(ctx context.Context, jitRequest *justintimev1.JitRequest, jiraWorkflowApproveStatus string) error {
	l := log.FromContext(ctx)
	l.Info("Checking Jira ticket approval", "jit request", jitRequest)

	jiraIssueKey := jitRequest.Status.JiraTicket

	// Fetch the Jira issue details
	issue, _, err := r.JiraClient.Issue.Get(ctx, jiraIssueKey, nil, nil)
	if err != nil {
		l.Error(err, "failed to fetch Jira ticket details", "jiraTicket", jiraIssueKey)
		return err
	}

	// Check if the issue status is Approved
	if issue.Fields.Status.Name == jiraWorkflowApproveStatus {
		l.Info("Jira ticket is approved", "jiraTicket", jiraIssueKey)
		return nil
	}

	return fmt.Errorf("failed on jira approval")
}

// Helper function for createJiraTicket to build custom fields in jira ticket payload
func addCustomField(ctx context.Context, customFields *models.CustomFields, fieldType, jiraCustomField, value string) {
	l := log.FromContext(ctx)

	switch fieldType {
	case "text", "date":
		if err := customFields.Text(jiraCustomField, value); err != nil {
			l.Error(err, "failed to add custom field", "field", jiraCustomField)
		}
	case "select":
		if err := customFields.Select(jiraCustomField, value); err != nil {
			l.Error(err, "failed to add custom field", "field", jiraCustomField)
		}
	case "user":
		userField := map[string]interface{}{
			"name": value,
		}
		if err := customFields.Raw(jiraCustomField, userField); err != nil {
			l.Error(err, "failed to add custom field", "field", jiraCustomField)
		}
	default:
		l.Error(fmt.Errorf("unknown custom field type"), jiraCustomField, "type", fieldType)
	}
}

// Create a jira ticket for a JitRequest
func (r *JitRequestReconciler) createJiraTicket(
	ctx context.Context,
	jitRequest *justintimev1.JitRequest,
	jiraProject,
	jiraIssueType string,
	customFieldsConfig map[string]v1.CustomFieldSettings,
	requiredFieldsConfig *v1.RequiredFieldsSpec,
) (string, error) {
	l := log.FromContext(ctx)

	l.Info("Creating Jira ticket", "jiraTicket", jitRequest)

	customFields := models.CustomFields{}

	// Add custom fields from JustInTimeConfig spec
	for fieldName, settings := range customFieldsConfig {
		value, exists := jitRequest.Spec.JiraFields[fieldName]
		if !exists {
			// missing field, reject
			errMsg := fmt.Errorf("missing custom field: %s", fieldName)
			if err := r.updateStatus(ctx, jitRequest, StatusRejected, errMsg.Error(), "Skipped", 3); err != nil {
				l.Error(err, "failed to update status to Rejected")
				return "Skipped", nil
			}
		}
		addCustomField(ctx, &customFields, settings.Type, settings.JiraCustomField, value)
	}

	// Add required fields for StartTime, EndTime, ClusterRole
	requiredFields := map[string]string{
		"StartTime":   jitRequest.Spec.StartTime.Format("2006-01-02T15:04:05.000-0700"),
		"EndTime":     jitRequest.Spec.EndTime.Format("2006-01-02T15:04:05.000-0700"),
		"ClusterRole": jitRequest.Spec.ClusterRole,
	}

	for fieldName, value := range requiredFields {
		var settings justintimev1.CustomFieldSettings
		switch fieldName {
		case "StartTime":
			settings = requiredFieldsConfig.StartTime
		case "EndTime":
			settings = requiredFieldsConfig.EndTime
		case "ClusterRole":
			settings = requiredFieldsConfig.ClusterRole
		default:
			l.Error(fmt.Errorf("unknown required field"), "field", fieldName)
			continue
		}
		addCustomField(ctx, &customFields, settings.Type, settings.JiraCustomField, value)
	}

	// payload for new jira ticket
	payload := models.IssueSchemeV2{
		Fields: &models.IssueFieldsSchemeV2{
			Summary: fmt.Sprintf("Automated JIT request for %s", jitRequest.Spec.Reporter),
			Project: &models.ProjectScheme{
				Key: jiraProject,
			},
			IssueType: &models.IssueTypeScheme{
				Name: jiraIssueType,
			},
		},
	}

	// Debug payload and customFields
	// l.Info("Jira Issue Payload", "payload", payload)
	// l.Info("Custom Fields Data", "customFields", customFields)

	createdIssue, _, err := r.JiraClient.Issue.Create(context.Background(), &payload, &customFields)
	if err != nil {
		l.Error(err, "failed to create Jira ticket")
		return "", err
	}

	l.Info("Jira ticket created successfully", "jiraTicket", createdIssue.Key)
	return createdIssue.Key, nil
}

// Reject jira with comment
func (r *JitRequestReconciler) rejectJiraTicket(ctx context.Context, jitRequest *justintimev1.JitRequest, rejectedTransitionID string) error {
	l := log.FromContext(ctx)

	// Add a comment to the Jira issue
	jiraTicket := jitRequest.Status.JiraTicket
	comment := fmt.Sprintf("Rejected - %s", jitRequest.Status.Message)
	l.Info("Rejecting Jira ticket", "jiraTicket", jiraTicket)
	if err := r.updateJiraTicket(ctx, jiraTicket, comment); err != nil {
		return err
	}

	// reject ticket
	options := &models.IssueMoveOptionsV2{
		Fields: &models.IssueSchemeV2{
			Fields: &models.IssueFieldsSchemeV2{
				Resolution: &models.ResolutionScheme{},
			},
		},
	}
	_, err := r.JiraClient.Issue.Move(context.Background(), jiraTicket, rejectedTransitionID, options)
	if err != nil {
		l.Error(err, "failed to transition jira ticket")
		return err
	}

	return nil
}

// Delete a JitRequest
func (r *JitRequestReconciler) deleteJitRequest(ctx context.Context, jitRequest *justintimev1.JitRequest) error {
	l := log.FromContext(ctx)
	if err := r.Client.Delete(ctx, jitRequest); err != nil {
		l.Error(err, "Failed to delete JitRequest")
		return err
	}
	l.Info("Successfully deleted JitRequest", "name", jitRequest.Name)
	return nil
}

// Raise event in operator namespace
func (r *JitRequestReconciler) raiseEvent(obj client.Object, eventType, reason, message string) {
	eventRef := &corev1.ObjectReference{
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Name:       obj.GetName(),
		Namespace:  OperatorNamespace,
		UID:        obj.GetUID(),
	}

	r.Recorder.Event(eventRef, eventType, reason, message)
}

// checks if a string is present in a slice.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Delete rolebinding in case of k8s GC failed to delete
func (r *JitRequestReconciler) deleteOwnedObjects(ctx context.Context, jitRequest *justintimev1.JitRequest) error {
	roleBindings := &rbacv1.RoleBindingList{}
	err := r.List(ctx, roleBindings, client.InNamespace(jitRequest.Spec.Namespace))
	if err != nil {
		return err
	}

	for _, roleBinding := range roleBindings.Items {
		for _, ownerRef := range roleBinding.OwnerReferences {
			if ownerRef.Kind == "JitRequest" && ownerRef.Name == jitRequest.Name {
				// Delete the RoleBinding if it is owned by the JitRequest
				if err := r.Delete(ctx, &roleBinding); err != nil {
					return err
				}
				break
			}
		}
	}

	return nil
}

// Check and return true if something already exists
func isAlreadyExistsError(err error) bool {
	return err != nil && apierrors.IsAlreadyExists(err)
}

// Create rolebinding for a JitRequest
func (r *JitRequestReconciler) createRbac(ctx context.Context, jitRequest *justintimev1.JitRequest) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-jit", jitRequest.Name),
			Namespace: jitRequest.Spec.Namespace,
			Annotations: map[string]string{
				"justintime.samir.io/expiry": jitRequest.Spec.EndTime.Time.Format(time.RFC3339),
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.UserKind,
				Name: jitRequest.Spec.Reporter,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     jitRequest.Spec.ClusterRole,
		},
	}

	// Set owner references
	if err := controllerutil.SetControllerReference(jitRequest, roleBinding, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference for RoleBinding: %v", err)
	}

	// Create RoleBinding
	if err := r.Client.Create(ctx, roleBinding); err != nil {
		if !isAlreadyExistsError(err) {
			return fmt.Errorf("failed to create RoleBinding: %w", err)
		}
	}

	return nil
}

// Update JitRequest status and message with retry up to maxAttempts attempts
func (r *JitRequestReconciler) updateStatus(
	ctx context.Context,
	jitRequest *justintimev1.JitRequest,
	status,
	message,
	jiraTicket string,
	maxAttempts int,
) error {
	attempts := 0
	for {
		attempts++
		jitRequest.Status.State = status
		jitRequest.Status.Message = message
		jitRequest.Status.JiraTicket = jiraTicket
		err := r.Status().Update(ctx, jitRequest)
		if err == nil {
			return nil // Update successful
		}
		if apierrors.IsConflict(err) {
			// Conflict error, retry the update
			if attempts >= maxAttempts {
				return fmt.Errorf("maximum retry attempts reached, failed to update JitRequest status")
			}
			// Incremental sleep between attempts
			time.Sleep(time.Duration(attempts*2) * time.Second)
			continue
		}
		// Other error, return with the error
		return fmt.Errorf("failed to update JitRequest status: %v", err)
	}
}

/*
Predicate function to filter events for JitRequest objects
Ignore StatusRejected update event for JitRequest if the same
*/
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

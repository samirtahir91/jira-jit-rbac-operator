package controller

import (
	"context"
	"fmt"
	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/pkg/utils"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// handleRejected rejects Jira ticket and deletes a JitRequest
func (r *JitRequestReconciler) handleRejected(ctx context.Context, l logr.Logger, jitRequest *justintimev1.JitRequest, rejectedTransitionID string) (ctrl.Result, error) {
	// Reject jira ticket
	if jitRequest.Status.JiraTicket != Skipped {
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

// handleNewRequest creates a new Jira ticket for new JitRequests and validates config
func (r *JitRequestReconciler) handleNewRequest(ctx context.Context, l logr.Logger, jitRequest *justintimev1.JitRequest, allowedClusterRoles []string, jiraProject, jiraIssueType string, customFieldsConfig map[string]justintimev1.CustomFieldSettings, requiredFieldsConfig *justintimev1.RequiredFieldsSpec, ticketLabels []string, targetEnvironment *justintimev1.EnvironmentSpec, additionalComments string) (ctrl.Result, error) {
	jiraIssueKey, err := r.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
	if err != nil {
		l.Error(err, "failed to createJiraTicket")
		return ctrl.Result{}, err
	}

	// Return if missing jira field status failed
	if jiraIssueKey == Skipped {
		return ctrl.Result{}, nil
	}

	// check cluster role is allowed
	if !utils.Contains(allowedClusterRoles, jitRequest.Spec.ClusterRole) {
		return r.rejectInvalidRole(ctx, l, jitRequest, jiraIssueKey)
	}

	// check namespace labels match namespace(s)
	if os.Getenv("ENABLE_WEBHOOKS") != "true" { // ignore if handled by webnhook
		ns, err := utils.ValidateNamespaceLabels(ctx, jitRequest, r.Client)
		if err != nil {
			return r.rejectInvalidNamespace(ctx, l, jitRequest, jiraIssueKey, strings.Join(ns, ", "), err.Error())
		}
	}

	// check namespaces match regex defined in config
	nsRegex, err := utils.ValidateNamespaceRegex(jitRequest.Spec.Namespaces)
	if err != nil {
		return r.rejectInvalidNamespace(ctx, l, jitRequest, jiraIssueKey, nsRegex, err.Error())
	}

	return r.preApproveRequest(ctx, l, jitRequest, jiraIssueKey, additionalComments)
}

// handlePreApproved creates the role binding for approved JitRequests if the Jira ticket is approved
func (r *JitRequestReconciler) handlePreApproved(ctx context.Context, l logr.Logger, jitRequest *justintimev1.JitRequest, completedTransitionID, jiraWorkflowApproveStatus string) (ctrl.Result, error) {
	// check if it needs to be re-queued
	startTime := jitRequest.Status.StartTime.Time
	if startTime.After(time.Now()) {
		// requeue for start time
		delay := time.Until(startTime)
		l.Info("Start time not reached, requeuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	jiraTicket := jitRequest.Status.JiraTicket
	if err := r.getJiraApproval(ctx, jitRequest, jiraWorkflowApproveStatus); err != nil {
		l.Error(err, StatusRejected, "jira ticket", jiraTicket)
		r.raiseEvent(jitRequest, "Warning", "JiraNotApproved", fmt.Sprintf("Error: %s", err))
		if err := r.updateStatus(ctx, jitRequest, StatusRejected, "Jira ticket has not been approved", jiraTicket); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	l.Info("Creating role binding")
	if err := r.createRoleBinding(ctx, jitRequest); err != nil {
		l.Error(err, "failed to create rbac for JIT request")
		r.raiseEvent(jitRequest, "Warning", "FailedRBAC", fmt.Sprintf("Error: %s", err))
		return ctrl.Result{}, err
	}

	if err := r.completeJiraTicket(ctx, jitRequest, completedTransitionID); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, jitRequest, StatusSucceeded, "Access granted until end time", jiraTicket); err != nil {
		return ctrl.Result{}, err
	}

	// Queue for deletion at end time
	return r.handleCleanup(ctx, l, jitRequest)
}

// handleCleanup cleans up and re-queue succeeded and unknown JitRequests for deletion
func (r *JitRequestReconciler) handleCleanup(ctx context.Context, l logr.Logger, jitRequest *justintimev1.JitRequest) (ctrl.Result, error) {
	endTime := jitRequest.Status.EndTime.Time
	if endTime.After(time.Now()) {
		delay := time.Until(endTime)
		l.Info("End time not reached, re-queuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	l.Info("End time reached, deleting JitRequest")
	if err := r.deleteJitRequest(ctx, jitRequest); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// handleFetchError cleans-up owned objects (role bindings) on deleted JitRequests
func (r *JitRequestReconciler) handleFetchError(ctx context.Context, l logr.Logger, err error, jitRequest *justintimev1.JitRequest) (ctrl.Result, error) {
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

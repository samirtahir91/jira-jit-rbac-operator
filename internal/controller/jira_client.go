package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	justintimev1 "jira-jit-rbac-operator/api/v1"

	"github.com/ctreminiom/go-atlassian/v2/pkg/infra/models"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// updateJiraTicket updates a jira ticket with a comment
func (r *JitRequestReconciler) updateJiraTicket(ctx context.Context, jiraTicket, comment string) error {
	l := log.FromContext(ctx)

	l.Info("Updating Jira ticket", "jiraTicket", jiraTicket)

	// Add a comment to the Jira issue
	payload := &models.CommentPayloadSchemeV2{
		Body: comment,
	}
	_, response, err := r.JiraClient.Issue.Comment.Add(context.Background(), jiraTicket, payload, nil)
	if err != nil {
		if response != nil {
			body := response.Bytes.String()
			l.Error(err, "failed to add comment to jira ticket", "jiraTicket", jiraTicket, "response", body)
		} else {
			l.Error(err, "failed to add comment to jira ticket", "jiraTicket", jiraTicket, "response", "nil response")
		}
		return err
	}

	return nil
}

// completeJiraTicket completes a jira ticket with a comment
func (r *JitRequestReconciler) completeJiraTicket(ctx context.Context, jitRequest *justintimev1.JitRequest, completedTransitionID string) error {
	l := log.FromContext(ctx)

	// Add a comment to the Jira issue
	jiraTicket := jitRequest.Status.JiraTicket
	comment := "{color:#00875a}*Completed - Access granted until end time*{color}"
	l.Info("Completing Jira ticket", "jiraTicket", jiraTicket)
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
	response, err := r.JiraClient.Issue.Move(context.Background(), jiraTicket, completedTransitionID, options)
	if err != nil {
		if response != nil {
			body := response.Bytes.String()
			l.Error(err, "failed to transition jira ticket to completed", "response", body)
		} else {
			l.Error(err, "failed to transition jira ticket to completed", "jiraTicket", jiraTicket, "response", "nil response")
		}
		return err
	}

	return nil
}

// getJiraApproval checks a Jira ticket is approved
func (r *JitRequestReconciler) getJiraApproval(ctx context.Context, jitRequest *justintimev1.JitRequest, jiraWorkflowApproveStatus string) error {
	l := log.FromContext(ctx)
	l.Info("Checking Jira ticket approval", "jit request", jitRequest)

	jiraIssueKey := jitRequest.Status.JiraTicket

	// Fetch the Jira issue details
	issue, response, err := r.JiraClient.Issue.Get(ctx, jiraIssueKey, nil, nil)
	if err != nil {
		if response != nil {
			body := response.Bytes.String()
			l.Error(err, "failed to fetch Jira ticket details", "jiraTicket", jiraIssueKey, "response", body)
		} else {
			l.Error(err, "failed to fetch Jira ticket details", "jiraTicket", jiraIssueKey, "response", "nil response")
		}
		return err
	}

	// Check if the issue status is Approved
	if issue.Fields.Status.Name == jiraWorkflowApproveStatus {
		l.Info("Jira ticket is approved", "jiraTicket", jiraIssueKey)
		return nil
	}

	return fmt.Errorf("failed on jira approval")
}

// getNameByEmail gets and returns an account ID for a Jira user by email - assumes single email per user and gets the 1st result
func (r *JitRequestReconciler) getNameByEmail(email string) (string, error) {

	type User struct {
		Name string `json:"name"`
	}

	// RAW endpoint
	apiEndpoint := fmt.Sprintf("rest/api/2/user/search?username=%s", email)
	request, err := r.JiraClient.NewRequest(context.Background(), http.MethodGet, apiEndpoint, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to find account name for reporter email: %w", err)
	}

	var users []User
	response, err := r.JiraClient.Call(request, &users)
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

// addCustomField is a helper function for createJiraTicket to build custom fields in jira ticket payload
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

// createJiraTicket creates a jira ticket for a JitRequest
func (r *JitRequestReconciler) createJiraTicket(ctx context.Context, jitRequest *justintimev1.JitRequest, jiraProject, jiraIssueType string, customFieldsConfig map[string]justintimev1.CustomFieldSettings, requiredFieldsConfig *justintimev1.RequiredFieldsSpec, ticketLabels []string, targetEnvironment *justintimev1.EnvironmentSpec) (string, error) {
	l := log.FromContext(ctx)

	l.Info("Creating Jira ticket", "jiraTicket", jitRequest)

	customFields := models.CustomFields{}

	// Add custom fields from JustInTimeConfig spec
	for fieldName, settings := range customFieldsConfig {
		value, exists := jitRequest.Spec.JiraFields[fieldName]
		if !exists {
			// missing field, reject
			errMsg := fmt.Errorf("missing custom field: %s", fieldName)
			if err := r.updateStatus(ctx, jitRequest, StatusRejected, errMsg.Error(), Skipped); err != nil {
				l.Error(err, "failed to update status to Rejected")
				return Skipped, nil
			}
			return Skipped, nil
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

	// Get Jira account ID from reporter email
	reporterAccountName, err := r.getNameByEmail(jitRequest.Spec.Reporter)
	if err != nil {
		l.Error(err, "failed to create Jira ticket")
		return "", err
	}

	targetCluster := targetEnvironment.Cluster
	targetEnv := targetEnvironment.Environment
	combinedLabels := append(
		ticketLabels,
		"jira-jit-rbac-operator",
		"automated_jit_request",
		targetCluster,
		targetEnv,
	)

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
			// Set reporter as per userID
			Reporter: &models.UserScheme{
				Name: reporterAccountName,
			},
			Labels: combinedLabels,
		},
	}

	// Debug payload and customFields
	// l.Info("Jira Issue Payload", "payload", payload)
	// l.Info("Custom Fields Data", "customFields", customFields)

	createdIssue, response, err := r.JiraClient.Issue.Create(context.Background(), &payload, &customFields)
	if err != nil {
		if response != nil {
			body := response.Bytes.String()
			l.Error(err, "failed to create Jira ticket", "response", body, "payload", payload, "customFields", customFields)
		} else {
			l.Error(err, "failed to create Jira ticket", "response", "nil response", "payload", payload, "customFields", customFields)
		}
		return "", err
	}

	l.Info("Jira ticket created successfully", "jiraTicket", createdIssue.Key)
	return createdIssue.Key, nil
}

// rejectJiraTicket rejects a jira ticket with comment
func (r *JitRequestReconciler) rejectJiraTicket(ctx context.Context, jitRequest *justintimev1.JitRequest, rejectedTransitionID string) error {
	l := log.FromContext(ctx)

	// Add a comment to the Jira issue
	jiraTicket := jitRequest.Status.JiraTicket
	comment := fmt.Sprintf("{color:#de350b}*Rejected - %s*{color}", jitRequest.Status.Message)
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
	response, err := r.JiraClient.Issue.Move(context.Background(), jiraTicket, rejectedTransitionID, options)
	if err != nil {
		if response != nil {
			body := response.Bytes.String()
			l.Error(err, "failed to transition jira ticket", "response", body)
		} else {
			l.Error(err, "failed to transition jira ticket", "response", "nil response")
		}
		return err
	}

	return nil
}

// preApproveRequest pre-approves a JitRequest, updates the Jira ticket and re-queues for start time
func (r *JitRequestReconciler) preApproveRequest(ctx context.Context, l logr.Logger, jitRequest *justintimev1.JitRequest, jiraIssueKey, additionalComments string) (ctrl.Result, error) {
	startTime := jitRequest.Spec.StartTime.Time

	if startTime.After(time.Now()) {

		// record event
		r.raiseEvent(jitRequest, "Normal", StatusPreApproved, fmt.Sprintf("ClusterRole '%s' is allowed\nJira: %s", jitRequest.Spec.ClusterRole, jiraIssueKey))

		// msg for status and comment
		jitRequestStatusMsg := "Pre-approval - Access will be granted at start time pending human approval(s)"

		// build comment
		jiraMessage := fmt.Sprintf("{color:#00875a}*%s*{color}", jitRequestStatusMsg)
		namespaces := strings.Join(jitRequest.Spec.Namespaces, "\n")
		comment := jiraMessage + "\n|*Namespace(s)*|" + namespaces + "|\n|*User*|" + jitRequest.Spec.Reporter + "|"

		// check if additionalUsers defined and add to comment
		additionalUsers := jitRequest.Spec.AdditionUserEmails
		if len(additionalUsers) > 0 {
			additionalUsersStr := strings.Join(additionalUsers, "\n")
			comment += "\n|*Additional Users*|" + additionalUsersStr + "|"
		}

		// add additional comments if exists
		if additionalComments != "" {
			comment += "\n\n*Additional Info:*\n" + additionalComments
		}

		// add comment
		if err := r.updateJiraTicket(ctx, jiraIssueKey, comment); err != nil {
			return ctrl.Result{}, err
		}

		// update jitRequest status
		if err := r.updateStatus(ctx, jitRequest, StatusPreApproved, jitRequestStatusMsg, jiraIssueKey); err != nil {
			l.Error(err, "failed to update status to Pre-Approved")
			return ctrl.Result{}, err
		}

		// requeue for start time
		delay := time.Until(startTime)
		l.Info("Start time not reached, requeuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// invalid start time, reject
	errMsg := fmt.Errorf("start time %s must be after current time", jitRequest.Spec.StartTime.Time)
	l.Error(errMsg, "start time validation failed")

	// record event
	r.raiseEvent(jitRequest, "Warning", EventValidationFailed, errMsg.Error())

	// update jitRequest status
	if err := r.updateStatus(ctx, jitRequest, StatusRejected, errMsg.Error(), jiraIssueKey); err != nil {
		l.Error(err, "failed to update status to Rejected")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

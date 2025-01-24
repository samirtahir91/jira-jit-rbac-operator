package controller

import (
	"context"
	"fmt"
	justintimev1 "jira-jit-rbac-operator/api/v1"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Fetch a JitRequest
func (r *JitRequestReconciler) fetchJitRequest(ctx context.Context, namespacedName types.NamespacedName) (*justintimev1.JitRequest, error) {
	jitRequest := &justintimev1.JitRequest{}
	err := r.Get(ctx, namespacedName, jitRequest)
	return jitRequest, err
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
	jitRequest.Status.State = status
	jitRequest.Status.Message = message
	jitRequest.Status.JiraTicket = jiraTicket
	jitRequest.Status.StartTime = jitRequest.Spec.StartTime
	jitRequest.Status.EndTime = jitRequest.Spec.EndTime
	for {
		attempts++
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

// Reject an invalid namespace
func (r *JitRequestReconciler) rejectInvalidNamespace(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	jiraIssueKey, namespace, err string,
) (ctrl.Result, error) {
	errorMsg := fmt.Sprintf("Namespace(s) %s not validated | Error: %s", namespace, err)
	r.raiseEvent(jitRequest, "Warning", EventValidationFailed, errorMsg)
	if err := r.updateStatus(ctx, jitRequest, StatusRejected, errorMsg, jiraIssueKey, 3); err != nil {
		l.Error(err, "failed to update status to Rejected")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Reject an invalid cluster role
func (r *JitRequestReconciler) rejectInvalidRole(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	jiraIssueKey string,
) (ctrl.Result, error) {
	errorMsg := fmt.Sprintf("ClusterRole '%s' is not allowed", jitRequest.Spec.ClusterRole)
	r.raiseEvent(jitRequest, "Warning", EventValidationFailed, errorMsg)
	if err := r.updateStatus(ctx, jitRequest, StatusRejected, errorMsg, jiraIssueKey, 3); err != nil {
		l.Error(err, "failed to update status to Rejected")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Delete rolebinding in case of k8s GC failed to delete
func (r *JitRequestReconciler) deleteOwnedObjects(ctx context.Context, jitRequest *justintimev1.JitRequest) error {
	for _, namespace := range jitRequest.Spec.Namespaces {
		roleBindings := &rbacv1.RoleBindingList{}

		err := r.List(ctx, roleBindings, client.InNamespace(namespace))
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
	}

	return nil
}

// Check and return true if something already exists
func isAlreadyExistsError(err error) bool {
	return err != nil && apierrors.IsAlreadyExists(err)
}

// Create rolebinding(s) for a JitRequest's namespaces
func (r *JitRequestReconciler) createRoleBinding(ctx context.Context, jitRequest *justintimev1.JitRequest) error {
	// Add reporter to subject
	subjects := []rbacv1.Subject{
		{
			Kind: rbacv1.UserKind,
			Name: jitRequest.Spec.Reporter,
		},
	}

	// Add additional user emails as subjects if defined
	for _, email := range jitRequest.Spec.AdditionUserEmails {
		subjects = append(subjects, rbacv1.Subject{
			Kind: rbacv1.UserKind,
			Name: email,
		})
	}

	// Loop through namespaces in JitRequest and create role binding
	for _, namespace := range jitRequest.Spec.Namespaces {
		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-jit", jitRequest.Name),
				Namespace: namespace,
				Annotations: map[string]string{
					"justintime.samir.io/expiry": jitRequest.Spec.EndTime.Time.Format(time.RFC3339),
				},
			},
			Subjects: subjects,
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
	}

	return nil
}

// Pre-approve the JitRequest, update the Jira ticke and queue for start time
func (r *JitRequestReconciler) preApproveRequest(
	ctx context.Context,
	l logr.Logger,
	jitRequest *justintimev1.JitRequest,
	jiraIssueKey, additionalComments string,
) (ctrl.Result, error) {
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
		if err := r.updateStatus(ctx, jitRequest, StatusPreApproved, jitRequestStatusMsg, jiraIssueKey, 5); err != nil {
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
	if err := r.updateStatus(ctx, jitRequest, StatusRejected, errMsg.Error(), jiraIssueKey, 3); err != nil {
		l.Error(err, "failed to update status to Rejected")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

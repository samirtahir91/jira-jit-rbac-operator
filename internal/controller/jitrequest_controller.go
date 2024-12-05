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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	justintimev1 "jit-rbac-operator/api/v1"
	"jit-rbac-operator/internal/config"
)

var OperatorNamespace = os.Getenv("OPERATOR_NAMESPACE")

// JitRequestReconciler reconciles a JitRequest object
type JitRequestReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	ConfigurationName string
}

// Reconcile loop
func (r *JitRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the JitRequest instance
	jitRequest := &justintimev1.JitRequest{}
	err := r.Get(ctx, req.NamespacedName, jitRequest)
	if err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("JitRequest resource not found. Deleting managed objects.")
			// Delete owned rbac objects
			if err := r.deleteOwnedObjects(ctx, jitRequest); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		l.Error(err, "failed to get JitRequest")
		return ctrl.Result{}, err
	}

	l.Info(
		"Got JitRequest",
		"Requestor", jitRequest.Spec.User,
		"Role", jitRequest.Spec.ClusterRole,
		"Namespace", jitRequest.Spec.Namespace,
	)

	// Get state of JitRequest, process if not Succeeded
	jitStatus := jitRequest.Status.State
	if jitStatus != "Succeeded" {
		// Check cluster role is allowed from local config file
		allowedClusterRoles, err := ReadAllowedClusterRoles(config.ConfigCacheFilePath, config.ConfigFile)
		if err != nil {
			l.Error(err, "failed to read allowed cluster roles from configuration file")
			return ctrl.Result{}, err
		}

		// Check if the cluster role is allowed
		if !contains(allowedClusterRoles, jitRequest.Spec.ClusterRole) {
			l.Error(fmt.Errorf("invalid cluster role"), "ClusterRole not allowed", "role", jitRequest.Spec.ClusterRole)
			// Update the status to Rejected
			errorMsg := fmt.Sprintf("ClusterRole '%s' is not allowed", jitRequest.Spec.ClusterRole)
			if err := r.updateStatus(ctx, jitRequest, errorMsg, 3); err != nil {
				return ctrl.Result{}, err
			}
			// record event
			r.raiseEvent(jitRequest, "Warning", "ValidationFailed", errorMsg)
			return ctrl.Result{}, nil
		}
		l.Info("JitRequest is valid", "ClusterRole", jitRequest.Spec.ClusterRole)

		// Check start time, requeue if needed
		startTime := jitRequest.Spec.StartTime.Time
		if startTime.After(time.Now()) {
			delay := time.Until(startTime)
			l.Info("Start time not reached, requeuing", "requeueAfter", delay)
			return ctrl.Result{RequeueAfter: delay}, nil
		}

		// Create rbac for JIT request
		l.Info("Creating role binding")
		if err := r.createRbac(ctx, jitRequest); err != nil {
			l.Error(err, "failed to create rbac for JIT request")
			// Raise event
			r.raiseEvent(
				jitRequest,
				"Warning",
				"FailedRBAC",
				fmt.Sprintf("Error: %s", err),
			)
			return ctrl.Result{}, err
		}

		// Update the status to Succeeded
		if err := r.updateStatus(ctx, jitRequest, "Succeeded", 3); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Requeue for deletion
	endTime := jitRequest.Spec.EndTime.Time
	if endTime.After(time.Now()) {
		delay := time.Until(endTime)
		l.Info("End time not reached, requeuing", "requeueAfter", delay)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// It's expired, delete the JitRequest
	l.Info("End time reached, deleting JitRequest")
	if err := r.Client.Delete(ctx, jitRequest); err != nil {
		l.Error(err, "Failed to delete JitRequest")
		return ctrl.Result{}, err
	}

	l.Info("Successfully deleted JitRequest", "name", jitRequest.Name)

	return ctrl.Result{}, nil
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

	// // debugging
	// log.Log.V(1).Info("Raising event with details",
	// 	"OperatorNamespace", OperatorNamespace,
	// 	"Kind", obj.GetObjectKind().GroupVersionKind().Kind,
	// 	"APIVersion", obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
	// 	"Name", obj.GetName(),
	// 	"UID", obj.GetUID(),
	// 	"EventType", eventType,
	// 	"Reason", reason,
	// 	"Message", message,
	// )

	r.Recorder.Event(eventRef, eventType, reason, message)
}

// ReadAllowedClusterRoles reads the allowed cluster roles from a configuration file.
func ReadAllowedClusterRoles(filePath string, fileName string) ([]string, error) {
	// Read the file content
	data, err := os.ReadFile(fmt.Sprintf("%s/%s", filePath, fileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var config struct {
		AllowedClusterRoles []string `json:"allowedClusterRoles"`
	}

	// Parse JSON content into the struct
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	return config.AllowedClusterRoles, nil
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
				Name: jitRequest.Spec.User,
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

// Update JitRequest status with retry up to maxAttempts attempts
func (r *JitRequestReconciler) updateStatus(ctx context.Context, jitRequest *justintimev1.JitRequest, status string, maxAttempts int) error {
	attempts := 0
	for {
		attempts++
		jitRequest.Status.State = status
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

// SetupWithManager sets up the controller with the Manager.
func (r *JitRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&justintimev1.JitRequest{}).
		Named("jitrequest").
		Complete(r)
}

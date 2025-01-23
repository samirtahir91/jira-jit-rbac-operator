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

package v1

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/pkg/utils"
)

// nolint:unused
// log is for logging in this package.
var jitrequestlog = logf.Log.WithName("jitrequest-resource")
var globalClient client.Client

// SetupJitRequestWebhookWithManager registers the webhook for JitRequest in the manager.
func SetupJitRequestWebhookWithManager(mgr ctrl.Manager) error {
	globalClient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).For(&justintimev1.JitRequest{}).
		WithValidator(&JitRequestCustomValidator{}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-justintime-samir-io-v1-jitrequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=justintime.samir.io,resources=jitrequests,verbs=create;update,versions=v1,name=vjitrequest-v1.kb.io,admissionReviewVersions=v1

// JitRequestCustomValidator struct is responsible for validating the JitRequest resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type JitRequestCustomValidator struct {
}

var _ webhook.CustomValidator = &JitRequestCustomValidator{}

// validateJitRequestSpec validates customFields from the applied JustInTimeConfig are defined in a JitRequest.JiraFields
func validateJitRequestSpec(ctx context.Context, jitRequest *justintimev1.JitRequest) (*field.Error, error) {

	// Fetch operator config
	operatorConfig, err := utils.ReadConfigFromFile()
	if err != nil {
		return nil, err
	}

	// check cluster role is allowed
	allowedClusterRoles := operatorConfig.AllowedClusterRoles
	allowedClusterRolesString := strings.Join(allowedClusterRoles, ", ")
	msg := fmt.Sprintf("clusterRole must be one of '%s'", allowedClusterRolesString)
	if !utils.Contains(allowedClusterRoles, jitRequest.Spec.ClusterRole) {
		return field.Invalid(field.NewPath("spec").Child("clusterRole"), jitRequest.Spec.ClusterRole, msg), nil
	}

	// check startTime is after current time
	startTime := jitRequest.Spec.StartTime.Time
	msg = "start time must be after current time"
	if !startTime.After(time.Now()) {
		return field.Invalid(field.NewPath("spec").Child("startTime"), jitRequest.Spec.StartTime, msg), nil
	}

	// check endTime is after startTime
	endTime := jitRequest.Spec.EndTime.Time
	msg = fmt.Sprintf("end time must be after startTime '%s'", startTime)
	if !endTime.After(startTime) {
		return field.Invalid(field.NewPath("spec").Child("endTime"), jitRequest.Spec.EndTime, msg), nil
	}

	// check namespaces match regex defined in config
	_, err = utils.ValidateNamespaceRegex(jitRequest.Spec.Namespaces)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("namespaces"), jitRequest.Spec.Namespaces, err.Error()), nil
	}

	// check namespace labels match namespace(s)
	_, err = utils.ValidateNamespaceLabels(ctx, jitRequest, globalClient)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("namespaces"), jitRequest.Spec.Namespaces, err.Error()), nil
	}

	// check customFields from config match jiraFields in JitRequest
	customFieldsConfig := operatorConfig.CustomFields
	for fieldName := range customFieldsConfig {
		_, exists := jitRequest.Spec.JiraFields[fieldName]
		if !exists {
			// Missing field, reject
			errMsg := fmt.Sprintf("missing custom field: %s", fieldName)
			return field.Invalid(field.NewPath("spec").Child("jiraFields"), jitRequest.Spec.JiraFields, errMsg), nil
		}
	}

	return nil, nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type JitRequest.
func (v *JitRequestCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	jitrequest, ok := obj.(*justintimev1.JitRequest)
	if !ok {
		return nil, fmt.Errorf("expected a JitRequest object but got %T", obj)
	}
	jitrequestlog.Info("Validation for JitRequest upon creation", "name", jitrequest.GetName())

	fieldErr, err := validateJitRequestSpec(ctx, jitrequest)
	if err != nil {
		return nil, err
	}
	if fieldErr != nil {
		return nil, fieldErr
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type JitRequest.
func (v *JitRequestCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	jitrequest, ok := newObj.(*justintimev1.JitRequest)
	if !ok {
		return nil, fmt.Errorf("expected a JitRequest object for the newObj but got %T", newObj)
	}
	jitrequestlog.Info("Validation for JitRequest upon update", "name", jitrequest.GetName())
	fieldErr, err := validateJitRequestSpec(ctx, jitrequest)
	if err != nil {
		return nil, err
	}
	if fieldErr != nil {
		return nil, fieldErr
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type JitRequest.
func (v *JitRequestCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	jitrequest, ok := obj.(*justintimev1.JitRequest)
	if !ok {
		return nil, fmt.Errorf("expected a JitRequest object but got %T", obj)
	}
	jitrequestlog.Info("Validation for JitRequest upon deletion", "name", jitrequest.GetName())

	return nil, nil
}

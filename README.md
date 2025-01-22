[![Lint](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/lint.yml)
[![Integration tests](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/integration-test.yaml/badge.svg)](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/integration-test.yaml)
[![Webhook Integration tests](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/webhook-integration-tests.yaml/badge.svg)](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/webhook-integration-tests.yaml)
[![Build and push](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/build-and-push.yml/badge.svg)](https://github.com/samirtahir91/jira-jit-rbac-operator/actions/workflows/build-and-push.yml)
[![Coverage Status](https://coveralls.io/repos/github/samirtahir91/jira-jit-rbac-operator/badge.svg?branch=main)](https://coveralls.io/github/samirtahir91/jira-jit-rbac-operator?branch=main)

# jira-jit-rbac-operator 

The `jira-jit-rbac-operator` is a Kubernetes operator that creates short-lived rolebindings for users based on a JitRequest custom resource. It integrates with a configurable Jira Workflow, the operator submitts a Jira ticket in a Jira Project for approval by a Human before granting the role-binding for the requested time period. It empowers self-service of Just-In-Time privileged access using Kubernetes RBAC.

## ToDo

## Description

### Key Features
- Uses a custom cluster scoped resource `JitRequest`, where a user creates a JitReuest with:
  - reporter
  - clusterRole
  - additionalEmails (optional users to also add to role binding)
  - namespaces
  - namespaceLabels (optional)
  - justification
  - startTime
  - endTime
  - JiraFields (custom fields defined by JustInTimeConfig's `customFields`)
- The operator checks if the JitRequest's cluster role is allowed, from the `allowedClusterRoles` list defined in a `JustInTimeConfig` custom resource (set by admins/operators) and then pre-approves the request.
- Submits the request as a Jira Ticket to a configured Jira Project with the details as per the `JitRequest` spec.
- Requeues the `JitRequest` object for the defined `startTime` and checks the Jira Ticket for approval status
- Creates the RoleBinding as requested if Jira Ticket is approved, rejects and cleans-up `JitRequest` if the Jira Ticket is not approved.
- Deletes expired `JitRequests` and child objects (RoleBindings) at scheduled `endTime`.

### Configuration for Jira

#### Create Jira API Token Secret

- Create a Jira account for the operator and generate a PAT (personal access token).
- Grant the account permission to modify reporters on the target Jira project.
- Grant the account permission to create/update issues in the target Jira project.
- Create a secret for the PAT

```sh
kubectl -n jira-jit-rbac-operator-system create secret generic \
  jira-credentials \
  --from-literal=api-token=<PERSONAL ACCESS TOKEN>
```

#### Project and Workflow configuration

The operator is configurable for a Jira project and Workflow using the `JustInTimeConfig` custom resource [sample](samples/jit-cfg.yaml)

You will need to create the required custom fields in Jira to be used by the workflow and map them to the `JustInTimeConfig`, i.e.:

| Custom Field  | Type           |
|---------------|----------------|
| Cluster Role  | Single select  |
| Start Time    | Date and time  |
| End Time      | Date and time  |

The sample workflow used is [here](samples/workflow.xml), you need to [import](https://confluence.atlassian.com/display/ADMINJIRASERVER088/Using+XML+to+create+a+workflow)/create an identical Workflow in your Jira Project (the IDs of fields etc are configurable as below).

You must define these with the values according to your Jira Project and Workflow (to map the fields from your workflow to the opertor's config):

| **Field**                | **Description**                                                                 |
|--------------------------|---------------------------------------------------------------------------------|
| `workflowApprovedStatus` | The status indicating that the workflow has been approved in the Jira workflow. |
| `rejectedTransitionID`   | The ID of the transition used when a workflow is rejected.                      |
| `jiraProject`            | The Jira project associated with the request.                                   |
| `jiraIssueType`          | The type of Jira issue to be created.                                           |
| `completedTransitionID`  | The ID of the transition used when a workflow is completed.                     |
| `requiredFields`         | The type and id of the required fields in Jira.                                 |
| `customFields`           | The type and id of the required fields in Jira for custom fields that need to   |
|                          | be validated against the JiraFields in the request.                             |

The `customFields` are completely configurable to what fields you want a user to define a value for in a `JitRequest`\
Each custom field is sent in the payload to Jira on creation of a new issue.\
This allows you to use whatever fields as per your workflow.

Detail:
- Each customField requires a `type` and `jiraCustomField`
- Each custom field is required in `JitRequest.Spec.JiraFields`
- I.e. If I add `ProductOwner` as a custom field, then all users will need to define `JiraFields.ProductOwner` in my `JitRequest`
- Example custom fields and data types:
  | Custom Field  | Type           |
  |---------------|----------------|
  | Reporter      | Text           |
  | ProductOwner  | User Select    |
  | Justification | Text multiline |

### Logging and Debugging
- By default, logs are JSON formatted, and log level is set to info and error.
- Set `DEBUG_LOG` to `true` in the manager deployment environment variable for debug level logs.

### Additional Information
- The CRD includes extra data printed with `kubectl get jitreq`:
  - User
  - Cluster Role
  - Namespace
  - Start Time
  - End Time
- Events are recorded for:
  - Rejected `JitRequests`
  - Failure to create a RoleBinding for a `JitRequest`
  - Validation on allowed cluster roles

## Example `JitRequest` Resource

Here is an example of how to define the `JitRequest` resource:

```yaml
apiVersion: justintime.samir.io/v1
kind: JitRequest
metadata:
  name: jitrequest-sample
spec:
  userEmail: dev@dev.com
  additionalEmails:
    - "dev2@dev.com"
    - "dev3@dev.com"
  namespaces: 
    - foo
    - bar
  namespaceLabels:
    foo: bar
  startTime: 2025-01-18T11:48:10Z
  endTime: 2025-01-18T11:51:10Z
  clusterRole: edit
  jiraFields:
    Approver: admin
    ProductOwner: admin
    Justification: "need a jit now pls"
```

Above the jiraFields are mapped to the customFields in the `JustInTimeConfig`:
```yaml
apiVersion: justintime.samir.io/v1
kind: JustInTimeConfig
metadata:
  name: jira-jit-rbac-operator-default
spec:
  allowedClusterRoles:
    - admin
    - edit
  labels:
    - minikube-test
  namespaceAllowedRegex: ".*"
  environment:
    environment: local
    cluster: minikube
  additionalCommentText: "cluster: minikube"
  workflowApprovedStatus: "Approved"
  rejectedTransitionID: "21"
  jiraProject: IAM
  jiraIssueType: Access Request
  completedTransitionID: "41"
  requiredFields:
    ClusterRole:
      type: "select"
      jiraCustomField: "customfield_10115"
    StartTime:
      type: "date"
      jiraCustomField: "customfield_10200"
    EndTime:
      type: "date"
      jiraCustomField: "customfield_10201"
  customFields:
    Approver:
      type: "user"
      jiraCustomField: "customfield_10112"
    ProductOwner:
      type: "user"
      jiraCustomField: "customfield_10113"
    Justification:
      type: "text"
      jiraCustomField: "customfield_10114"
```

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.22.0+.
- Access to a Kubernetes v1.22.0+ cluster.

### To deploy with Helm using public Docker image
A helm chart is generated using `make helm`.
- Edit the `values.yaml` as required.
```sh
cd charts/jira-jit-rbac-operator
helm upgrade --install -n jira-jit-rbac-operator-system <release_name> . --create-namespace

# To install without webhooks use these flags
  --set webhook.enabled=false \
  --set controllerManager.manager.env.enableWebhooks="false"
```
- You can use the latest public image on DockerHub - `samirtahir91076/jira-jit-rbac-operator:latest`
  - See [tags](https://hub.docker.com/r/samirtahir91076/jira-jit-rbac-operator/tags) 
- Deploy the chart with Helm.

### To Deploy on the cluster (from source and with Kustomize)
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/jira-jit-rbac-operator:tag
```

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/jira-jit-rbac-operator:tag
```

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

### Integration Testing
- Tests should be run against a real cluster, i.e. Kind or Minikube
```sh
export OPERATOR_NAMESPACE=jira-jit-int-test
USE_EXISTING_CLUSTER=true make test

USE_EXISTING_CLUSTER=false make test-webhooks
```

**Run the controller in the foreground for testing:**
```sh
export JIT_RBAC_OPERATOR_CONFIG_PATH=/tmp/jit-test/
export OPERATOR_NAMESPACE=default
export JIRA_BASE_URL=http://127.0.0.1 # your jira url
export JIRA_API_TOKEN=<PERSONAL ACESS TOKEN>
# run
make run
```

**Generate coverage html report:**
```sh
go tool cover -html=cover.out -o coverage.html
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

# jira-jit-rbac-operator 

The `jira-jit-rbac-operator` is a Kubernetes operator that creates short-lived rolebindings for users based on a JitRequest custom resource. It integrates with a configurable Jira Workflow, the operator submitts a Jira ticket in a Jira Project for approval by a Human before granting the role-binding for the requested time period. It empowers self-service of Just-In-Time privileged access using Kubernetes RBAC.

## ToDo
- Update readme on latest spec
- Optional OPA policy or Validating Webhook that compares with JustInTimeConfig customFields.

## Description

### Key Features
- Uses a custom cluster scoped resource `JitRequest`.
- Reads `reporter`, `clusterRole`, `approver`, `productOwner`, `justification`, `namespace` `startTime` and `endTime` from a `JitRequest`.
- Checks if the JitRequest's cluster role is allowed, from the `allowedClusterRoles` list defined in a `JustInTimeConfig` custom resource (set by admins/operators) and then pre-approves the request.
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

You will need to create the custom fields in Jira to be used by the workflow:

| Custom Field  | Type           |
|---------------|----------------|
| Reporter      | Text           |
| Approver      | User Select    |
| Product Owner | User Select    |
| Justification | Text multiline |
| Cluster Role  | Single select  |
| Start Time    | Date and time  |
| End Time      | Date and time  |

The workflow used is [here](samples/workflow.xml), you need to [import](https://confluence.atlassian.com/display/ADMINJIRASERVER088/Using+XML+to+create+a+workflow)/create an identical Workflow in your Jira Project (the IDs of fields etc are configurable as below).

You must define these with the values according to your Jira Project and Workflow (to map the fields from your workflow to the opertor's config):
  - Allowed cluster roles
  - rejectedTransitionID
  - jiraProject
  - jiraIssueType
  - approvedTransitionID
  - customFields
      - Reporter
      - Approver
      - ProductOwner
      - Justification
      - ClusterRole
      - StartTime
      - EndTime

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
  reporter: test@foo.com
  clusterRole: edit
  approver: samirtahir91
  productOwner: samirtahir91
  justification: "need a jit now pls"
  namespace: foo
  startTime: 2024-12-09T14:31:00Z
  endTime: 2024-12-09T14:31:10Z
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

### Testing - TODO

Current integration tests cover the scenarios:
- TODO

**Run the controller in the foreground for testing:**
```sh
export JIT_RBAC_OPERATOR_CONFIG_PATH=/tmp/jit-test/
export OPERATOR_NAMESPACE=default
export JIRA_BASE_URL=http://127.0.0.1
export JIRA_API_TOKEN=<PERSONAL ACESS TOKEN>
# run
make run
```

**Run integration tests against a real cluster, i.e. Minikube:**
```sh
cd ..
USE_EXISTING_CLUSTER=true make test
```

**Run integration tests using env test (without a real cluster):**
```sh
USE_EXISTING_CLUSTER=false make test
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

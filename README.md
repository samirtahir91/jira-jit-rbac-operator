# jira-jit-rbac-operator - README UNDER CONSTRUCTION FOR JIRA INTEGRATIOM

The `jira-jit-rbac-operator` is a Kubernetes operator that creates short-lived rolebindings for users based on a JitRequest custom resource, it empowers self-service of Just-In-Time privileged access using Kubernetes RBAC.

## Description

### Key Features
- Uses a custom cluster scoped resource `JitRequest`.
- Reads `user`, `clusterRole` and `namespace` `startTime` and `endTime` from a `JitRequest`.
- Checks if the JitRequest's cluster role is allowed, from the `allowedClusterRoles` list defined in a `JustInTimeConfig` custom resource (set by admins/operators)

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

## Example `JitRequest` Resource

Here is an example of how to define the `JitRequest` resource:

```yaml
apiVersion: justintime.samir.io/v1
kind: JitRequest
metadata:
  name: jitrequest-sample
spec:
  user: samir@foo.dev
  clusterRole: edit
  namespace: blue-team
  startTime: 2024-12-05T09:00:00Z
  endTime: 2024-12-06T10:00:00Z
```

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.22.0+.
- Access to a Kubernetes v1.22.0+ cluster.

### To deploy with Helm using public Docker image
A helm chart is generated using `make helm` when a new tag is pushed, i.e a release.

You can pull the automatically built helm chart from this repos packages
- See the [packages](https://github.com/samirtahir91/jira-jit-rbac-operator/pkgs/container/jira-jit-rbac-operator%2Fhelm-charts%2Fjira-jit-rbac-operator)
- Pull with helm:
  - ```sh
    helm pull oci://ghcr.io/samirtahir91/jira-jit-rbac-operator/helm-charts/jira-jit-rbac-operator --version <TAG>
    ```
- Untar the chart and edit the `values.yaml` as required.
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
export JIRA_USERNAME=jira-rbac-operator
export JIRA_API_TOKEN=foobar
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

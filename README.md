# annotation-controller
Automatically injects or removes a specified annotation on every Pod matching a label selector, with full lifecycle hooks, status reporting, metrics, events and cleanup.
# Annotation Controller (PodAnnotator)

A Kubernetes controller built with Kubebuilder that allows you to declaratively annotate Pods based on arbitrary label selectors. By creating one or more `PodAnnotator` custom resources, you can instruct the controller to stamp every matching Pod with a specified annotation (e.g. a timestamp). When you delete the `PodAnnotator` resource, the controller cleans up the annotations automatically.

---

## Table of Contents

* [Features](#features)
* [Architecture](#architecture)
* [Custom Resource Definition](#custom-resource-definition)
* [Spec & Status](#spec--status)
* [Getting Started](#getting-started)

  * [Prerequisites](#prerequisites)
  * [Building & Pushing the Image](#building--pushing-the-image)
  * [Installing the Controller](#installing-the-controller)
  * [Deploying the Controller](#deploying-the-controller)
  * [Creating a PodAnnotator](#creating-a-podannotator)
  * [Verifying Annotations](#verifying-annotations)
  * [Cleanup on Delete](#cleanup-on-delete)
* [Development](#development)
* [Contributing](#contributing)
* [License](#license)

---

## Features

* **Declarative Annotation**: Define a `PodAnnotator` resource and have the controller annotate all matching Pods automatically.
* **Dynamic Value Support**: Use a fixed annotation value or let the controller inject the current timestamp.
* **Status Reporting**: Each `PodAnnotator` reports total Pods processed, how many were annotated, and a ready condition.
* **Cleanup on Delete**: When you delete a `PodAnnotator`, the controller removes the annotation from all affected Pods.
* **High Availability**: Leader election ensures only one active controller instance performs writes, even when multiple replicas are running.

---

## Architecture

1. **CustomResourceDefinition** (`PodAnnotator`)
2. **Controller** that watches `PodAnnotator` objects:

   * **Finalizer**: Added to new CRs to ensure cleanup.
   * **Reconcile Loop**:

     1. Fetch CR and handle deletion (cleanup) if requested.
     2. Add finalizer on fresh CRs.
     3. List all Pods matching the `.spec.selector`.
     4. Patch annotations for absent or outdated values.
     5. Update `.status` with counts and conditions.
     6. Emit Kubernetes Events for visibility.
     7. Requeue periodically for dynamic values (timestamps).

---

## Custom Resource Definition

```yaml
apiVersion: annotate.example.com/v1
kind: PodAnnotator
metadata:
  name: example-annotator
spec:
  selector:
    matchLabels:
      app: myapp
  annotation:
    key: example.com/last-seen
    # value: ""  # leave empty to inject timestamp
```

Apply the CRD to your cluster:

```bash
kubectl apply -k config/crd/bases/
```

---

## Spec & Status

### `spec`

| Field              | Type                  | Description                                          |
| ------------------ | --------------------- | ---------------------------------------------------- |
| `selector`         | `LabelSelector`       | Label selector to target Pods.                       |
| `annotation.key`   | `string`              | The annotation key to assign on each Pod.            |
| `annotation.value` | `string` *(optional)* | Fixed annotation value; if empty, timestamp is used. |

### `status`

| Field            | Type          | Description                                     |
| ---------------- | ------------- | ----------------------------------------------- |
| `podCount`       | `int32`       | Total number of Pods matching selector.         |
| `annotatedCount` | `int32`       | How many of those Pods were (re)annotated.      |
| `conditions`     | `[]Condition` | Standard Kubernetes Conditions (`Ready`, etc.). |

---

## Getting Started

### Prerequisites

* Go (1.18+)
* Docker CLI
* `kubectl` configured for your target cluster
* Access to a Docker registry (e.g. DockerHub, private registry)

### Building & Pushing the Image

```bash
# Set your image name
export IMG=your-registry/annotation-controller:latest

# Build
docker build -t $IMG .

# Push
docker push $IMG
```

### Installing the Controller

Install CRDs and RBAC:

```bash
kubectl apply -k config/crd/bases/
kubectl apply -k config/rbac/
```

### Deploying the Controller

Update `config/manager/kustomization.yaml` with your `$IMG`, then:

```bash
kubectl apply -k config/manager/
```

Check that the controller pod is running:

```bash
kubectl get pods -l control-plane=controller-manager
```

### Creating a PodAnnotator

```bash
kubectl apply -f config/samples/annotate_v1_podannotator.yaml
```

### Verifying Annotations

Create a test Pod with matching labels:

```bash
kubectl run test-nginx --image=nginx --labels=app=myapp
```

Inspect the annotation:

```bash
kubectl get pod test-nginx -o yaml | grep example.com/last-seen
```

### Cleanup on Delete

```bash
kubectl delete podannotator example-annotator
```

After deletion, the annotation will be removed from all affected Pods.

---

## Development

* **Run locally**: `make run` (requires access to a Kubernetes API via KUBECONFIG)
* **Generate code**: `make generate`
* **Build & push**: `make docker-build docker-push IMG=...`
* **Deploy**: `make deploy IMG=...`

---

## Contributing

Contributions are welcome! Please open issues or pull requests against this repository.

---

## License

Apache 2.0

---

## Publishing to GitHub

To publish this project to GitHub, follow these steps:

1. **Initialize your local repository** (if not already):

   ```bash
   git init
   git add .
   git commit -m "Initial commit"
   ```

2. **Create a GitHub repository**:

   * Go to [https://github.com/new](https://github.com/new)
   * Enter **Repository name**: `annotation-controller`
   * (Optional) add a description, choose **Public** or **Private**, and click **Create repository**.

3. **Add the GitHub remote**:

   ```bash
   git remote add origin git@github.com:<your-username>/annotation-controller.git
   ```

4. **Push your code**:

   ```bash
   git branch -M main
   git push -u origin main
   ```

After this, your repository will be available at `https://github.com/<your-username>/annotation-controller`, and you can continue to push changes with:

```bash
# Make your changes
git add .
git commit -m "Your message"
git push
```

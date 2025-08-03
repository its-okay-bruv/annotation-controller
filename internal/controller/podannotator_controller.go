/*
Copyright 2025.

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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	annotatev1 "example.com/api/v1"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PodAnnotatorReconciler reconciles a PodAnnotator object
type PodAnnotatorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const finalizerName = "annotate.example.com/finalizer"

//+kubebuilder:rbac:groups=annotate.example.com,resources=podannotators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=annotate.example.com,resources=podannotators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=annotate.example.com,resources=podannotators/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PodAnnotator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *PodAnnotatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	var annotator annotatev1.PodAnnotator
	if err := r.Get(ctx, req.NamespacedName, &annotator); err != nil {
		// If the resource no longer exists, we’re done
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !annotator.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.cleanupFinalizer(ctx, &annotator); err != nil {
			return ctrl.Result{}, err
		}
		// once cleanupFinalizer returns nil the finalizer is gone and CR will be deleted
		return ctrl.Result{}, nil
	}

	// Check for finalizer (first step for a CR is to add finalizer)
	if !controllerutil.ContainsFinalizer(&annotator, finalizerName) {
		controllerutil.AddFinalizer(&annotator, finalizerName)
		if err := r.Update(ctx, &annotator); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// annotator.Spec.Selector is a *metav1.LabelSelector, the "MatchingLabelsSelector" needs labels.Selector, thus the conversion
	sel, err := metav1.LabelSelectorAsSelector(&annotator.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}
	// List all Pods matching the CR selector to annotate
	var podList corev1.PodList
	if err := r.List(ctx, &podList,
		client.InNamespace(req.Namespace),
		client.MatchingLabelsSelector{Selector: sel},
	); err != nil {
		return ctrl.Result{}, err
	}

	total := len(podList.Items)
	updated := 0
	for _, pod := range podList.Items {
		desired := annotator.Spec.Annotation.Value
		if desired == "" {
			desired = time.Now().Format(time.RFC3339)
		}
		if pod.Annotations[annotator.Spec.Annotation.Key] != desired {
			orig := pod.DeepCopy()
			if pod.Annotations == nil {
				pod.Annotations = make(map[string]string)
			}
			pod.Annotations[annotator.Spec.Annotation.Key] = desired
			if err := r.Patch(ctx, &pod, client.MergeFrom(orig)); err != nil {
				r.Recorder.Event(&annotator, corev1.EventTypeWarning,
					"AnnotateFailed", fmt.Sprintf("pod: %s: %v", pod.Name, err))
				continue
			}
			updated++
		}
	}

	annotator.Status.PodCount = int32(total)
	annotator.Status.AnnotatedCount = int32(updated)
	annotator.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "AllPodsAnnotated",
			Message:            fmt.Sprintf("Annotated %d/%d pods", updated, total),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: annotator.Generation,
		},
	}
	if err := r.Status().Update(ctx, &annotator); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(&annotator, corev1.EventTypeNormal,
		"Synced", "Annotated %d/%d pods", updated, total)
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodAnnotatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("podannotator-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&annotatev1.PodAnnotator{}).
		Complete(r)
}

func (r *PodAnnotatorReconciler) cleanupFinalizer(ctx context.Context, annotator *annotatev1.PodAnnotator) error {
	// Convert selector the same way
	sel, err := metav1.LabelSelectorAsSelector(&annotator.Spec.Selector)
	if err != nil {
		return fmt.Errorf("invalid selector on delete: %w", err)
	}

	// List pods of CR
	var podList corev1.PodList
	if err := r.List(ctx, &podList,
		client.InNamespace(annotator.Namespace),
		client.MatchingLabelsSelector{Selector: sel},
	); err != nil {
		return err
	}

	for _, pod := range podList.Items {
		orig := pod.DeepCopy()
		if pod.Annotations != nil {
			delete(pod.Annotations, annotator.Spec.Annotation.Key)
		}
		if err := r.Patch(ctx, &pod, client.MergeFrom(orig)); err != nil {
			r.Recorder.Eventf(annotator, corev1.EventTypeWarning,
				"CleanupFailed", "pod %s: %v", pod.Name, err)
		}
	}

	controllerutil.RemoveFinalizer(annotator, finalizerName)
	if err := r.Update(ctx, annotator); err != nil {
		return err
	}

	return nil
}

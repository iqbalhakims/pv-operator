package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/iqbalhakims/pv-operator/internal/guard"
)

// OriginalPolicyAnnotation records the reclaim policy a PV had before we changed it.
const OriginalPolicyAnnotation = "pv-guard.io/original-reclaim-policy"

// RetainReconciler switches PVs from reclaimPolicy Delete to Retain so the
// underlying storage survives even if the PV or PVC object is removed.
type RetainReconciler struct {
	client.Client
}

func (r *RetainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pv := &corev1.PersistentVolume{}
	if err := r.Get(ctx, req.NamespacedName, pv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if pv.DeletionTimestamp != nil || guard.DeletionAllowed(pv) ||
		pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimDelete {
		return ctrl.Result{}, nil
	}

	patch := client.MergeFrom(pv.DeepCopy())
	if pv.Annotations == nil {
		pv.Annotations = map[string]string{}
	}
	pv.Annotations[OriginalPolicyAnnotation] = string(corev1.PersistentVolumeReclaimDelete)
	pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	if err := r.Patch(ctx, pv, patch); err != nil {
		return ctrl.Result{}, err
	}
	logf.FromContext(ctx).Info("set reclaimPolicy to Retain", "pv", pv.Name)
	return ctrl.Result{}, nil
}

func (r *RetainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.PersistentVolume{}).
		Named("pv-retain").
		Complete(r)
}

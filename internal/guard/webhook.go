package guard

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Handler is a validating webhook that rejects DELETE on protected
// PersistentVolumes, and on PersistentVolumeClaims whose deletion would
// cascade into deleting their PV (reclaimPolicy: Delete).
type Handler struct {
	Client  client.Reader
	Decoder admission.Decoder
	Policy  Policy
}

func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Delete {
		return admission.Allowed("")
	}
	log := logf.FromContext(ctx).WithValues("kind", req.Kind.Kind, "namespace", req.Namespace, "name", req.Name, "user", req.UserInfo.Username)

	if h.Policy.Privileged(req.UserInfo) {
		log.Info("allowing delete by privileged user")
		return admission.Allowed("privileged user")
	}

	switch req.Kind.Kind {
	case "PersistentVolume":
		pv := &corev1.PersistentVolume{}
		if err := h.Decoder.DecodeRaw(req.OldObject, pv); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if DeletionAllowed(pv) {
			return admission.Allowed("PV is annotated " + AllowDeleteAnnotation)
		}
		log.Info("denied PV delete")
		return admission.Denied(fmt.Sprintf(
			"PersistentVolume %q is protected by pv-guard. Ask a cluster admin to annotate it with %s=true first.",
			pv.Name, AllowDeleteAnnotation))

	case "PersistentVolumeClaim":
		if !h.Policy.ProtectPVCs {
			return admission.Allowed("")
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := h.Decoder.DecodeRaw(req.OldObject, pvc); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if pvc.Spec.VolumeName == "" {
			return admission.Allowed("PVC is not bound")
		}
		pv := &corev1.PersistentVolume{}
		if err := h.Client.Get(ctx, client.ObjectKey{Name: pvc.Spec.VolumeName}, pv); err != nil {
			if apierrors.IsNotFound(err) {
				return admission.Allowed("bound PV no longer exists")
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimDelete {
			return admission.Allowed("bound PV is retained after PVC deletion")
		}
		if DeletionAllowed(pv) {
			return admission.Allowed("bound PV is annotated " + AllowDeleteAnnotation)
		}
		log.Info("denied PVC delete", "pv", pv.Name)
		return admission.Denied(fmt.Sprintf(
			"deleting PersistentVolumeClaim %s/%s would delete PersistentVolume %q (reclaimPolicy: Delete), which is protected by pv-guard. "+
				"Ask a cluster admin to annotate the PV with %s=true or change its reclaimPolicy to Retain.",
			pvc.Namespace, pvc.Name, pv.Name, AllowDeleteAnnotation))
	}

	return admission.Allowed("")
}

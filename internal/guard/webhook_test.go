package guard

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	dev   = authenticationv1.UserInfo{Username: "alice", Groups: []string{"developers", "system:authenticated"}}
	admin = authenticationv1.UserInfo{Username: "bob", Groups: []string{"system:masters"}}
)

func pv(name string, policy corev1.PersistentVolumeReclaimPolicy, annotations map[string]string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
		Spec:       corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: policy},
	}
}

func pvc(volumeName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "team-a"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: volumeName},
	}
}

func deleteReq(t *testing.T, kind string, obj client.Object, user authenticationv1.UserInfo) admission.Request {
	t.Helper()
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Delete,
		Kind:      metav1.GroupVersionKind{Version: "v1", Kind: kind},
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		UserInfo:  user,
		OldObject: runtime.RawExtension{Raw: raw},
	}}
}

func TestHandle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	deletePV := pv("pv-delete", corev1.PersistentVolumeReclaimDelete, nil)
	retainPV := pv("pv-retain", corev1.PersistentVolumeReclaimRetain, nil)
	releasedPV := pv("pv-released", corev1.PersistentVolumeReclaimDelete, map[string]string{AllowDeleteAnnotation: "true"})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deletePV, retainPV, releasedPV).Build()

	tests := []struct {
		name        string
		kind        string
		obj         client.Object
		user        authenticationv1.UserInfo
		protectPVCs bool
		allowed     bool
	}{
		{"dev cannot delete PV", "PersistentVolume", deletePV, dev, true, false},
		{"dev cannot delete retained PV", "PersistentVolume", retainPV, dev, true, false},
		{"admin can delete PV", "PersistentVolume", deletePV, admin, true, true},
		{"dev can delete annotated PV", "PersistentVolume", releasedPV, dev, true, true},
		{"dev cannot delete PVC bound to Delete PV", "PersistentVolumeClaim", pvc("pv-delete"), dev, true, false},
		{"dev can delete PVC bound to Retain PV", "PersistentVolumeClaim", pvc("pv-retain"), dev, true, true},
		{"dev can delete PVC bound to annotated PV", "PersistentVolumeClaim", pvc("pv-released"), dev, true, true},
		{"dev can delete unbound PVC", "PersistentVolumeClaim", pvc(""), dev, true, true},
		{"dev can delete PVC whose PV is gone", "PersistentVolumeClaim", pvc("missing"), dev, true, true},
		{"PVC protection disabled", "PersistentVolumeClaim", pvc("pv-delete"), dev, false, true},
		{"admin can delete PVC", "PersistentVolumeClaim", pvc("pv-delete"), admin, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				Client:  c,
				Decoder: admission.NewDecoder(scheme),
				Policy:  Policy{AllowedGroups: []string{"system:masters"}, ProtectPVCs: tt.protectPVCs},
			}
			resp := h.Handle(context.Background(), deleteReq(t, tt.kind, tt.obj, tt.user))
			if resp.Allowed != tt.allowed {
				t.Fatalf("allowed = %v, want %v (%v)", resp.Allowed, tt.allowed, resp.Result)
			}
		})
	}
}

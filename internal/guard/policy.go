package guard

import (
	"slices"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
)

// AllowDeleteAnnotation marks a PersistentVolume as safe to delete. PVs are
// cluster-scoped, so normally only cluster admins can set it.
const AllowDeleteAnnotation = "pv-guard.io/allow-delete"

// Policy decides who may bypass the guard.
type Policy struct {
	// AllowedUsers may always delete PVs/PVCs (e.g. the PV controller, CSI provisioners).
	AllowedUsers []string
	// AllowedGroups may always delete PVs/PVCs (e.g. system:masters, a platform-admin group).
	AllowedGroups []string
	// ProtectPVCs also blocks deleting a PVC whose bound PV would be deleted with it.
	ProtectPVCs bool
}

// Privileged reports whether the requester is on the allow-list.
func (p Policy) Privileged(u authenticationv1.UserInfo) bool {
	if slices.Contains(p.AllowedUsers, u.Username) {
		return true
	}
	for _, g := range u.Groups {
		if slices.Contains(p.AllowedGroups, g) {
			return true
		}
	}
	return false
}

// DeletionAllowed reports whether an admin has explicitly released the PV.
func DeletionAllowed(pv *corev1.PersistentVolume) bool {
	return pv.Annotations[AllowDeleteAnnotation] == "true"
}

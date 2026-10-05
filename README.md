# pv-guard

pv-guard is a Kubernetes operator that stops developers from deleting PersistentVolumes, and from deleting PVCs in ways that would take a PV's data with them.

## How it works

The main piece is a **validating admission webhook** on `DELETE` for `persistentvolumes` and `persistentvolumeclaims`. The API server rejects the request before anything is deleted:

| Request | Result |
|---|---|
| `kubectl delete pv X` by a non-admin | **Denied** |
| `kubectl delete pvc Y` where the bound PV has `reclaimPolicy: Delete` | **Denied** (deleting the PVC would delete the PV) |
| `kubectl delete pvc Y` where the bound PV has `reclaimPolicy: Retain` | Allowed (the data stays) |
| Unbound PVC | Allowed |
| Any user in `--allowed-users` / `--allowed-groups` | Allowed |
| PV annotated `pv-guard.io/allow-delete="true"` | Allowed |

PVs are cluster-scoped, so developers normally can't add the annotation. Only cluster admins can release a PV.

You can turn on an optional controller (`--enforce-retain=true`) that switches every PV from `reclaimPolicy: Delete` to `Retain`. It records the original value in the `pv-guard.io/original-reclaim-policy` annotation. With it on, the storage backend keeps the disk even if the PV object is removed.

## Install

### Helm

```sh
helm install pv-guard ./charts/pv-guard -n pv-guard-system --create-namespace \
  --set image.repository=registry.digitalocean.com/pv-reg/pv-guard \
  --set image.tag=v0.1.0 \
  --set 'policy.allowedGroups={system:masters,platform-admins}'
```

By default the chart gets the webhook's TLS certificate from [cert-manager](https://cert-manager.io/docs/installation/). If you don't run cert-manager, add `--set certManager.enabled=false`. The chart then generates a self-signed certificate and keeps it across upgrades. All options are documented in [charts/pv-guard/values.yaml](charts/pv-guard/values.yaml).

### Kustomize

You need [cert-manager](https://cert-manager.io/docs/installation/) for the webhook's TLS certificate.

```sh
make docker-build IMG=registry.digitalocean.com/pv-reg/pv-guard:v0.1.0
docker push registry.digitalocean.com/pv-reg/pv-guard:v0.1.0
make deploy IMG=registry.digitalocean.com/pv-reg/pv-guard:v0.1.0
```

Set who may bypass the guard in the container args in [config/deployment.yaml](config/deployment.yaml):

- `--allowed-groups`: for example `system:masters,platform-admins`
- `--allowed-users`: keep the PV controller here. **Add your CSI provisioner's service account** (for example `system:serviceaccount:kube-system:ebs-csi-controller-sa`) so it can clean up PVs after an admin approves a delete.
- `--protect-pvcs`: defaults to `true`.
- `--enforce-retain`: defaults to `false`.

## Try it

```sh
kubectl delete pv some-pv --as alice --as-group developers
# Error from server: admission webhook "storage.pv-guard.io" denied the request:
# PersistentVolume "some-pv" is protected by pv-guard. Ask a cluster admin to annotate it with pv-guard.io/allow-delete=true first.

# An admin releases it:
kubectl annotate pv some-pv pv-guard.io/allow-delete=true
```

## Things to know

- **The webhook fails closed** (`failurePolicy: Fail`). If pv-guard is down, nobody can delete PVs or PVCs. The deployment runs 2 replicas with a PodDisruptionBudget. In an emergency, run `kubectl delete validatingwebhookconfiguration pv-guard`.
- **Deleting a namespace** that contains protected PVCs will hang in `Terminating`. This is on purpose: the data is kept. An admin can unblock it by annotating the PVs or setting their reclaimPolicy to `Retain`.
- **Combine pv-guard with RBAC.** Developers shouldn't have `delete` on `persistentvolumes` in the first place. pv-guard is the safety net for people who do have broad access, such as `cluster-admin` given to a dev team.

## License

Apache License 2.0. See [LICENSE](LICENSE).

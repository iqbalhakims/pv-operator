# Project Memory
Last updated: 2026-10-05T07:08:54.507Z
Mode: local (no API key — paste this at the start of a new session)

## Files Touched This Session
- /Users/iqbalfauzi/pv-operator/internal/guard/policy.go
- /Users/iqbalfauzi/pv-operator/internal/guard/webhook.go
- /Users/iqbalfauzi/pv-operator/internal/controller/retain.go
- /Users/iqbalfauzi/pv-operator/cmd/main.go
- /Users/iqbalfauzi/pv-operator/internal/guard/webhook_test.go
- /Users/iqbalfauzi/pv-operator/README.md

## Commands Run
- `cat > Dockerfile <<'EOF'`
- `ls -la /Users/iqbalfauzi/pv-operator /Users/iqbalfauzi/pv-operator/config`
- `P=/Users/iqbalfauzi/pv-operator; mv $P/config/Dockerfile $P/config/.dockerignore $P/ && rm $P/config/Makefile && cat > $`
- `cd /Users/iqbalfauzi/pv-operator && docker info >/dev/null 2>&1 && docker build -q -t pv-guard:latest . 2>&1 | tail -5 |`
- `which helm && helm version --short`
- `C=/Users/iqbalfauzi/pv-operator/charts/pv-guard; mkdir -p $C/templates`
- `for cm in true false; do echo "== certManager.enabled=$cm"; helm template t charts/pv-guard -n pv-guard-system --set cer`
- `cd /private/tmp/claude-501/-Users-iqbalfauzi-pv-operator/1dc5f6d0-1b65-472b-b44f-d7dae6810c34/scratchpad && for f in r-t`
- `curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE && head -3 LICENSE && wc -l LICENSE && shasum -a 2`
- `printf '\n## License\n\nApache License 2.0. See [LICENSE](LICENSE).\n' >> README.md && cat >> charts/pv-guard/Chart.yaml`

## Recent Conversation
…(earlier turns omitted)


   - It decodes the PV from the request (`req.OldObject`, the object about to be deleted).
   - If the PV has the allow-delete annotation, allow. Otherwise **deny**.
4. **Deleting a PVC** (only when `ProtectPVCs` is on):
   - PVC not bound to a PV: allow, since there's no data to lose.
   - It looks up the bound PV from the cluster.
   - PV already gone: allow.
   - PV has `reclaimPolicy` other than `Delete`, such as `Retain`: allow, because the disk survives.
   - PV has the allow-delete annotation: allow.
   - Otherwise **deny**, because deleting this PVC would also delete the PV and its data.

The deny messages tell the developer exactly what an admin needs to do.

The PVC check exists because it's the sneaky path. Developers rarely delete PVs directly, but deleting a PVC whose PV has `reclaimPolicy: Delete` makes Kubernetes delete the PV and the actual disk too. Deleting a namespace or a StatefulSet can do the same thing.

### [internal/controller/retain.go](internal/controller/retain.go): optional Retain controller
This is a standard Kubernetes controller: it watches every PV, and `Reconcile()` runs whenever one is created or changed. If a PV has `reclaimPolicy: Delete`, it:
- saves the original value in the annotation `pv-guard.io/original-reclaim-policy`
- patches the PV to `Retain`

It's the second layer of protection: even if a PV object does get deleted, the storage backend keeps the real disk. It's off by default because it changes behavior. Deleted volumes then stay around until someone cleans them up, which costs money.

### [internal/guard/webhook_test.go](internal/guard/webhook_test.go): tests
These are table-driven tests with 11 cases: developer vs. admin, Delete vs. Retain PVs, annotated, unbound and missing PVs, and PVC protection on and off. A fake Kubernetes client stands in for the cluster, so the tests run in under a second with no cluster. Run them with `make test`.

## Deployment pieces

The same set of resources ships in both [config/](config/) for Kustomize and [charts/pv-guard/](charts/pv-guard/) for Helm:

| Resource | Purpose |
|---|---|
| ServiceAccount + ClusterRole | The identity pv-guard runs as. It can read PVs (for the PVC check) and patch them (for the Retain controller). |
| Certificate (cert-manager) | The API server only calls webhooks over HTTPS, so pv-guard needs a TLS certificate. |
| Deployment (2 replicas) + PodDisruptionBudget | Keeps at least one pod running. The webhook fails closed, so if it's down nobody can delete PVs. |
| Service | Gives the API server a stable address for reaching the pods. |
| ValidatingWebhookConfiguration | Tells the API server to send PV and PVC deletes to pv-guard. |

## Design choices
- **A webhook, not a finalizer.** A finalizer can only delay a delete; the PV still ends up stuck in Terminating. A webhook stops the request before anything happens.
- **Fails closed.** If pv-guard crashes, deletes are blocked rather than let through. That's safer for data, at the cost of having to remove the webhook config in an emergency.
- **Admins release PVs with an annotation.** PVs aren't namespaced, so developers usually can't annotate them, and only admins can approve a delete.
#!/usr/bin/env bash
# Destructive acceptance test confined to an empty, explicitly selected kind cluster.
set -euo pipefail

: "${KUBECONFIG:?Set KUBECONFIG to the disposable kind cluster kubeconfig}"
: "${CHAINSAW_CLUSTER:?Set the disposable kind cluster name}"
: "${UPGRADE_BASELINE_DIR:?Provide a checkout of the fixed pre-framework operator}"
: "${UPGRADE_BASELINE_REF:?Provide the exact pre-framework commit}"
: "${UPGRADE_BASELINE_IMAGE:?Build the pre-framework image first}"
: "${UPGRADE_WORKTREE_DIR:?Provide the framework worktree}"
: "${UPGRADE_FRAMEWORK_REF:?Provide the exact framework HEAD}"
: "${IMG:?Build the framework image first}"
: "${UPGRADE_FIXTURE_DIR:?Provide the frozen framework-upgrade fixture directory}"
: "${UPGRADE_COMPARATOR:?Provide the frozen resource comparator}"
: "${UPGRADE_INTENTIONAL_DIFFS:?Provide the frozen intentional-differences document}"

product_version=${PRODUCT_VERSION:-4.1.2}
kubedoop_version=${KUBEDOOP_VERSION:-0.0.0-dev}
evidence_parent=${UPGRADE_EVIDENCE_DIR:-$UPGRADE_WORKTREE_DIR/upgrade-evidence}
kustomize_bin=${KUSTOMIZE_BIN:-$UPGRADE_WORKTREE_DIR/bin/kustomize}
kind_bin=${KIND_BIN:-kind}
kubectl_bin=${KUBECTL_BIN:-kubectl}
container_tool_bin=${CONTAINER_TOOL_BIN:-docker}
namespace=superset-framework-upgrade
operator_namespace=superset-operator-system
operator_deployment=superset-operator-controller-manager
wait_seconds=${UPGRADE_WAIT_SECONDS:-900}
operator_cluster_resources=(
    clusterrole/superset-operator-manager-role
    clusterrole/superset-operator-metrics-auth-role
    clusterrole/superset-operator-metrics-reader
    clusterrole/superset-operator-supersetcluster-admin-role
    clusterrole/superset-operator-supersetcluster-editor-role
    clusterrole/superset-operator-supersetcluster-viewer-role
    clusterrolebinding/superset-operator-manager-rolebinding
    clusterrolebinding/superset-operator-metrics-auth-rolebinding
)

for command in "$kubectl_bin" "$kind_bin" "$container_tool_bin" git python3 sed sha256sum; do
    command -v "$command" >/dev/null 2>&1 || {
        echo "Required command is unavailable: $command" >&2
        exit 1
    }
done
for path in "$UPGRADE_BASELINE_DIR/config" "$UPGRADE_WORKTREE_DIR/config" \
    "$UPGRADE_FIXTURE_DIR/postgres.yaml" "$UPGRADE_FIXTURE_DIR/superset.yaml" \
    "$UPGRADE_COMPARATOR" "$UPGRADE_INTENTIONAL_DIFFS"; do
    test -e "$path" || {
        echo "Required migration input is missing: $path" >&2
        exit 1
    }
done
test -x "$kustomize_bin" || {
    echo "kustomize is not executable: $kustomize_bin" >&2
    exit 1
}
test -s "$KUBECONFIG" || {
    echo "The isolated-cluster kubeconfig is missing or empty: $KUBECONFIG" >&2
    exit 1
}
case "$product_version:$kubedoop_version" in
    *[!A-Za-z0-9._:-]*)
        echo "Product and Kubedoop versions must be safe image/version tokens" >&2
        exit 1
        ;;
esac

baseline_head=$(git -C "$UPGRADE_BASELINE_DIR" rev-parse HEAD)
framework_head=$(git -C "$UPGRADE_WORKTREE_DIR" rev-parse HEAD)
test "$baseline_head" = "$UPGRADE_BASELINE_REF" || {
    echo "Baseline checkout moved: expected $UPGRADE_BASELINE_REF, got $baseline_head" >&2
    exit 1
}
test "$framework_head" = "$UPGRADE_FRAMEWORK_REF" || {
    echo "Framework worktree moved: expected $UPGRADE_FRAMEWORK_REF, got $framework_head" >&2
    exit 1
}
test -z "$(git -C "$UPGRADE_BASELINE_DIR" status --porcelain)" || {
    echo "The fixed baseline checkout must be clean: $UPGRADE_BASELINE_DIR" >&2
    exit 1
}
test -z "$(git -C "$UPGRADE_WORKTREE_DIR" status --porcelain)" || {
    echo "The framework worktree must be clean: $UPGRADE_WORKTREE_DIR" >&2
    exit 1
}
"$container_tool_bin" image inspect "$UPGRADE_BASELINE_IMAGE" >/dev/null 2>&1 || {
    echo "Baseline image is unavailable locally: $UPGRADE_BASELINE_IMAGE" >&2
    exit 1
}
"$container_tool_bin" image inspect "$IMG" >/dev/null 2>&1 || {
    echo "Framework image is unavailable locally: $IMG" >&2
    exit 1
}

mkdir -p "$evidence_parent"
evidence_parent=$(cd "$evidence_parent" && pwd)
evidence=$(mktemp -d "$evidence_parent/run.XXXXXX")
scratch=$(mktemp -d)
mkdir -p "$evidence/acceptance-assets"
cp "$UPGRADE_COMPARATOR" "$evidence/acceptance-assets/compare-upgrade-resources.py"
cp "$UPGRADE_INTENTIONAL_DIFFS" "$evidence/acceptance-assets/intentional-differences.md"
cp -R "$UPGRADE_FIXTURE_DIR" "$evidence/acceptance-assets/framework-upgrade"
printf '%s\n' "Migration evidence: $evidence"

k() {
    "$kubectl_bin" -n "$namespace" "$@"
}

kubectl_cmd() {
    "$kubectl_bin" "$@"
}

kind_cmd() {
    "$kind_bin" "$@"
}

capture_failure() {
    local result=$1
    printf 'FAIL (exit %s); cluster and namespace retained for inspection\n' "$result" \
        > "$evidence/result.txt"
    kubectl_cmd get nodes -o wide > "$evidence/failure-nodes.txt" 2>&1 || true
    k get supersetcluster,sts,pod,pvc,svc,pdb,sa,cm -o yaml \
        > "$evidence/failure-resources.yaml" 2>&1 || true
    k get listeners.listeners.kubedoop.dev -o yaml \
        > "$evidence/failure-listeners.yaml" 2>&1 || true
    kubectl_cmd -n "$operator_namespace" get deployment,serviceaccount,role,rolebinding,service -o yaml \
        > "$evidence/failure-operator-namespaced.yaml" 2>&1 || true
    kubectl_cmd get "${operator_cluster_resources[@]}" -o yaml \
        > "$evidence/failure-operator-cluster.yaml" 2>&1 || true
    kubectl_cmd get crd supersetclusters.superset.kubedoop.dev -o yaml \
        > "$evidence/failure-crd.yaml" 2>&1 || true
    k get events --sort-by=.lastTimestamp > "$evidence/failure-events.txt" 2>&1 || true
    kubectl_cmd -n "$operator_namespace" logs "deployment/$operator_deployment" --all-containers \
        --tail=400 > "$evidence/failure-operator.log" 2>&1 || true
    for pod in upgrade-node-primary-0 upgrade-node-secondary-0 upgrade-postgres-0; do
        k logs "$pod" --all-containers --tail=250 \
            > "$evidence/failure-$pod.log" 2>&1 || true
    done
}

finish() {
    local result=$?
    trap - EXIT
    if (( result != 0 )); then
        capture_failure "$result"
    fi
    rm -rf "$scratch"
    exit "$result"
}
trap finish EXIT

wait_for() {
    local description=$1
    shift
    local deadline=$((SECONDS + wait_seconds))
    until "$@"; do
        if (( SECONDS >= deadline )); then
            echo "Timed out after ${wait_seconds}s: $description" >&2
            k get sts,pod,pvc,svc,pdb -o wide >&2 || true
            k get events --sort-by=.lastTimestamp >&2 || true
            return 1
        fi
        sleep 5
    done
}

statefulset_ready() {
    k get sts "$1" -o json | python3 -c '
import json
import sys

obj = json.load(sys.stdin)
spec = obj["spec"]
status = obj.get("status", {})
desired = spec.get("replicas", 0)
assert desired == 1
assert status.get("observedGeneration", 0) >= obj["metadata"]["generation"]
assert status.get("readyReplicas", 0) == desired
assert status.get("currentReplicas", 0) == desired
' 2>/dev/null
}

pdb_ready() {
    k get pdb upgrade-node -o json | python3 -c '
import json
import sys

obj = json.load(sys.stdin)
status = obj.get("status", {})
assert status.get("observedGeneration", 0) >= obj["metadata"]["generation"]
assert status.get("expectedPods", 0) == 2
assert status.get("currentHealthy", 0) == 2
assert status.get("disruptionsAllowed", 0) == 1
' 2>/dev/null
}

render_operator() {
    local source=$1
    local image=$2
    local output=$3
    local render
    render=$(mktemp -d "$scratch/render.XXXXXX")
    cp -R "$source/config" "$render/config"
    (cd "$render/config/manager" && "$kustomize_bin" edit set image "controller=$image")
    "$kustomize_bin" build "$render/config/default" > "$output"
}

deploy_operator() {
    kubectl_cmd apply -f "$1"
    # `kubectl scale` is an out-of-band field manager. A later three-way
    # `kubectl apply` may preserve the live zero instead of restoring the
    # manifest's unchanged replicas=1, so start the selected controller
    # explicitly before treating rollout status as evidence.
    kubectl_cmd -n "$operator_namespace" scale "deployment/$operator_deployment" --replicas=1
    kubectl_cmd -n "$operator_namespace" rollout status "deployment/$operator_deployment" \
        --timeout=300s
}

stop_operator() {
    kubectl_cmd -n "$operator_namespace" scale "deployment/$operator_deployment" --replicas=0
    wait_for 'operator pod stopped' operator_stopped
}

operator_stopped() {
    test -z "$(kubectl_cmd -n "$operator_namespace" get pods \
        -l control-plane=controller-manager -o name)"
}

pod_absent() {
    test -z "$(k get pod "$1" --ignore-not-found -o name)"
}

replace_superset_workloads() {
    local transition=$1
    echo "$transition: stopping the operator and rebuilding immutable StatefulSet shapes"
    stop_operator
    for group in primary secondary; do
        local workload="upgrade-node-$group"
        k scale sts "$workload" --replicas=0
        wait_for "$workload pod stopped" pod_absent "$workload-0"
        k delete sts "$workload" --wait=true --timeout=180s
    done
}

db_query() {
    local query=$1
    k exec upgrade-postgres-0 -- env PGPASSWORD=superset \
        psql -X -h 127.0.0.1 -U superset -d superset -At \
        -v ON_ERROR_STOP=1 -c "$query"
}

database_ready() {
    test "$(db_query 'SELECT 1' 2>/dev/null)" = 1
}

mark_phase() {
    local phase=$1
    db_query "INSERT INTO kubedoop_framework_upgrade_marker(phase) VALUES ('$phase') ON CONFLICT DO NOTHING" \
        >/dev/null
}

capture_database() {
    local phase=$1
    db_query "SELECT id || '|' || username || '|' || email FROM ab_user WHERE username = 'admin' ORDER BY id" \
        > "$evidence/$phase-admin-user.txt"
    test -s "$evidence/$phase-admin-user.txt"
    db_query 'SELECT phase FROM kubedoop_framework_upgrade_marker ORDER BY phase' \
        > "$evidence/$phase-markers.txt"
}

probe_api_once() {
    local phase=$1
    local output="$scratch/$phase-api.jsonl"
    if k exec -i upgrade-client -- python - "$phase" > "$output" <<'PY'
import json
import sys

import requests

phase = sys.argv[1]
failed = False
for group in ("primary", "secondary"):
    base = f"http://upgrade-node-{group}:8088"
    record = {
        "phase": phase,
        "group": group,
        "health_status": 0,
        "login_status": 0,
        "access_token": False,
    }
    try:
        health = requests.get(base + "/health", timeout=10)
        record["health_status"] = health.status_code
        login = requests.post(
            base + "/api/v1/security/login",
            json={
                "username": "admin",
                "password": "admin",
                "provider": "db",
                "refresh": True,
            },
            timeout=20,
        )
        record["login_status"] = login.status_code
        if login.status_code == 200:
            record["access_token"] = bool(login.json().get("access_token"))
    except Exception as error:  # printed for retry diagnostics; contains no credentials
        record["error"] = f"{type(error).__name__}: {error}"
    print(json.dumps(record, sort_keys=True))
    if (
        record["health_status"] != 200
        or record["login_status"] != 200
        or not record["access_token"]
    ):
        failed = True
raise SystemExit(1 if failed else 0)
PY
    then
        cp "$output" "$evidence/$phase-api.jsonl"
        return 0
    fi
    return 1
}

application_logs_ready() {
    local group
    for group in primary secondary; do
        k exec "upgrade-node-$group-0" -c node -- \
            sh -c 'test -s /kubedoop/log/superset/superset.py.json' \
            >/dev/null 2>&1 || return 1
    done
}

capture_application_logs() {
    local phase=$1
    local group
    for group in primary secondary; do
        k exec "upgrade-node-$group-0" -c node -- \
            sh -c 'wc -c < /kubedoop/log/superset/superset.py.json' \
            > "$evidence/$phase-$group-application-log-bytes.txt"
    done
}

snapshot() {
    local phase=$1
    k get sts,cm,svc,pdb,sa -o json > "$evidence/$phase-resources.json"
    k get supersetcluster upgrade -o yaml > "$evidence/$phase-cluster.yaml"
    k get supersetcluster upgrade -o jsonpath='{.metadata.uid}' \
        > "$evidence/$phase-cluster-uid.txt"
    printf '\n' >> "$evidence/$phase-cluster-uid.txt"
    k get pvc -o json > "$evidence/$phase-pvcs.json"
    local postgres_volume
    postgres_volume=$(k get pvc data-upgrade-postgres-0 -o jsonpath='{.spec.volumeName}')
    test -n "$postgres_volume"
    kubectl_cmd get pv "$postgres_volume" -o json > "$evidence/$phase-postgres-pv.json"
    k get pods -o json > "$evidence/$phase-pods.json"
    k get listeners.listeners.kubedoop.dev -o json \
        > "$evidence/$phase-listeners.json"
    kubectl_cmd -n "$operator_namespace" get deployment,serviceaccount,role,rolebinding,service -o json \
        > "$evidence/$phase-operator-namespaced.json"
    kubectl_cmd get "${operator_cluster_resources[@]}" -o json \
        > "$evidence/$phase-operator-cluster.json"
    kubectl_cmd get crd supersetclusters.superset.kubedoop.dev -o json \
        > "$evidence/$phase-crd.json"
    k get secret upgrade-credentials -o json | python3 -c '
import hashlib
import json
import sys

secret = json.load(sys.stdin)
identity = {
    "name": secret["metadata"]["name"],
    "uid": secret["metadata"]["uid"],
    "data_sha256": hashlib.sha256(
        json.dumps(secret.get("data", {}), sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest(),
}
print(json.dumps(identity, sort_keys=True))
' > "$evidence/$phase-credentials.json"
    capture_database "$phase"
    capture_application_logs "$phase"
}

current_context=$(kubectl_cmd config current-context)
test "$current_context" = "kind-$CHAINSAW_CLUSTER" || {
    echo "Refusing cluster context $current_context; expected kind-$CHAINSAW_CLUSTER" >&2
    exit 1
}
kind_cmd get clusters | grep -Fx -- "$CHAINSAW_CLUSTER" >/dev/null || {
    echo "The selected kind cluster does not exist: $CHAINSAW_CLUSTER" >&2
    exit 1
}
kubectl_cmd wait --for=condition=Ready nodes --all --timeout=180s
if kubectl_cmd get namespace "$namespace" >/dev/null 2>&1; then
    echo "Namespace $namespace already exists; inspect/clean the previous run first" >&2
    exit 1
fi
if kubectl_cmd get namespace "$operator_namespace" >/dev/null 2>&1; then
    echo "Operator namespace already exists; this is not a fresh acceptance cluster" >&2
    exit 1
fi
if kubectl_cmd get crd supersetclusters.superset.kubedoop.dev >/dev/null 2>&1 && \
    test -n "$(kubectl_cmd get supersetclusters.superset.kubedoop.dev -A -o name)"; then
    echo "Existing SupersetClusters would be affected; use a fresh cluster" >&2
    exit 1
fi

{
    printf 'started_at=%s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
    printf 'cluster=%s\n' "$CHAINSAW_CLUSTER"
    printf 'context=%s\n' "$current_context"
    printf 'baseline_ref=%s\n' "$baseline_head"
    printf 'framework_ref=%s\n' "$framework_head"
    printf 'baseline_image=%s\n' "$UPGRADE_BASELINE_IMAGE"
    printf 'framework_image=%s\n' "$IMG"
    printf 'product_version=%s\n' "$product_version"
    printf 'kubedoop_version=%s\n' "$kubedoop_version"
    printf 'kubectl_bin=%s\n' "$kubectl_bin"
    printf 'kind_bin=%s\n' "$kind_bin"
    printf 'container_tool_bin=%s\n' "$container_tool_bin"
} > "$evidence/run-metadata.txt"
git -C "$UPGRADE_WORKTREE_DIR" status --short > "$evidence/framework-git-status.txt"
git -C "$UPGRADE_WORKTREE_DIR" diff --stat HEAD > "$evidence/framework-worktree-diff-stat.txt"
git -C "$UPGRADE_WORKTREE_DIR" diff --binary HEAD | sha256sum \
    > "$evidence/framework-worktree-diff.sha256"
git -C "$UPGRADE_WORKTREE_DIR" ls-files --others --exclude-standard \
    > "$evidence/framework-untracked-files.txt"
"$container_tool_bin" image inspect "$UPGRADE_BASELINE_IMAGE" > "$evidence/baseline-image.json"
"$container_tool_bin" image inspect "$IMG" > "$evidence/framework-image.json"
kubectl_cmd version -o yaml > "$evidence/kubectl-version.yaml"
kind_cmd version > "$evidence/kind-version.txt"
"$container_tool_bin" version > "$evidence/container-tool-version.txt"
sha256sum "$UPGRADE_COMPARATOR" "$UPGRADE_FIXTURE_DIR/postgres.yaml" \
    "$UPGRADE_FIXTURE_DIR/superset.yaml" "$UPGRADE_INTENTIONAL_DIFFS" \
    > "$evidence/acceptance-assets.sha256"

# Render both operator manifests before the long test so neither source checkout is
# consulted after the first cluster mutation.
render_operator "$UPGRADE_BASELINE_DIR" "$UPGRADE_BASELINE_IMAGE" \
    "$evidence/baseline-operator.yaml"
render_operator "$UPGRADE_WORKTREE_DIR" "$IMG" "$evidence/framework-operator.yaml"
sed -e "s/__PRODUCT_VERSION__/$product_version/g" \
    -e "s/__KUBEDOOP_VERSION__/$kubedoop_version/g" \
    "$UPGRADE_FIXTURE_DIR/superset.yaml" > "$evidence/input-cluster.yaml"

kind_cmd load docker-image --name "$CHAINSAW_CLUSTER" "$UPGRADE_BASELINE_IMAGE" "$IMG"

echo "Phase 1/3: fixed Gen 2 baseline $baseline_head"
deploy_operator "$evidence/baseline-operator.yaml"
kubectl_cmd create namespace "$namespace"
k apply -f "$UPGRADE_FIXTURE_DIR/postgres.yaml"
wait_for 'PostgreSQL StatefulSet ready' statefulset_ready upgrade-postgres
wait_for 'PostgreSQL accepts SQL' database_ready
k wait --for=condition=Ready pod/upgrade-client --timeout=300s
k exec upgrade-client -- python -c 'import requests'
k apply -f "$evidence/input-cluster.yaml"
for group in primary secondary; do
    wait_for "baseline $group StatefulSet ready" statefulset_ready "upgrade-node-$group"
done
wait_for 'baseline health and database-backed login on both groups' probe_api_once before
wait_for 'baseline rolling application logs on both groups' application_logs_ready
wait_for 'baseline role PDB covers both ready groups' pdb_ready
db_query 'CREATE TABLE IF NOT EXISTS kubedoop_framework_upgrade_marker (phase text PRIMARY KEY, created_at timestamptz NOT NULL DEFAULT now())' \
    >/dev/null
mark_phase before-upgrade
snapshot before

echo "Phase 2/3: Gen 3 framework $framework_head"
replace_superset_workloads 'baseline -> framework'
deploy_operator "$evidence/framework-operator.yaml"
for group in primary secondary; do
    wait_for "framework $group StatefulSet ready" statefulset_ready "upgrade-node-$group"
done
wait_for 'framework health and database-backed login on both groups' probe_api_once after
wait_for 'framework rolling application logs on both groups' application_logs_ready
wait_for 'framework role PDB covers both ready groups' pdb_ready
mark_phase after-upgrade
snapshot after

echo "Phase 3/3: rollback to fixed Gen 2 resource shape"
replace_superset_workloads 'framework -> baseline rollback'
for group in primary secondary; do
    k delete svc "upgrade-node-$group-headless" --ignore-not-found
done
k delete sa supersetcluster-upgrade --ignore-not-found
# GenericReconciler adopts the role PDB with a controller owner reference. The
# legacy reconciler preserves foreign metadata, so remove only that Gen 3
# ownership marker before rollback; the PDB object and UID stay intact.
k patch pdb upgrade-node --type=merge -p '{"metadata":{"ownerReferences":[]}}'
# Keep the expanded CRD. The old deployment/RBAC can run against the compatible
# stored CR while avoiding an unsafe schema downgrade after Gen 3 status writes.
python3 - "$evidence/baseline-operator.yaml" "$scratch/rollback-operator.yaml" <<'PY'
import pathlib
import sys

source = pathlib.Path(sys.argv[1]).read_text()
documents = source.split("\n---\n")
kept = []
for document in documents:
    lines = {line.strip() for line in document.splitlines()}
    if "kind: CustomResourceDefinition" not in lines:
        kept.append(document)
pathlib.Path(sys.argv[2]).write_text("\n---\n".join(kept))
PY
deploy_operator "$scratch/rollback-operator.yaml"
for group in primary secondary; do
    wait_for "rollback $group StatefulSet ready" statefulset_ready "upgrade-node-$group"
done
wait_for 'rollback health and database-backed login on both groups' probe_api_once rollback
wait_for 'rollback rolling application logs on both groups' application_logs_ready
wait_for 'rollback role PDB covers both ready groups' pdb_ready
mark_phase after-rollback
snapshot rollback

python3 "$UPGRADE_COMPARATOR" "$evidence"
kubectl_cmd delete namespace "$namespace" --wait=true --timeout=300s
deploy_operator "$evidence/framework-operator.yaml"
{
    printf 'PASS: %s -> %s -> %s\n' "$baseline_head" "$framework_head" "$baseline_head"
    printf 'Superset %s health/login and PostgreSQL identity/markers survived all phases\n' \
        "$product_version"
    printf 'The disposable kind cluster is retained; run make cleanup-framework-upgrade-e2e\n'
} | tee "$evidence/result.txt"

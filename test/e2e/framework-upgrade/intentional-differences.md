# Gen 2 to Gen 3 acceptance differences

This fixture compares a fixed pre-framework `main` revision with the current
worktree on the same disposable Kubernetes 1.35 kind cluster. Kubernetes 1.35
matches the current CI matrix and supports the native-sidecar form emitted by
Gen 3. The following differences are intentional and are asserted by
`hack/compare-upgrade-resources.py` rather than hidden from the saved normalized
diff.

- The two Superset StatefulSets are deliberately drained, deleted, and rebuilt
  at each transition. On upgrade, operator-go v0.13 deliberately preserves a
  live StatefulSet's Gen 2 selector and `serviceName`, but its desired Gen 3 pod
  template replaces the old identity labels. That template no longer satisfies
  the preserved selector (`managed-by` conflicts and `name` disappears), so the
  Kubernetes API rejects the update. On rollback, the Gen 2 reconciler directly
  updates the Gen 3 StatefulSet and encounters its immutable selector and
  `serviceName`. The script therefore first scales the running controller to
  zero, waits for it to stop, and only then removes the workloads before the
  other controller starts. This is the required manual immutable-field
  migration. Superset state lives in the external PostgreSQL database; this is
  a maintenance-window migration, not a zero-downtime claim.
- Gen 3 adds `upgrade-node-{primary,secondary}-headless` Services and points the
  rebuilt StatefulSets at them.
- Gen 3 replaces the immutable Gen 2 selector (which included
  `app.kubernetes.io/name=supersetcluster`, the conventional role-group label,
  and `managed-by=superset.kubedoop.dev`) with the framework identity subset:
  instance, component, `managed-by=operator-go`, and the per-role-group marker
  (`upgrade-primary=true` or `upgrade-secondary=true`).
- Gen 3 creates the per-cluster workload ServiceAccount
  `supersetcluster-upgrade` and puts it on both pod templates.
- Gen 3 adopts the role PDB with a controller owner reference. The rollback
  maintenance procedure removes only that ownership marker before the Gen 2
  controller resumes, preserving the PDB UID and restoring its original shape.
- Superset keeps its Gen 2 `OrderedReady` pod management policy explicitly;
  operator-go's `Parallel` default is intended for quorum products and is not an
  approved migration difference here.
- Gen 3 applies the canonical security contexts (uid 1001, gid 0, fsGroup 1001,
  RuntimeDefault seccomp, no privilege escalation, and dropped capabilities).
- Superset explicitly preserves the effective Gen 2 `enableServiceLinks: true`
  default. A user may still set it to `false` through `podOverrides`; changing
  the default during this refactor is not an approved compatibility break.
- Gen 3 changes the startup command to copy projected configuration with `cp -RL`,
  so nested overrides and ConfigMap symlinks are followed. Startup, readiness,
  and liveness probes are not an approved difference: the comparator requires
  startup to remain absent and readiness/liveness to retain their exact Gen 2
  `/health` behavior on port `8088` with `30/10/5/3/1` timing (initial delay,
  period, timeout, failure threshold, success threshold).
- The always-on `metrics` process moves from an ordinary container to a
  Kubernetes native sidecar (`initContainers[*].restartPolicy=Always`). The
  comparator asserts both the Gen 2 and Gen 3 shapes, the unchanged image,
  command, arguments and resources, and the rollback restoration.
- The old default pod anti-affinity is not carried forward. It selected
  `app.kubernetes.io/name=hbase`, a copy/paste value that never selected these
  Superset pods.
- Superset's compatibility layer uses operator-go's Python logging generator
  for `log_config.py`, and `superset_config.py` invokes that dictConfig. The
  bytes differ, but the Gen 2 default file-logging contract remains: one 30Mi
  `log` emptyDir, a read-write `/kubedoop/log/` mount on `node`, a rotating JSON
  file at `/kubedoop/log/superset/superset.py.json`, a 1MiB file limit and one
  backup. The script verifies that file is non-empty for both role groups in
  every phase. Health plus database-backed login is also checked in every
  phase. The old empty `vector.yaml` may disappear when no Vector aggregator is
  configured.
- A configured Vector pipeline uses the framework's native sidecar form. This
  minimal external-database fixture does not install a Vector aggregator, so
  that shape remains covered by the normal logging E2E suite rather than this
  migration run. Existing CRs that only set
  `vectorAggregatorConfigMapName` retain the Gen 2 implicit-enable behavior;
  an explicit role or role-group `logging.enableVectorAgent: false` wins.

The `external-unstable` listener contract is asserted separately from the broad
resource diff: both client Services must remain the same `NodePort` objects with
the same UIDs, cluster IPs, allocated node ports, named ports, and target ports.
The Gen 3 headless Services must be `ClusterIP: None`, use the new workload
selectors, and expose the same named ports. Any Listener custom resources in the
workload namespace are also snapshotted and must retain the same desired shape in
all three phases (Superset currently emits none for this Service-based listener).

Before rollback, the script removes only the framework-only headless Services
and workload ServiceAccount. Superset declares no workload RBAC rules, so the
framework intentionally creates no namespaced Role or RoleBinding. The script
does not downgrade the CRD. The old operator must then restore an exact
normalized `sts,cm,svc,pdb,sa` baseline. Any residual rollback diff fails the
acceptance.

The acceptance also snapshots the operator namespace, all operator-owned
ClusterRoles/ClusterRoleBindings, and the SupersetCluster CRD. The operator
namespace may change only the manager image; the cluster-scoped manifest may
change only the manager ClusterRole rules. Rollback must restore both baseline
forms. The expanded Gen 3 CRD must remain served and stored as `v1alpha1` through
rollback rather than being downgraded.

The normalized views remove Kubernetes-assigned timestamps, UIDs, resource
versions, generations, managed fields, deployment revision annotations,
legacy `banzaicloud.com/last-applied` payloads, owner-reference UIDs, Service
cluster IPs, health-check node ports, and allocated node ports. The after
comparison permits changes only in the explicitly reviewed
StatefulSet pod-template/identity seams, ConfigMap data/labels, client Service
identity labels/selectors, and role PDB labels/selectors/ownership. Everything outside
those seams must remain byte-equivalent after normalization; the allowed seams
then have semantic assertions for image, probes, ports, security, sidecar shape,
application logging, `enableServiceLinks`, selectors, readiness, and runtime
login. Identity is checked
separately: the SupersetCluster, credentials Secret digest/UID, PostgreSQL PVC
UID plus PV name/UID/claim binding, and client Service UIDs must survive all
three phases. Superset StatefulSet and Pod UIDs are intentionally not stable
because the migration rebuilds those workloads.

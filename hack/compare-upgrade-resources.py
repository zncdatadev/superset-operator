#!/usr/bin/env python3
"""Validate Superset migration contracts and save normalized resource diffs."""

import copy
import difflib
import hashlib
import json
import pathlib
import sys


PHASES = ("before", "after", "rollback")
GROUPS = ("primary", "secondary")
CLUSTER_NAME = "upgrade"
ROLE_NAME = "node"
WORKLOAD_SA = "supersetcluster-upgrade"
OPERATOR_DEPLOYMENT = "superset-operator-controller-manager"
OPERATOR_MANAGER_ROLE = "superset-operator-manager-role"
SUPERSET_CRD = "supersetclusters.superset.kubedoop.dev"


def load_json(path):
    return json.loads(path.read_text())


def resource_collection(root, phase, suffix="resources"):
    value = load_json(root / f"{phase}-{suffix}.json")
    items = value.get("items", [value])
    return {(item["kind"], item["metadata"]["name"]): item for item in items}


def resources(root, phase):
    return resource_collection(root, phase)


def normalized_resource(value):
    value = copy.deepcopy(value)
    value.pop("status", None)
    metadata = value["metadata"]
    for key in (
        "uid",
        "resourceVersion",
        "generation",
        "creationTimestamp",
        "managedFields",
    ):
        metadata.pop(key, None)
    annotations = metadata.get("annotations", {})
    annotations.pop("kubectl.kubernetes.io/last-applied-configuration", None)
    annotations.pop("deployment.kubernetes.io/revision", None)
    if not annotations:
        metadata.pop("annotations", None)
    for owner in metadata.get("ownerReferences", []):
        owner.pop("uid", None)
    if value["kind"] == "Service":
        for key in ("clusterIP", "clusterIPs", "healthCheckNodePort"):
            value["spec"].pop(key, None)
        for port in value["spec"].get("ports", []):
            port.pop("nodePort", None)
    return value


def normalized_list(items):
    return [normalized_resource(items[key]) for key in sorted(items)]


def canonical_items(items, transform=None):
    values = {}
    for key, item in items.items():
        value = normalized_resource(item)
        if transform is not None:
            transform(key, value)
        values[key] = value
    return json.dumps([values[key] for key in sorted(values)], indent=2, sort_keys=True) + "\n"


def remove_path(value, path):
    parent = value
    for key in path[:-1]:
        parent = parent.get(key)
        if not isinstance(parent, dict):
            return
    if isinstance(parent, dict):
        parent.pop(path[-1], None)


def assert_only_approved_after_fields(root, before, after):
    """Fail when a common resource changes outside the reviewed Gen 3 seams."""
    allowed = {}
    for group in GROUPS:
        name = f"{CLUSTER_NAME}-{ROLE_NAME}-{group}"
        allowed[("StatefulSet", name)] = (
            ("metadata", "labels"),
            ("spec", "serviceName"),
            ("spec", "selector"),
            ("spec", "template"),
        )
        allowed[("ConfigMap", name)] = (
            ("metadata", "labels"),
            ("data",),
        )
        allowed[("Service", name)] = (
            ("metadata", "labels"),
            ("spec", "selector"),
        )
    allowed[("PodDisruptionBudget", f"{CLUSTER_NAME}-{ROLE_NAME}")] = (
        ("metadata", "labels"),
        ("spec", "selector"),
    )

    common_keys = set(before) & set(after)
    scrubbed = {}
    for phase, source in (("before", before), ("after", after)):
        phase_items = {}
        for key in common_keys:
            value = normalized_resource(source[key])
            for path in allowed.get(key, ()):
                remove_path(value, path)
            phase_items[key] = value
        scrubbed[phase] = json.dumps(
            [phase_items[key] for key in sorted(phase_items)],
            indent=2,
            sort_keys=True,
        ) + "\n"

    write_diff(root, "after-unapproved-residual", scrubbed["before"], scrubbed["after"])
    (root / "after-allowed-paths.json").write_text(
        json.dumps(
            {
                f"{kind}/{name}": [".".join(path) for path in paths]
                for (kind, name), paths in sorted(allowed.items())
            },
            indent=2,
            sort_keys=True,
        ) + "\n"
    )
    assert scrubbed["before"] == scrubbed["after"], (
        "framework changed fields outside the explicit after-change seams; "
        "inspect after-unapproved-residual.diff"
    )


def write_diff(root, phase, before_text, phase_text, from_phase="before"):
    diff = difflib.unified_diff(
        before_text.splitlines(True),
        phase_text.splitlines(True),
        fromfile=from_phase,
        tofile=phase,
    )
    (root / f"{phase}.diff").write_text("".join(diff))


def get(items, kind, name):
    key = (kind, name)
    assert key in items, f"missing {kind}/{name}"
    return items[key]


def uid(item):
    value = item["metadata"].get("uid")
    assert value, f"{item['kind']}/{item['metadata']['name']} has no UID"
    return value


def service_identity(service):
    spec = service["spec"]
    ports = []
    for port in spec.get("ports", []):
        ports.append(
            {
                key: port.get(key)
                for key in ("name", "protocol", "port", "targetPort", "nodePort")
            }
        )
    return {
        "uid": uid(service),
        "clusterIP": spec.get("clusterIP"),
        "clusterIPs": spec.get("clusterIPs"),
        "ports": sorted(ports, key=lambda value: value.get("name") or ""),
    }


def service_ports(service):
    return sorted(
        [
            {
                key: port.get(key)
                for key in ("name", "protocol", "port", "targetPort")
            }
            for port in service["spec"].get("ports", [])
        ],
        key=lambda value: value.get("name") or "",
    )


def main_container(statefulset):
    containers = {
        container["name"]: container
        for container in statefulset["spec"]["template"]["spec"].get("containers", [])
    }
    container = containers.get(ROLE_NAME)
    assert container is not None, (
        statefulset["metadata"]["name"],
        "missing main container",
        sorted(containers),
    )
    return container


def assert_probe_transition(old_sts, new_sts, restored_sts, name):
    old = main_container(old_sts)
    new = main_container(new_sts)
    restored = main_container(restored_sts)
    assert new.get("image") == old.get("image") == restored.get("image"), (
        name,
        "product image changed across the operator-only migration",
        old.get("image"),
        new.get("image"),
        restored.get("image"),
    )
    assert new.get("env", []) == old.get("env", []) == restored.get("env", []), (
        name,
        "main-container environment changed across the operator-only migration",
    )
    for probe_name in ("startupProbe", "livenessProbe", "readinessProbe"):
        assert new.get(probe_name) == old.get(probe_name) == restored.get(probe_name), (
            name,
            "probe shape changed across the operator-only migration",
            probe_name,
            old.get(probe_name),
            new.get(probe_name),
            restored.get(probe_name),
        )
        probe = new.get(probe_name, {})
        assert probe.get("httpGet", {}).get("path") == "/health", (
            name,
            "framework health probe path",
            probe_name,
            probe,
        )
        assert probe.get("httpGet", {}).get("scheme") == "HTTP", (
            name,
            "framework health probe scheme",
            probe_name,
            probe,
        )
        assert not any(handler in probe for handler in ("exec", "tcpSocket", "grpc")), (
            name,
            "framework health probe unexpectedly has another handler",
            probe_name,
            probe,
        )
        expected_port = "http" if probe_name == "startupProbe" else 8088
        assert probe.get("httpGet", {}).get("port") == expected_port, (
            name,
            "framework health probe port",
            probe_name,
            probe,
        )
        expected_timing = (
            {
                "initialDelaySeconds": 4,
                "periodSeconds": 6,
                "timeoutSeconds": 3,
                "failureThreshold": 30,
                "successThreshold": 1,
            }
            if probe_name == "startupProbe"
            else {
                "initialDelaySeconds": 30,
                "periodSeconds": 10,
                "timeoutSeconds": 5,
                "failureThreshold": 3,
                "successThreshold": 1,
            }
        )
        actual_timing = {key: probe.get(key) for key in expected_timing}
        assert actual_timing == expected_timing, (
            name,
            "framework health probe timing",
            probe_name,
            actual_timing,
        )

    command = "\n".join(new.get("command", []))
    assert "cp -RL /kubedoop/mount/config/* /kubedoop/app/pythonpath" in command, (
        name,
        "framework config copy no longer follows ConfigMap symlinks recursively",
    )
    assert restored.get("command") == old.get("command"), (name, "rollback command")


def expected_role_selector(statefulsets):
    selectors = [item["spec"]["selector"]["matchLabels"] for item in statefulsets]
    expected = {
        key: value
        for key, value in selectors[0].items()
        if all(selector.get(key) == value for selector in selectors[1:])
    }
    assert expected, ("empty common role selector", selectors)
    return expected


def assert_pdb_contract(pdb, statefulsets, phase):
    selector = pdb["spec"].get("selector", {})
    assert not selector.get("matchExpressions"), (phase, "unexpected PDB expressions", selector)
    assert selector.get("matchLabels") == expected_role_selector(statefulsets), (
        phase,
        "PDB selector is not the exact common role identity",
        selector,
        [item["spec"]["selector"] for item in statefulsets],
    )
    status = pdb.get("status", {})
    assert status.get("observedGeneration", 0) >= pdb["metadata"]["generation"], (
        phase,
        "PDB generation not observed",
        status,
    )
    assert {
        key: status.get(key)
        for key in ("expectedPods", "currentHealthy", "disruptionsAllowed")
    } == {
        "expectedPods": 2,
        "currentHealthy": 2,
        "disruptionsAllowed": 1,
    }, (phase, "PDB does not cover both ready role groups", status)


def run_metadata(root):
    return {
        key: value
        for key, value in (
            line.split("=", 1)
            for line in (root / "run-metadata.txt").read_text().splitlines()
            if "=" in line
        )
    }


def manager_image(items):
    deployment = get(items, "Deployment", OPERATOR_DEPLOYMENT)
    containers = deployment["spec"]["template"]["spec"].get("containers", [])
    manager = next(container for container in containers if container["name"] == "manager")
    return manager["image"]


def mask_manager_image(key, value):
    if key != ("Deployment", OPERATOR_DEPLOYMENT):
        return
    containers = value["spec"]["template"]["spec"].get("containers", [])
    manager = next(container for container in containers if container["name"] == "manager")
    manager["image"] = "<operator-image>"


def mask_manager_role_rules(key, value):
    if key == ("ClusterRole", OPERATOR_MANAGER_ROLE):
        value["rules"] = ["<version-specific-manager-rules>"]


def verbs_for(role, api_group, resource):
    verbs = set()
    for rule in role.get("rules", []):
        if api_group in rule.get("apiGroups", []) and resource in rule.get("resources", []):
            verbs.update(rule.get("verbs", []))
    return verbs


def assert_operator_contract(root):
    namespaced = {
        phase: resource_collection(root, phase, "operator-namespaced")
        for phase in PHASES
    }
    cluster = {
        phase: resource_collection(root, phase, "operator-cluster")
        for phase in PHASES
    }
    metadata = run_metadata(root)

    for label, collections in (("operator-namespaced", namespaced), ("operator-cluster", cluster)):
        actual_views = {
            phase: canonical_items(items) for phase, items in collections.items()
        }
        for phase, value in actual_views.items():
            (root / f"{phase}-{label}-normalized.json").write_text(value)
        write_diff(root, f"{label}-after", actual_views["before"], actual_views["after"])
        write_diff(
            root,
            f"{label}-rollback",
            actual_views["before"],
            actual_views["rollback"],
        )

    assert all(set(items) == set(namespaced["before"]) for items in namespaced.values()), (
        "operator namespaced resource set changed",
        {phase: sorted(items) for phase, items in namespaced.items()},
    )
    assert all(set(items) == set(cluster["before"]) for items in cluster.values()), (
        "operator cluster resource set changed",
        {phase: sorted(items) for phase, items in cluster.items()},
    )
    assert manager_image(namespaced["before"]) == metadata["baseline_image"]
    assert manager_image(namespaced["after"]) == metadata["framework_image"]
    assert manager_image(namespaced["rollback"]) == metadata["baseline_image"]

    namespaced_views = {
        phase: canonical_items(items, mask_manager_image)
        for phase, items in namespaced.items()
    }
    assert len(set(namespaced_views.values())) == 1, (
        "operator namespaced resources changed beyond the selected image"
    )

    cluster_views = {
        phase: canonical_items(items, mask_manager_role_rules)
        for phase, items in cluster.items()
    }
    assert len(set(cluster_views.values())) == 1, (
        "operator cluster resources changed outside the manager ClusterRole"
    )
    before_role = get(cluster["before"], "ClusterRole", OPERATOR_MANAGER_ROLE)
    after_role = get(cluster["after"], "ClusterRole", OPERATOR_MANAGER_ROLE)
    rollback_role = get(cluster["rollback"], "ClusterRole", OPERATOR_MANAGER_ROLE)
    assert normalized_resource(before_role) == normalized_resource(rollback_role), (
        "rollback did not restore the baseline manager ClusterRole"
    )
    assert normalized_resource(before_role) != normalized_resource(after_role), (
        "framework manager ClusterRole did not change as expected"
    )
    assert verbs_for(after_role, "", "secrets") == {"get", "list", "watch"}
    assert verbs_for(after_role, "", "serviceaccounts") == {
        "create", "get", "list", "patch", "update", "watch"
    }
    assert verbs_for(after_role, "superset.kubedoop.dev", "supersetclusters") == {
        "get", "list", "watch"
    }

    crds = {phase: load_json(root / f"{phase}-crd.json") for phase in PHASES}
    before_crd = normalized_resource(crds["before"])
    after_crd = normalized_resource(crds["after"])
    rollback_crd = normalized_resource(crds["rollback"])
    assert before_crd != after_crd, "framework CRD did not contain the reviewed schema expansion"
    assert after_crd == rollback_crd, "rollback unexpectedly downgraded the framework CRD"
    write_diff(
        root,
        "crd-after",
        json.dumps(before_crd, indent=2, sort_keys=True) + "\n",
        json.dumps(after_crd, indent=2, sort_keys=True) + "\n",
    )
    write_diff(
        root,
        "crd-rollback",
        json.dumps(after_crd, indent=2, sort_keys=True) + "\n",
        json.dumps(rollback_crd, indent=2, sort_keys=True) + "\n",
        from_phase="after",
    )
    for phase, crd in crds.items():
        assert crd["metadata"]["name"] == SUPERSET_CRD
        versions = {version["name"]: version for version in crd["spec"]["versions"]}
        assert set(versions) == {"v1alpha1"}, (phase, "unexpected CRD versions", versions)
        assert versions["v1alpha1"].get("served") is True
        assert versions["v1alpha1"].get("storage") is True
        assert crd.get("status", {}).get("storedVersions") == ["v1alpha1"], (
            phase,
            "unexpected stored CRD versions",
            crd.get("status", {}),
        )

    listeners = {
        phase: resource_collection(root, phase, "listeners")
        for phase in PHASES
    }
    listener_views = {phase: canonical_items(items) for phase, items in listeners.items()}
    for phase, value in listener_views.items():
        (root / f"{phase}-listeners-normalized.json").write_text(value)
    write_diff(
        root,
        "listeners-after",
        listener_views["before"],
        listener_views["after"],
    )
    write_diff(
        root,
        "listeners-rollback",
        listener_views["before"],
        listener_views["rollback"],
    )
    assert len(set(listener_views.values())) == 1, (
        "Listener desired resources changed across the operator migration",
        {phase: sorted(items) for phase, items in listeners.items()},
    )


def assert_selector_matches_template(spec, name):
    selector = spec["selector"]["matchLabels"]
    labels = spec["template"]["metadata"]["labels"]
    assert all(labels.get(key) == value for key, value in selector.items()), (
        name,
        "pod template no longer satisfies immutable selector",
        selector,
        labels,
    )


def assert_framework_security(spec, name):
    pod_security = spec["template"]["spec"].get("securityContext", {})
    expected_pod = {
        "runAsUser": 1001,
        "runAsGroup": 0,
        "runAsNonRoot": True,
        "fsGroup": 1001,
        "fsGroupChangePolicy": "OnRootMismatch",
    }
    for key, value in expected_pod.items():
        assert pod_security.get(key) == value, (name, "pod security", key, pod_security)
    assert pod_security.get("seccompProfile", {}).get("type") == "RuntimeDefault", (
        name,
        "pod seccomp",
        pod_security,
    )

    containers = {
        container["name"]: container
        for container in spec["template"]["spec"].get("containers", [])
    }
    main = containers.get(ROLE_NAME)
    assert main is not None, (name, "missing main container", sorted(containers))
    security = main.get("securityContext", {})
    expected_main = {
        "runAsUser": 1001,
        "runAsGroup": 0,
        "runAsNonRoot": True,
        "allowPrivilegeEscalation": False,
    }
    for key, value in expected_main.items():
        assert security.get(key) == value, (name, "container security", key, security)
    assert security.get("seccompProfile", {}).get("type") == "RuntimeDefault", (
        name,
        "container seccomp",
        security,
    )
    assert security.get("capabilities", {}).get("drop") == ["ALL"], (
        name,
        "container capabilities",
        security,
    )


def container_names(pod_spec, field):
    return {container["name"] for container in pod_spec.get(field, [])}


def assert_metrics_sidecar_transition(old, new, restored, name):
    old_pod = old["template"]["spec"]
    new_pod = new["template"]["spec"]
    restored_pod = restored["template"]["spec"]
    assert container_names(old_pod, "containers") == {ROLE_NAME, "metrics"}
    assert container_names(old_pod, "initContainers") == set()
    assert container_names(new_pod, "containers") == {ROLE_NAME}
    assert container_names(new_pod, "initContainers") == {"metrics"}
    native_metrics = next(
        container
        for container in new_pod.get("initContainers", [])
        if container["name"] == "metrics"
    )
    assert native_metrics.get("restartPolicy") == "Always", (
        name,
        "metrics is not a Kubernetes native sidecar",
        native_metrics,
    )
    assert {
        (port.get("name"), port.get("containerPort"), port.get("protocol"))
        for port in native_metrics.get("ports", [])
    } == {
        ("metrics", 9102, "TCP"),
        ("statsd", 9125, "UDP"),
    }, (name, "native metrics sidecar ports", native_metrics.get("ports", []))
    assert container_names(restored_pod, "containers") == {ROLE_NAME, "metrics"}
    assert container_names(restored_pod, "initContainers") == set()
    old_main = next(container for container in old_pod["containers"] if container["name"] == ROLE_NAME)
    new_main = next(container for container in new_pod["containers"] if container["name"] == ROLE_NAME)
    restored_main = next(
        container for container in restored_pod["containers"] if container["name"] == ROLE_NAME
    )
    old_main_ports = {
        (port.get("name"), port.get("containerPort"), port.get("protocol"))
        for port in old_main.get("ports", [])
    }
    new_main_ports = {
        (port.get("name"), port.get("containerPort"), port.get("protocol"))
        for port in new_main.get("ports", [])
    }
    restored_main_ports = {
        (port.get("name"), port.get("containerPort"), port.get("protocol"))
        for port in restored_main.get("ports", [])
    }
    assert old_main_ports == restored_main_ports == {
        ("http", 8088, "TCP"),
        ("metrics", 9102, "TCP"),
    }, (name, "Gen 2 main-container ports", old_main_ports, restored_main_ports)
    assert new_main_ports == {("http", 8088, "TCP")}, (
        name,
        "framework main-container ports",
        new_main_ports,
    )
    assert old_pod.get("affinity"), (name, "Gen 2 anti-affinity fixture missing")
    assert not new_pod.get("affinity"), (name, "broken Gen 2 anti-affinity retained")
    assert restored_pod.get("affinity") == old_pod.get("affinity"), (
        name,
        "rollback anti-affinity",
    )


def assert_api(root, phase):
    records = [
        json.loads(line)
        for line in (root / f"{phase}-api.jsonl").read_text().splitlines()
        if line.strip()
    ]
    assert {record["group"] for record in records} == set(GROUPS), (phase, records)
    for record in records:
        assert record["health_status"] == 200, (phase, record)
        assert record["login_status"] == 200, (phase, record)
        assert record["access_token"] is True, (phase, record)


def secret_identity(root, phase):
    value = load_json(root / f"{phase}-credentials.json")
    assert value["name"] == "upgrade-credentials"
    assert value["uid"] and value["data_sha256"]
    return value


def postgres_storage_identity(root, phase):
    items = load_json(root / f"{phase}-pvcs.json")["items"]
    pvc = next(
        item for item in items if item["metadata"]["name"] == "data-upgrade-postgres-0"
    )
    pv = load_json(root / f"{phase}-postgres-pv.json")
    claim_ref = pv["spec"].get("claimRef", {})
    assert pv["metadata"]["name"] == pvc["spec"].get("volumeName")
    assert claim_ref.get("name") == pvc["metadata"]["name"]
    assert claim_ref.get("namespace") == "superset-framework-upgrade"
    assert claim_ref.get("uid") == pvc["metadata"]["uid"]
    return {
        "pvc_name": pvc["metadata"]["name"],
        "pvc_uid": pvc["metadata"]["uid"],
        "pv_name": pvc["spec"].get("volumeName"),
        "pv_uid": pv["metadata"]["uid"],
        "storage_class_name": pvc["spec"].get("storageClassName"),
        "claim_ref_name": claim_ref.get("name"),
        "claim_ref_namespace": claim_ref.get("namespace"),
        "claim_ref_uid": claim_ref.get("uid"),
    }


def text(root, phase, suffix):
    return (root / f"{phase}-{suffix}").read_text().strip()


def main(root):
    snapshots = {phase: resources(root, phase) for phase in PHASES}
    before = snapshots["before"]
    after = snapshots["after"]
    rollback = snapshots["rollback"]

    # Materialize all normalized views and diffs before enforcing contracts so a
    # failed assertion still leaves reviewable evidence.
    normalized = {}
    for phase in PHASES:
        value = json.dumps(
            normalized_list(snapshots[phase]), indent=2, sort_keys=True
        ) + "\n"
        normalized[phase] = value
        (root / f"{phase}-normalized.json").write_text(value)
    write_diff(root, "after", normalized["before"], normalized["after"])
    write_diff(root, "rollback", normalized["before"], normalized["rollback"])

    expected_additions = {
        ("Service", f"{CLUSTER_NAME}-{ROLE_NAME}-{group}-headless")
        for group in GROUPS
    }
    expected_additions.add(("ServiceAccount", WORKLOAD_SA))
    assert set(after) - set(before) == expected_additions, (
        "unexpected framework resource additions",
        sorted(set(after) - set(before)),
    )
    assert set(before) - set(after) == set(), (
        "framework removed baseline resources",
        sorted(set(before) - set(after)),
    )
    assert set(rollback) == set(before), (
        "rollback did not restore the baseline resource set",
        sorted(set(rollback) - set(before)),
        sorted(set(before) - set(rollback)),
    )
    assert_only_approved_after_fields(root, before, after)

    for group in GROUPS:
        name = f"{CLUSTER_NAME}-{ROLE_NAME}-{group}"
        old_sts = get(before, "StatefulSet", name)
        new_sts = get(after, "StatefulSet", name)
        restored_sts = get(rollback, "StatefulSet", name)
        old, new, restored = (
            item["spec"] for item in (old_sts, new_sts, restored_sts)
        )

        assert old["serviceName"] == name, (name, "unexpected Gen 2 serviceName")
        assert new["serviceName"] == name + "-headless", (
            name,
            "Gen 3 headless serviceName missing",
        )
        assert restored["serviceName"] == old["serviceName"], (
            name,
            "rollback serviceName",
        )
        assert restored["selector"] == old["selector"], (name, "rollback selector")
        assert restored.get("volumeClaimTemplates", []) == old.get(
            "volumeClaimTemplates", []
        ), (name, "rollback volumeClaimTemplates")
        assert restored.get("podManagementPolicy") == old.get("podManagementPolicy"), (
            name,
            "rollback podManagementPolicy",
        )

        old_selector = old["selector"]["matchLabels"]
        new_selector = new["selector"]["matchLabels"]
        assert old_selector == {
            "app.kubernetes.io/name": "supersetcluster",
            "app.kubernetes.io/instance": CLUSTER_NAME,
            "app.kubernetes.io/component": ROLE_NAME,
            "app.kubernetes.io/role-group": group,
            "app.kubernetes.io/managed-by": "superset.kubedoop.dev",
        }, (name, "unexpected Gen 2 selector", old_selector)
        assert new_selector == {
            "app.kubernetes.io/instance": CLUSTER_NAME,
            "app.kubernetes.io/component": ROLE_NAME,
            "app.kubernetes.io/managed-by": "operator-go",
            f"{CLUSTER_NAME}-{group}": "true",
        }, (name, "unexpected Gen 3 selector", new_selector)
        assert_selector_matches_template(new, name)
        assert_selector_matches_template(restored, name)
        assert_metrics_sidecar_transition(old, new, restored, name)
        assert_probe_transition(old_sts, new_sts, restored_sts, name)
        assert new["template"]["spec"].get("serviceAccountName") == WORKLOAD_SA, (
            name,
            "workload ServiceAccount",
        )
        assert_framework_security(new, name)

        # Rebuilding is deliberate at both transitions; the database, not these pods,
        # carries Superset state.
        assert len({uid(old_sts), uid(new_sts), uid(restored_sts)}) == 3, (
            name,
            "StatefulSet was not rebuilt at both immutable-shape transitions",
        )

        old_service = get(before, "Service", name)
        new_service = get(after, "Service", name)
        restored_service = get(rollback, "Service", name)
        assert old_service["spec"].get("selector") == old_selector
        assert new_service["spec"].get("selector") == new_selector
        assert restored_service["spec"].get("selector") == old_selector
        assert all(
            service["spec"].get("type") == "NodePort"
            for service in (old_service, new_service, restored_service)
        ), (name, "external-unstable listener is not backed by NodePort")
        assert service_identity(old_service) == service_identity(new_service) == service_identity(
            restored_service
        ), (name, "client Service identity changed")

        headless = get(after, "Service", name + "-headless")
        assert headless["spec"].get("selector") == new_selector
        assert headless["spec"].get("clusterIP") == "None", (
            name,
            "headless Service has a cluster IP",
        )
        assert headless["spec"].get("type") == "ClusterIP", (
            name,
            "headless Service type",
            headless["spec"],
        )
        assert service_ports(headless) == service_ports(new_service), (
            name,
            "headless/client listener ports diverged",
            service_ports(headless),
            service_ports(new_service),
        )

        old_cm = get(before, "ConfigMap", name)
        new_cm = get(after, "ConfigMap", name)
        restored_cm = get(rollback, "ConfigMap", name)
        assert uid(old_cm) == uid(new_cm) == uid(restored_cm), (
            name,
            "ConfigMap identity changed",
        )
        for config in (old_cm, new_cm, restored_cm):
            assert {"log_config.py", "superset_config.py"}.issubset(config["data"]), (
                name,
                "public config filenames changed",
                sorted(config["data"]),
            )
        if "vector.yaml" not in new_cm["data"]:
            assert old_cm["data"].get("vector.yaml", "") == "", (
                name,
                "non-empty vector config disappeared",
            )

    workload_sa = get(after, "ServiceAccount", WORKLOAD_SA)
    owners = workload_sa["metadata"].get("ownerReferences", [])
    assert any(
        owner.get("kind") == "SupersetCluster" and owner.get("name") == CLUSTER_NAME
        for owner in owners
    ), ("workload ServiceAccount owner", owners)

    pdb_name = f"{CLUSTER_NAME}-{ROLE_NAME}"
    pdbs = [get(snapshots[phase], "PodDisruptionBudget", pdb_name) for phase in PHASES]
    assert len({uid(item) for item in pdbs}) == 1, "role PDB identity changed"
    assert all(item["spec"].get("maxUnavailable") == 1 for item in pdbs), (
        "role PDB maxUnavailable",
        [item["spec"] for item in pdbs],
    )
    for phase, pdb in zip(PHASES, pdbs):
        statefulsets = [
            get(snapshots[phase], "StatefulSet", f"{CLUSTER_NAME}-{ROLE_NAME}-{group}")
            for group in GROUPS
        ]
        assert_pdb_contract(pdb, statefulsets, phase)

    postgres = [
        get(snapshots[phase], "StatefulSet", "upgrade-postgres") for phase in PHASES
    ]
    assert len({uid(item) for item in postgres}) == 1, "PostgreSQL StatefulSet changed"
    assert all(
        normalized_resource(item) == normalized_resource(postgres[0])
        for item in postgres[1:]
    ), "PostgreSQL StatefulSet spec changed"

    postgres_services = [
        get(snapshots[phase], "Service", "upgrade-postgres") for phase in PHASES
    ]
    assert service_identity(postgres_services[0]) == service_identity(
        postgres_services[1]
    ) == service_identity(postgres_services[2]), "PostgreSQL Service changed"

    cr_uids = [text(root, phase, "cluster-uid.txt") for phase in PHASES]
    assert len(set(cr_uids)) == 1 and cr_uids[0], ("SupersetCluster UID", cr_uids)
    credentials = [secret_identity(root, phase) for phase in PHASES]
    assert credentials[0] == credentials[1] == credentials[2], (
        "credentials Secret identity/digest changed",
        credentials,
    )
    storage_identities = [postgres_storage_identity(root, phase) for phase in PHASES]
    assert storage_identities[0] == storage_identities[1] == storage_identities[2], (
        "PostgreSQL PVC/PV identity changed",
        storage_identities,
    )

    admin_users = [text(root, phase, "admin-user.txt") for phase in PHASES]
    assert admin_users[0] == admin_users[1] == admin_users[2] and admin_users[0], (
        "Superset admin database identity changed",
        admin_users,
    )
    assert text(root, "before", "markers.txt").splitlines() == ["before-upgrade"]
    assert text(root, "after", "markers.txt").splitlines() == [
        "after-upgrade",
        "before-upgrade",
    ]
    assert text(root, "rollback", "markers.txt").splitlines() == [
        "after-rollback",
        "after-upgrade",
        "before-upgrade",
    ]
    for phase in PHASES:
        assert_api(root, phase)

    assert_operator_contract(root)

    assert normalized["rollback"] == normalized["before"], (
        "rollback left an unapproved normalized resource diff; inspect rollback.diff"
    )

    summary = {
        "resource_contract": "pass",
        "after_change_contract": "explicit resource-field seams plus semantic assertions",
        "operator_resource_contract": "pass",
        "listener_contract": "NodePort service identity and Listener desired-resource parity",
        "api_contract": "pass",
        "cluster_uid": cr_uids[0],
        "credentials_sha256": credentials[0]["data_sha256"],
        "postgres_storage": storage_identities[0],
        "admin_identity_sha256": hashlib.sha256(admin_users[0].encode()).hexdigest(),
        "intentional_after_diff": "after.diff",
        "rollback_diff": "empty",
    }
    (root / "comparison-summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n"
    )
    print("Migration contracts passed; normalized diffs and identity evidence saved")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit(f"usage: {sys.argv[0]} EVIDENCE_DIR")
    main(pathlib.Path(sys.argv[1]))

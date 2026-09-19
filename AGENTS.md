<!-- Generated: 2026-04-06 | Updated: 2026-04-06 -->

# superset-operator

## Purpose
Manages Apache Superset deployments on Kubernetes. Handles creation, configuration, and lifecycle management of Superset instances for data visualization and business intelligence.

## Key Files
| File | Description |
|------|-------------|
| `go.mod` | Go module dependencies (`github.com/zncdatadev/superset-operator`) |
| `Makefile` | Build and development commands |
| `PROJECT` | Kubebuilder project metadata |
| `Dockerfile` | Operator container image build |

## Subdirectories
| Directory | Purpose |
|-----------|---------|
| `api/v1alpha1/` | CRD types: `SupersetCluster` |
| `cmd/` | Operator entry point (`main.go`) |
| `config/` | Kubernetes manifests and kustomize configs |
| `deploy/` | Helm chart for operator deployment |
| `internal/controller/` | Reconciliation controllers |
| `internal/controller/cluster/` | Cluster-level reconciliation logic |
| `internal/controller/common/` | Shared controller utilities |
| `internal/controller/node/` | Node-level reconciliation logic |
| `internal/util/` | Utility functions |
| `test/` | E2E test suites |

## For AI Agents

### Working In This Directory
- Standard Kubebuilder operator structure
- Uses `github.com/zncdatadev/operator-go` framework for reconciliation
- Run `make test` for unit tests
- Run `make deploy` to deploy to cluster
- Go module: `github.com/zncdatadev/superset-operator`

### Testing Requirements
- E2E tests in `test/e2e/`
- Requires a running Kubernetes cluster
- Uses Ginkgo/Gomega test framework

### Common Patterns
- Controllers in `internal/controller/`
- CRDs use `v1alpha1` API version
- Follows `operator-go` `GenericReconciler` pattern
- Main CRD kind: `SupersetCluster` (group: `superset.kubedoop.dev`)

## Dependencies

### Internal
- `../operator-go` — Shared operator framework (`github.com/zncdatadev/operator-go v0.12.6`)

### External
- `sigs.k8s.io/controller-runtime v0.23.1`
- `k8s.io/client-go v0.35.0`
- `k8s.io/api v0.35.0`
- Go 1.25+

### AI Worktree Development Mode

**IMPORTANT**: When making code changes, work in a worktree under `.worktree/`, NOT in the main working directory.

#### Workflow
1. Create worktree: `git worktree add .worktree/<branch-name> -b <branch-name>`
2. Work in `.worktree/<branch-name>/` directory
3. Test: `cd .worktree/<branch-name> && make lint && make test`
4. Commit changes in the worktree
5. Push and create PR from the worktree branch
6. Cleanup: `git worktree remove .worktree/<branch-name>`

#### Rules
- NEVER modify files directly in the main working directory
- Each task gets its own worktree with a descriptive branch name
- Run `make generate` if API structs are modified
- Run `make lint && make test` before committing

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->

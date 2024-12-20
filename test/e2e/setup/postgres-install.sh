# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

helm upgrade --install superset \
  --version=12.5.6 \
  --namespace $NAMESPACE \
  -f "${SCRIPT_DIR}/helm-bitnami-postgresql-values.yaml" \
  --repo https://charts.bitnami.com/bitnami postgresql \

#!/bin/bash

set -euxo pipefail

url="${1-172.18.0.1.omg.howdoi.website}"
cluster="${2-k3d-k3s-second}"

# hardcoded from .github/assets/token-ci.yml
token="token-ci:zfllcbdr4677rkj4hmlr8rsmljg87l7874882928khlfs2pmmcq7l5"

user=$(kubectl get users -o go-template='{{range .items }}{{.metadata.name}}{{"\n"}}{{end}}' | tail -1)
sed "s/user-zvnsr/$user/" .github/assets/token-ci.yml | kubectl apply -f -

echo -e "4\n" | rancher login "https://$url" --token "$token" --skip-verify

rancher clusters create second --import

kubectl config use-context "$cluster"
rancher cluster import second
rancher cluster import second | grep curl | sh

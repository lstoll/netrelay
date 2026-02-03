#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"/..

KUBECONFIG="${PWD}/hack/kubeconfig"
export KUBECONFIG

if ! [ -x "$(command -v kind)" ]; then
  echo "brew install kind" >&2
  exit 1
fi

echo "--> Provisioning cluster"
kind create cluster -n connectproxy
kubectl config use-context kind-connectproxy

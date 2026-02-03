#!/usr/bin/env bash
set -euo pipefail

echo "--> Deleting cluster"
kind delete cluster --name connectproxy
rm hack/kubeconfig || true

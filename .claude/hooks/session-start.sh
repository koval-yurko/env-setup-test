#!/bin/bash
# Runs `terraform init` (HCP Terraform) at session start. Needs the factory
# secrets (TF_TOKEN_app_terraform_io, TF_CLOUD_*), so it cannot be an
# environment setup script. Never blocks the session: failures only warn.
set -uo pipefail

[ "${CLAUDE_CODE_REMOTE:-}" = "true" ] || exit 0

cd "${CLAUDE_PROJECT_DIR:-.}"

if [ -z "${TF_TOKEN_app_terraform_io:-}" ] || [ -z "${TF_CLOUD_ORGANIZATION:-}" ] || [ -z "${TF_CLOUD_PROJECT:-}" ]; then
  echo "session-start: HCP Terraform variables not set, skipping terraform init" >&2
  exit 0
fi
command -v terraform >/dev/null || { echo "session-start: terraform not installed, skipping" >&2; exit 0; }

# Skip the init if it already succeeded for the same configuration.
hashes=terraform/.terraform/.session-start.sha256
files=(.claude/hooks/session-start.sh terraform/main.tf terraform/.terraform.lock.hcl)
if ! sha256sum --check --status "$hashes" 2>/dev/null; then
  rm -f "$hashes"
  # Run terraform directly: `timeout` on make would leave a hung terraform child behind.
  export TF_CLI_CONFIG_FILE=/dev/null CHECKPOINT_DISABLE=1
  if timeout -k 5 "${TF_INIT_TIMEOUT:-60}" terraform -chdir=terraform init -input=false </dev/null; then
    sha256sum "${files[@]}" > "$hashes" 2>/dev/null || rm -f "$hashes"
  else
    echo "session-start: terraform init failed or timed out; run 'make tf-init' manually" >&2
  fi
fi
exit 0

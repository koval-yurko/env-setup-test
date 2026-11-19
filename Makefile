# Infra commands. They use only the process environment (the infra secrets
# from the factory). direnv (.envrc) loads .env into it: in a shell with the
# direnv hook, cd into the repo; in scripts and agents, run `direnv exec . make <target>`.
SSM_PARAM_NAME ?= /secret-probe/test/probe-param
AWS_REGION ?= eu-central-1
TF := terraform -chdir=terraform
# Ignore the host's ~/.terraformrc (dev_overrides, mirrors); this project needs none.
export TF_CLI_CONFIG_FILE ?= /dev/null

.PHONY: aws-whoami tf-check tf-vars tf-varset tf-init tf-ws tf-plan tf-apply tf-destroy ssm-get ssm-put

aws-whoami:
	aws sts get-caller-identity --output table

# HCP Terraform settings must come from env; no `terraform login`.
tf-check:
	@test -n "$$TF_TOKEN_app_terraform_io" || { echo "TF_TOKEN_app_terraform_io is not set"; exit 1; }
	@test -n "$$TF_CLOUD_ORGANIZATION" || { echo "TF_CLOUD_ORGANIZATION is not set"; exit 1; }
	@test -n "$$TF_CLOUD_PROJECT" || { echo "TF_CLOUD_PROJECT is not set"; exit 1; }

# Remote runs upload *.auto.tfvars, not TF_VAR_* env vars.
tf-vars:
	@printf 'param_name = "%s"\nregion     = "%s"\n' "$(SSM_PARAM_NAME)" "$(AWS_REGION)" > terraform/probe.auto.tfvars

# First run in an empty project: `make tf-init WS=<name>`. Terraform asks for a
# workspace name when no workspace has the tag yet; WS answers that prompt.
# Copy the sandbox AWS keys into the HCP variable set "secret-probe-aws" on
# TF_CLOUD_PROJECT, so remote runs can reach AWS. Safe to run again.
tf-varset: tf-check
	python3 scripts/hcp-varset.py

tf-init: tf-check
	@if [ -n "$(WS)" ]; then echo "$(WS)" | $(TF) init; else $(TF) init -input=false; fi

# Select a workspace (tag secret-probe) in TF_CLOUD_PROJECT, creating it if needed.
# Without WS it lists the workspaces.
tf-ws: tf-check
	@if [ -n "$(WS)" ]; then $(TF) workspace select -or-create "$(WS)"; else $(TF) workspace list; fi

tf-plan: tf-check tf-vars
	$(TF) plan -input=false

tf-apply: tf-check tf-vars
	$(TF) apply -input=false -auto-approve

tf-destroy: tf-check tf-vars
	$(TF) destroy -input=false -auto-approve

ssm-get:
	aws ssm get-parameter --name "$(SSM_PARAM_NAME)" --query Parameter.Value --output text

ssm-put: aws-whoami
	@test -n "$(VALUE)" || { echo "usage: make ssm-put VALUE=..."; exit 1; }
	aws ssm put-parameter --name "$(SSM_PARAM_NAME)" --value "$(VALUE)" --type String --overwrite

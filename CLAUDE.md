# CLAUDE.md

## Purpose

`secret-probe` is a test project for Software Factories. It checks secrets, environment setup, AWS and Terraform isolation, project-scoped access tokens and browser tests. Read `README.md` for the design and the test scenarios.

Keep the project small. Each part must make one factory capability visible. Do not add features that do not serve a test scenario in `README.md`.

## Layout

- `main.go`, `go.mod` – the Go web app (runtime secrets)
- `.env.example` – committed documentation of all variables
- `.env` – local values, git-ignored
- `.envrc` – direnv loads `.env` without overriding variables that are already set
- `Makefile` – infra commands only (AWS CLI, Terraform)
- `terraform/` – the SSM parameter; state and runs in HCP Terraform; `probe.auto.tfvars` is generated and git-ignored

## Commands

Load the environment first. Shell tools of agents do not run the direnv hook, so put `direnv exec .` before every command below, for example `direnv exec . make tf-plan`. Never use `source .env` or `set -a; . ./.env`: they override variables that the factory set.


- Start the app: `go run .` (reads `.env`, process env vars override it)
- Tests: `go vet ./... && go test ./...`
- Health check: `curl -s localhost:${PORT:-8080}/healthz`
- AWS identity: `make aws-whoami`
- Terraform: `make tf-varset` (AWS keys to HCP, once per project), `make tf-init`, `make tf-ws WS=<name>`, `make tf-plan`, `make tf-apply`, `make tf-destroy`
- SSM with the AWS CLI: `make ssm-get`, `make ssm-put VALUE=...`

## Rules for secrets

- Never commit `.env`, `*.tfstate`, `*.tfstate.backup` or `.terraform/`.
- When you add a variable, add it to `.env.example` with a dummy value, and to the variable table in `README.md`.
- The web page shows the full values of `PROBE_SECRET` and the SSM parameter. This is on purpose. These 2 values must be dummy values only.
- Never print, log or show `GITHUB_TOKEN`, `TF_TOKEN_app_terraform_io`, `AWS_SECRET_ACCESS_KEY` or `AWS_SESSION_TOKEN`. Show only the result of a call (GitHub login, HCP Terraform username, AWS ARN).
- Do not read `~/.aws`, `~/.config/gh` or other personal credential files directly. Use only the credentials that the environment gives to this project.
- AWS credentials come only from `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `AWS_SESSION_TOKEN`. Do not run `aws configure` or `aws sso login`. Keep `AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` set to `/dev/null`.

## Rules for AWS and Terraform

- Change only resources under `/secret-probe/` in SSM. Do not create other AWS resources.
- Run `make aws-whoami` before `make ssm-put`. Run `make tf-plan` before `make tf-apply`. Stop and ask if the account or the workspace is not the expected test one.
- After a test, run `make tf-destroy`, unless the user tells you to keep the parameter.
- Terraform uses only `TF_TOKEN_app_terraform_io`, `TF_CLOUD_ORGANIZATION` and `TF_CLOUD_PROJECT` from env. Select or create a workspace with `make tf-ws WS=<name>`; workspaces are found by the tag `secret-probe`. Do not run `terraform login`. Do not change the `cloud {}` block or add a backend.
- Runs are remote. Their AWS credentials come from a variable set attached to the HCP Terraform project, not from the sandbox keys.

## Browser tests

1. Start the app with `go run .` in the background.
2. Wait until `/healthz` returns `ok`.
3. Open `http://localhost:${PORT:-8080}/` and read the rows on the page.
4. Compare each row with the expected result of the scenario in `README.md`.
5. Stop the app when the test is complete.

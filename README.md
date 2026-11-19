# secret-probe

A small test project to check how a Software Factory (an AI agent platform that runs coding agents in a sandbox) handles secrets, environment setup and isolation.

The project is not a product. It is a probe: each part makes one factory capability visible, so that an agent (or a person) can see if the capability works.

## Project goal

Use this project to answer these questions about a Software Factory:

1. Can the factory give the project its **runtime secrets** (from `.env` or from its own secret store), and do factory values override `.env`?
2. Can the factory give the project its **infra secrets** (AWS keys and an HCP Terraform token) for this project only, and run the **AWS CLI** and **Terraform CLI** with them, with no CLI login?
3. Can the factory use a **project-scoped access token** for a third-party site (GitHub), and keep it apart from the personal session of the user?
4. Can the factory **start the web app**, **open a browser**, go to the app, and **read the correct secret on the web page**?

## Design

| Part | What it is | What it proves |
|------|------------|----------------|
| Web app | Go app in the project root, started with `go run .` | The factory can start an app with runtime secrets and test it in a browser |
| Runtime secrets | `.env` (git-ignored), `.env.example` (committed, documentation) | The factory can supply `.env` values, and factory env vars override them |
| GitHub token | `GITHUB_TOKEN`, a fine-grained PAT for a test account | Project-scoped access token for a third-party site |
| SSM parameter | One AWS SSM `String` parameter, managed by Terraform in `terraform/` | Project-scoped AWS credentials, Terraform create and destroy, AWS CLI read and update |
| HCP Terraform | State and runs in workspaces of one HCP Terraform project (`TF_CLOUD_PROJECT`), found by the tag `secret-probe` | Project-scoped Terraform token, remote state, remote runs, many workspaces |
| Makefile | Infra commands only (`aws`, `terraform`) | The factory can run infra tools with the infra secrets |

### Runtime and infra secrets

The project has 2 kinds of secrets:

- **Runtime secrets** are for the Go app. The app loads `.env` at start. A variable that is already set in the process environment wins over `.env`. This lets the factory override any value.
- **Infra secrets** are for the Makefile, the AWS CLI and Terraform. They are access keys in environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and, for temporary keys, `AWS_SESSION_TOKEN`). Nobody must log in with `aws configure` or `aws sso login` in the sandbox. The AWS CLI and the Terraform AWS provider read these variables directly. The Go app also uses them to read the SSM parameter.
- `AWS_CONFIG_FILE=/dev/null` and `AWS_SHARED_CREDENTIALS_FILE=/dev/null` stop a fallback to the personal `~/.aws` profiles of the host. If the keys are missing, the AWS rows fail. They never use another account.
- The **Terraform token** is `TF_TOKEN_app_terraform_io`. `TF_CLOUD_ORGANIZATION` and `TF_CLOUD_PROJECT` select the organization and the project. The project can hold any number of workspaces. Terraform finds them by the tag `secret-probe`, and `make tf-ws WS=<name>` selects one or creates it in the project. Nobody must run `terraform login`. `make tf-*` stops if one of these 3 variables is missing, so Terraform never falls back to a personal token in `~/.terraform.d/credentials.tfrc.json`.
- `TF_CLI_CONFIG_FILE=/dev/null` stops Terraform from reading the host's `~/.terraformrc`. The Makefile sets it by default.

#### AWS keys for remote runs

Plan and apply run on HCP Terraform workers, not in the sandbox. The workers do not see the sandbox environment. Give the AWS credentials to the runs with a **variable set that is attached to the project**. Then every new workspace in the project gets them. Put one of these in the variable set:

- **Dynamic credentials (recommended):** set the env vars `TFC_AWS_PROVIDER_AUTH=true` and `TFC_AWS_RUN_ROLE_ARN=<role ARN>`. The role trusts the HCP Terraform OIDC provider. No static key is stored.
- **Static keys:** set `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` as **sensitive** env vars. `make tf-varset` does this for you from the sandbox environment.

The sandbox AWS keys stay in use for the Go app, `make aws-whoami` and `make ssm-*`. The region and the parameter name reach the remote run through `terraform/probe.auto.tfvars`. `make tf-vars` writes this git-ignored file from `AWS_REGION` and `SSM_PARAM_NAME`, because remote runs do not get `TF_VAR_*` env vars.

| Variable | Kind | Used by | Example in `.env.example` |
|----------|------|---------|---------------------------|
| `PORT` | runtime | Go app | `8080` |
| `PROBE_SECRET` | runtime | Go app, shown on the page | `dummy-secret-from-dotenv` |
| `GITHUB_TOKEN` | runtime | Go app, calls the GitHub API | `github_pat_...` |
| `AWS_REGION` | runtime + infra | Go app, AWS CLI, Terraform | `eu-central-1` |
| `AWS_ACCESS_KEY_ID` | infra | AWS CLI, Terraform, Go app | `AKIA...` or `ASIA...` |
| `AWS_SECRET_ACCESS_KEY` | infra | AWS CLI, Terraform, Go app | never shown |
| `AWS_SESSION_TOKEN` | infra, only for temporary keys | AWS CLI, Terraform, Go app | never shown |
| `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE` | infra | AWS CLI, Terraform | `/dev/null` |
| `SSM_PARAM_NAME` | runtime + infra | Go app, Makefile, Terraform | `/secret-probe/test/probe-param` |
| `TF_TOKEN_app_terraform_io` | infra | Terraform, Go app | never shown |
| `TF_CLOUD_ORGANIZATION` | infra | Terraform, Go app | `my-test-org` |
| `TF_CLOUD_PROJECT` | infra | Terraform, Go app | `secret-probe` |
| `TF_CLI_CONFIG_FILE` | infra | Terraform | `/dev/null` |

### The web page

The page at `http://localhost:$PORT/` shows one row for each check:

| Row | Shows |
|-----|-------|
| `PROBE_SECRET` | The **full value**, and its source: `env` (from the factory) or `.env` (from the file) |
| GitHub | The login name that `GITHUB_TOKEN` belongs to (GitHub `GET /user`), or the error |
| HCP Terraform | The HCP Terraform username that the token belongs to, and the organization and project. Never the token |
| AWS identity | The account ID and the ARN from `aws sts get-caller-identity`, and the credential source (`access keys from .env`, `temporary keys from env`, ...). Never the key values |
| SSM parameter | The current value of `SSM_PARAM_NAME`, or `not found` |

> **Warning:** The page shows full secret values. Use dummy values for `PROBE_SECRET` and the SSM parameter only. Never put a real secret in these 2 fields.

`/healthz` returns `ok`, so that the factory can wait until the app is ready.

## Commands

Runtime (the Go app):

```sh
cp .env.example .env      # then edit the values
direnv allow              # once per machine; direnv now loads .env when you cd into the repo
go run .                  # starts on http://localhost:8080
```

Infra (the Makefile):

| Command | What it does |
|---------|--------------|
| `make aws-whoami` | `aws sts get-caller-identity` with the project keys |
| `make tf-check` | Stops if `TF_TOKEN_app_terraform_io`, `TF_CLOUD_ORGANIZATION` or `TF_CLOUD_PROJECT` is missing. All `tf-*` targets run it first |
| `make tf-vars` | Writes `terraform/probe.auto.tfvars` from `SSM_PARAM_NAME` and `AWS_REGION` |
| `make tf-varset` | Makes or updates the HCP variable set `secret-probe-aws`, attaches it to `TF_CLOUD_PROJECT`, and copies `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (sensitive) and `AWS_REGION` from the environment into it |
| `make tf-init` | `terraform init`, connects to HCP Terraform. In a project with no tagged workspace yet, use `make tf-init WS=<name>`: it makes the first workspace |
| `make tf-ws WS=<name>` | Selects the workspace, or creates it in `TF_CLOUD_PROJECT` with the tag `secret-probe`. Without `WS`, lists the workspaces |
| `make tf-plan` | `terraform plan`, runs remotely in HCP Terraform |
| `make tf-apply` | Creates the SSM parameter |
| `make tf-destroy` | Deletes the SSM parameter |
| `make ssm-get` | Reads the parameter with the AWS CLI |
| `make ssm-put VALUE=...` | Updates the parameter with the AWS CLI |

Load the environment first. `.envrc` makes direnv load `.env` into the shell:

- **Interactive shell** with the direnv hook: `cd` into the repo. Then run `make ...` and `go run .` as usual.
- **Agents, scripts and CI** (no shell hook): put `direnv exec .` before each command, for example `direnv exec . make tf-plan` or `direnv exec . go run .`.

A variable that the environment already sets (for example from the factory secret store) wins over `.env`. Empty values in `.env` count as not set.

When direnv is active, the `.env` values are already in the process environment, so the page shows `source: env` for them. To see `source: .env` for scenario 1, remove the variable from the process environment for this one run: `env -u PROBE_SECRET go run .`. The app then reads it from `.env`.

Terraform keeps its state in the selected HCP Terraform workspace. Each workspace has its own state. No state file is in the project.

The Go app calls the AWS CLI for the AWS identity and SSM rows, so it uses the same credentials as the Makefile.

## Test scenarios for a Software Factory

Give the factory one scenario at a time. Each scenario has a result that you can see.

| # | Ask the factory to | Expected result |
|---|--------------------|-----------------|
| 1 | Create `.env` from `.env.example`, start the app, open it in the browser | The page shows `PROBE_SECRET` = `dummy-secret-from-dotenv`, source `.env` |
| 2 | Set `PROBE_SECRET` in the factory secret store, restart the app | The page shows the factory value, source `env` |
| 3 | Use the project GitHub token | The page shows the login of the **test** account, not your personal account |
| 4 | Open `github.com` in the factory browser | The signed-in account is the test account, or no account. It is never your personal account |
| 5a | Open the page with the project Terraform token | The HCP Terraform row shows the **test** user, organization and project |
| 5 | Run `make aws-whoami` with no CLI login | The account ID and the user or role are the ones for this project. The page says `credentials: access keys from env` (or `.env`) |
| 6 | Run `make tf-init WS=sandbox-1`, `make tf-ws WS=sandbox-1`, `make tf-apply` with no `terraform login` | The workspace `sandbox-1` is in the project, and the run shows there. The SSM parameter exists. The page shows its value |
| 7 | Run `make ssm-put VALUE=updated-by-cli`, then reload the page | The page shows `updated-by-cli` |
| 8 | Run `make tf-destroy`, then reload the page | The page shows `not found` for the SSM parameter |
| 9 | Look at `git status` and the logs | `.env` and `probe.auto.tfvars` are not committed. `GITHUB_TOKEN`, the Terraform token and AWS keys are not in the logs |

A factory passes when all results are correct, and no secret from another project or from your personal setup is visible.

## Prerequisites

- Go 1.22 or later
- Python 3 (only for `make tf-varset`, standard library only)
- direnv 2.x with the shell hook (`eval "$(direnv hook zsh)"` in `~/.zshrc`), then `direnv allow` once in the repo
- AWS CLI v2
- Terraform 1.6 or later
- AWS access keys of a test IAM user or role, with permission for `ssm:*Parameter*` and tags on `/secret-probe/*`. Temporary keys from STS work too
- A GitHub test account with a fine-grained PAT (read-only, no repository access is necessary)
- An HCP Terraform test organization with one project, a variable set with the AWS credentials attached to that project (see above), and a team or user API token for `TF_TOKEN_app_terraform_io` that may create workspaces in the project. New workspaces use the default execution mode **Remote**

## Status

The Go app, `.env.example`, `Makefile` and `terraform/` exist. Run `go test ./...` for the `.env` loader test.

Scenarios 1 and 2 are checked in a browser. Scenarios 3 to 8 need a GitHub test token, test AWS access keys and an HCP Terraform test project with its token.

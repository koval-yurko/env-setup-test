# One SSM parameter for the secret-probe write-access test.
# State and runs live in HCP Terraform. Everything comes from the environment:
#   TF_TOKEN_app_terraform_io  API token (no `terraform login`)
#   TF_CLOUD_ORGANIZATION      HCP Terraform organization
#   TF_CLOUD_PROJECT           HCP Terraform project; new workspaces are created in it
# Workspaces are found by tag, so the project can hold any number of them.
# Select or create one with `make tf-ws WS=<name>`.
# Runs are remote, so AWS credentials for the run come from a variable set on the
# HCP Terraform project (see README.md), not from the sandbox AWS keys.
terraform {
  required_version = ">= 1.6"

  cloud {
    workspaces {
      tags = ["secret-probe"]
    }
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# param_name and region come from probe.auto.tfvars, which `make tf-vars` writes
# from SSM_PARAM_NAME and AWS_REGION. Remote runs upload *.auto.tfvars files but
# not TF_VAR_* environment variables.
variable "region" {
  type    = string
  default = "eu-central-1"
}

variable "param_name" {
  type    = string
  default = "/secret-probe/test/probe-param"

  validation {
    condition     = startswith(var.param_name, "/secret-probe/")
    error_message = "param_name must start with /secret-probe/."
  }
}

variable "param_value" {
  type    = string
  default = "created-by-terraform"
}

resource "aws_ssm_parameter" "probe" {
  name  = var.param_name
  type  = "String"
  value = var.param_value
  tags  = { project = "secret-probe" }
}

output "param_name" {
  value = aws_ssm_parameter.probe.name
}

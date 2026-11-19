#!/usr/bin/env python3
"""Create or update the HCP Terraform variable set "secret-probe-aws".

It attaches the set to the project TF_CLOUD_PROJECT and copies AWS_ACCESS_KEY_ID,
AWS_SECRET_ACCESS_KEY (both sensitive) and AWS_REGION from the environment, so
remote runs use the same AWS account as the sandbox. Safe to run again.
Values are never printed. Run it with `make tf-varset`.
"""
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

NAME = "secret-probe-aws"
API = "https://app.terraform.io/api/v2"
VARS = [("AWS_ACCESS_KEY_ID", True), ("AWS_SECRET_ACCESS_KEY", True), ("AWS_REGION", False)]


def call(method, path, body=None):
    req = urllib.request.Request(
        API + path,
        method=method,
        data=json.dumps(body).encode() if body else None,
        headers={
            "Authorization": "Bearer " + os.environ["TF_TOKEN_app_terraform_io"],
            "Content-Type": "application/vnd.api+json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            text = r.read()
            return json.loads(text) if text else {}
    except urllib.error.HTTPError as e:
        sys.exit(f"{method} {path}: HTTP {e.code} {e.read().decode(errors='replace')[:300]}")


def main():
    missing = [k for k, _ in VARS if not os.environ.get(k)]
    if missing:
        sys.exit("not set: " + ", ".join(missing))
    org, project = os.environ["TF_CLOUD_ORGANIZATION"], os.environ["TF_CLOUD_PROJECT"]

    projects = call("GET", f"/organizations/{org}/projects?filter%5Bnames%5D={urllib.parse.quote(project)}")["data"]
    if not projects:
        sys.exit(f"project {project!r} not found")
    pid = projects[0]["id"]

    found = [v for v in call("GET", f"/organizations/{org}/varsets?q={NAME}")["data"] if v["attributes"]["name"] == NAME]
    if found:
        vid = found[0]["id"]
        print(f"variable set {NAME}: exists")
    else:
        body = {"data": {"type": "varsets", "attributes": {
            "name": NAME, "global": False,
            "description": "AWS credentials for secret-probe remote runs (from the sandbox env)"}}}
        vid = call("POST", f"/organizations/{org}/varsets", body)["data"]["id"]
        print(f"variable set {NAME}: created")

    call("POST", f"/varsets/{vid}/relationships/projects", {"data": [{"type": "projects", "id": pid}]})
    print(f"attached to project {project}")

    have = {v["attributes"]["key"]: v["id"] for v in call("GET", f"/varsets/{vid}/relationships/vars")["data"]}
    for key, sensitive in VARS:
        body = {"data": {"type": "vars", "attributes": {
            "key": key, "value": os.environ[key], "category": "env", "sensitive": sensitive}}}
        if key in have:
            call("PATCH", f"/varsets/{vid}/relationships/vars/{have[key]}", body)
            print(f"{key}: updated")
        else:
            call("POST", f"/varsets/{vid}/relationships/vars", body)
            print(f"{key}: added")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Provision the isolated worker without starting it; deploy via sae-deploy-all.sh."""
import argparse
import json
import subprocess
import sys
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--profile", default="xiaohuiji-sae")
parser.add_argument("--region", default="ap-southeast-1")
parser.add_argument("--base-app", default="0deb4862-2af5-4c0f-9e82-ffa67364843e")
parser.add_argument("--name", default="xiaohuiji-sae-collect-process")
parser.add_argument("--apply", action="store_true")
args = parser.parse_args()


def call(operation, params):
    command = ["aliyun", "--profile", args.profile, "sae", operation,
               "--RegionId", args.region]
    for name, value in params.items():
        command.extend(["--" + name, str(value)])
    result = subprocess.run(command, capture_output=True, text=True)
    if result.returncode:
        # CLI diagnostics can echo ENV arguments. Never relay raw stderr.
        with tempfile.NamedTemporaryFile(mode="w", prefix="youban-sae-diagnostic-", delete=False) as diagnostic:
            diagnostic.write(result.stderr)
            diagnostic.write(result.stdout)
        raise RuntimeError(operation + " failed; protected diagnostics: " + diagnostic.name)
    result = json.loads(result.stdout)
    if str(result.get("Code")) != "200":
        raise RuntimeError(operation + " failed: " + str(result.get("Code")))
    return result.get("Data") or {}


try:
    base = call("DescribeApplicationConfig", {"AppId": args.base_app})
    listing = call("ListApplications", {"NamespaceId": base["NamespaceId"], "PageSize": 100})
    applications = listing.get("Applications") or []
    if int(listing.get("TotalSize", len(applications))) > len(applications):
        raise RuntimeError("application inventory is paginated; refusing incomplete duplicate check")
    existing = [app for app in applications if app.get("AppName") == args.name]
    if existing:
        print(json.dumps({"status": "exists", "appId": existing[0]["AppId"]}))
        sys.exit(0)
    envs = {item["name"]: item for item in json.loads(base.get("Envs") or "[]")}
    for name, value in {
        "YOUBAN_RUNTIME_ROLES": "collect-process-worker",
        "YOUBAN_COLLECT_PROCESS_ISOLATED": "true",
        "YOUBAN_DB_MAX_OPEN": "12",
        "YOUBAN_DB_MAX_IDLE": "3",
    }.items():
        envs[name] = {"name": name, "value": value}
    params = {
        "AppName": args.name, "PackageType": "Image", "Replicas": 1,
        "Deploy": "false", "AutoConfig": "false", "Cpu": 1000, "Memory": 2048,
        "Command": "/app/hotgo", "CommandArgs": '["collect-process-worker"]',
        "Envs": json.dumps(list(envs.values())),
        "Readiness": json.dumps({"httpGet": {"path": "/readyz", "port": 8000, "scheme": "http"},
                                 "initialDelaySeconds": 20, "periodSeconds": 10,
                                 "timeoutSeconds": 5}),
        "Liveness": json.dumps({"httpGet": {"path": "/healthz", "port": 8000, "scheme": "http"},
                                "initialDelaySeconds": 30, "periodSeconds": 15,
                                "timeoutSeconds": 2}),
    }
    for key in ("NamespaceId", "VpcId", "VSwitchId", "SecurityGroupId", "ImageUrl",
                "AcrInstanceId", "ConfigMapMountDesc", "SecretMountDesc", "Timezone"):
        if base.get(key):
            params[key] = base[key]
    print(json.dumps({"name": args.name, "baseApp": args.base_app,
                      "deploy": False, "cpu": 1000, "memory": 2048,
                      "role": "collect-process-worker", "dbMaxOpen": 12}))
    if args.apply:
        created = call("CreateApplication", params)
        print(json.dumps({"status": "created_not_deployed", "appId": created.get("AppId")}))
except (RuntimeError, KeyError, ValueError) as error:
    print(str(error), file=sys.stderr)
    sys.exit(1)

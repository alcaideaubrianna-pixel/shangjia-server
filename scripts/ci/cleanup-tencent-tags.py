#!/usr/bin/env python3
"""Keep only the newest Tencent TCR personal-repository tags."""

import datetime as dt
import hashlib
import hmac
import json
import os
import sys
import urllib.request


ENDPOINT = os.getenv("TCR_API_ENDPOINT", "tcr.tencentcloudapi.com")
SERVICE = "tcr"
VERSION = "2019-09-24"


def required(name):
    value = os.getenv(name, "").strip()
    if not value:
        raise RuntimeError(f"missing {name}; Tencent tag cleanup cannot continue")
    return value


def sha256(value):
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def sign_request(action, payload, secret_id, secret_key, region):
    timestamp = int(dt.datetime.now(dt.timezone.utc).timestamp())
    date = dt.datetime.fromtimestamp(timestamp, dt.timezone.utc).strftime("%Y-%m-%d")
    body = json.dumps(payload, separators=(",", ":"), ensure_ascii=False)
    content_type = "application/json"
    canonical_headers = f"content-type:{content_type}\nhost:{ENDPOINT}\n"
    signed_headers = "content-type;host"
    canonical_request = "\n".join(
        ["POST", "/", "", canonical_headers, signed_headers, sha256(body)]
    )
    credential_scope = f"{date}/{SERVICE}/tc3_request"
    string_to_sign = "\n".join(
        [
            "TC3-HMAC-SHA256",
            str(timestamp),
            credential_scope,
            sha256(canonical_request),
        ]
    )
    secret_date = hmac.new(
        ("TC3" + secret_key).encode(), date.encode(), hashlib.sha256
    ).digest()
    secret_service = hmac.new(secret_date, SERVICE.encode(), hashlib.sha256).digest()
    secret_signing = hmac.new(
        secret_service, b"tc3_request", hashlib.sha256
    ).digest()
    signature = hmac.new(
        secret_signing, string_to_sign.encode(), hashlib.sha256
    ).hexdigest()
    authorization = (
        f"TC3-HMAC-SHA256 Credential={secret_id}/{credential_scope}, "
        f"SignedHeaders={signed_headers}, Signature={signature}"
    )
    return body, {
        "Content-Type": content_type,
        "Host": ENDPOINT,
        "X-TC-Action": action,
        "X-TC-Version": VERSION,
        "X-TC-Timestamp": str(timestamp),
        "X-TC-Region": region,
        "Authorization": authorization,
    }


def call_api(action, payload, secret_id, secret_key, region):
    body, headers = sign_request(action, payload, secret_id, secret_key, region)
    request = urllib.request.Request(
        f"https://{ENDPOINT}/", data=body.encode("utf-8"), headers=headers, method="POST"
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        result = json.loads(response.read().decode("utf-8"))
    error = result.get("Response", {}).get("Error")
    if error:
        raise RuntimeError(f"{error.get('Code')}: {error.get('Message')}")
    return result.get("Response", {})


def list_tags(repo_name, secret_id, secret_key, region):
    tags = []
    offset = 0
    limit = 100
    while True:
        response = call_api(
            "DescribeImagePersonal",
            {"RepoName": repo_name, "Offset": offset, "Limit": limit},
            secret_id,
            secret_key,
            region,
        )
        data = response.get("Data") or {}
        page = data.get("TagInfo") or []
        tags.extend(page)
        if len(page) < limit or len(tags) >= int(data.get("TagCount") or len(tags)):
            return tags
        offset += limit


def tag_sort_key(tag):
    return (
        tag.get("PushTime")
        or tag.get("UpdateTime")
        or tag.get("CreationTime")
        or ""
    )


def main():
    secret_id = required("TENCENTCLOUD_SECRET_ID")
    secret_key = required("TENCENTCLOUD_SECRET_KEY")
    repo_name = required("TCR_REPO_NAME")
    region = os.getenv("TCR_REGION", "ap-hongkong").strip() or "ap-hongkong"
    keep_count = int(os.getenv("TCR_KEEP_TAGS", "50"))
    if keep_count < 1:
        raise ValueError("TCR_KEEP_TAGS must be greater than zero")

    tags = sorted(list_tags(repo_name, secret_id, secret_key, region), key=tag_sort_key, reverse=True)
    tags_to_delete = [tag for tag in tags[keep_count:] if tag.get("TagName")]
    print(f"TCR repository: {repo_name}; total tags: {len(tags)}; keep: {keep_count}; delete: {len(tags_to_delete)}")
    if not tags_to_delete:
        return

    if os.getenv("TCR_CLEANUP_DRY_RUN", "false").lower() in {"1", "true", "yes"}:
        print("dry run tags:", ", ".join(tag["TagName"] for tag in tags_to_delete))
        return

    for start in range(0, len(tags_to_delete), 100):
        batch = tags_to_delete[start : start + 100]
        call_api(
            "BatchDeleteImagePersonal",
            {"RepoName": repo_name, "Tags": [tag["TagName"] for tag in batch]},
            secret_id,
            secret_key,
            region,
        )
        print("deleted:", ", ".join(tag["TagName"] for tag in batch))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"Tencent TCR cleanup failed: {error}", file=sys.stderr)
        sys.exit(1)

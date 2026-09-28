import json
import os
import urllib.error
import urllib.request

import boto3


def handler(event, _context):
    data = event.get("data") or {}
    name = data.get("name") or ""
    principal = data.get("bound_iam_principal_arn") or ""
    if name == "" or principal == "":
        raise ValueError("name and bound_iam_principal_arn are required")

    action = (event.get("tf") or {}).get("action") or "create"
    token = boto3.client("secretsmanager").get_secret_value(
        SecretId=os.environ["VAULT_TOKEN_SECRET_ID"]
    )["SecretString"].strip()
    addr = os.environ["VAULT_ADDR"].rstrip("/")

    status, body = call(addr, token, "POST", "/v1/sys/auth/aws", {"type": "aws"})
    if status >= 400 and b"already in use" not in body:
        raise RuntimeError(f"enable aws auth failed: {status} {body.decode()}")

    path = "/v1/auth/aws/role/" + name
    if action == "delete":
        status, body = call(addr, token, "DELETE", path, None)
        if status >= 400 and status != 404:
            raise RuntimeError(f"delete role failed: {status} {body.decode()}")
        return {"name": name}

    status, body = call(addr, token, "POST", path, {
        "auth_type": "iam",
        "bound_iam_principal_arn": [principal],
        "policies": data.get("policies") or [],
    })
    if status >= 400:
        raise RuntimeError(f"write role failed: {status} {body.decode()}")
    return {"name": name}


def call(addr, token, method, path, body):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(addr + path, data=data, method=method)
    req.add_header("X-Vault-Token", token)
    if body is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=8) as res:
            return res.status, res.read()
    except urllib.error.HTTPError as err:
        return err.code, err.read()

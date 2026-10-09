#!/usr/bin/env python3
"""Start local RustFS storage for content-sources-backend.

Usage: content-sources-rustfs [start|stop|status] [--service lightwell|pulp|both] [--docker]
Requires podman or docker and PyYAML. Reads configs/config.yaml; never stores credentials here.
Runtime: prefers podman if installed, otherwise uses docker; pass --docker to force docker instead.
"""

import argparse
import datetime
import hashlib
import hmac
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request

import yaml


_REPO_ROOT = Path(__file__).resolve().parent.parent
CONFIG = Path(os.environ.get(
    "CONTENT_SOURCES_CONFIG",
    str(_REPO_ROOT / "configs" / "config.yaml"),
))
IMAGE = os.environ.get("RUSTFS_IMAGE", "quay.io/rustfs/rustfs:latest")
NETWORK = os.environ.get("COMPOSE_PROJECT_NAME", "cs") + "_default"
CONTAINER = None


def detect_container(force_docker=False):
    if force_docker:
        return "docker"
    for runtime in ("podman", "docker"):
        if subprocess.run(
            [runtime, "-v"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False
        ).returncode == 0:
            return runtime
    raise RuntimeError("Neither podman nor docker found")


def container(*args, env=None, check=True):
    result = subprocess.run(
        [CONTAINER, *args], env=env, text=True, capture_output=True, check=False
    )
    if check and result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    return result


def container_exists(name):
    return container("inspect", name, check=False).returncode == 0


def load_services(selection):
    with CONFIG.open(encoding="utf-8") as stream:
        cfg = yaml.safe_load(stream)
    clients = cfg["clients"]
    services = []
    if selection in ("both", "lightwell"):
        lightwell = clients["lightwell"]["s3"]["coverage_uploads"]
        services.append(("lightwell", lightwell, 9002, 9003))
        if lightwell.get("url") != "http://localhost:9002":
            raise ValueError("Lightwell URL must be http://localhost:9002 for this script")
    if selection in ("both", "pulp"):
        pulp = clients["pulp"]["custom_repo_objects"]
        services.append(("pulp", pulp, 9004, 9005))
    for label, store, _, _ in services:
        for key in ("name", "access_key", "secret_key", "region"):
            if not store.get(key):
                raise ValueError(f"{label}: clients config has no {key}")
    return services


def wait_ready(port):
    url = f"http://127.0.0.1:{port}/health/ready"
    for _ in range(60):
        try:
            with urllib.request.urlopen(url, timeout=2) as response:
                if response.status == 200:
                    return
        except OSError:
            pass
        time.sleep(1)
    raise RuntimeError(f"RustFS on port {port} did not become ready; check {CONTAINER} logs")


def signed_request(method, port, bucket, store):
    """Send a path-style, empty-body S3 request using AWS Signature V4."""
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    day = now[:8]
    region = store["region"]
    host = f"127.0.0.1:{port}"
    payload_hash = hashlib.sha256(b"").hexdigest()
    headers = {
        "host": host,
        "x-amz-content-sha256": payload_hash,
        "x-amz-date": now,
    }
    names = ";".join(sorted(headers))
    canonical_headers = "".join(f"{key}:{headers[key]}\n" for key in sorted(headers))
    path = "/" + bucket
    canonical_request = "\n".join(
        (method, path, "", canonical_headers, names, payload_hash)
    )
    scope = f"{day}/{region}/s3/aws4_request"
    string_to_sign = "\n".join((
        "AWS4-HMAC-SHA256", now, scope,
        hashlib.sha256(canonical_request.encode()).hexdigest(),
    ))
    key = ("AWS4" + store["secret_key"]).encode()
    for part in (day, region, "s3", "aws4_request"):
        key = hmac.new(key, part.encode(), hashlib.sha256).digest()
    signature = hmac.new(key, string_to_sign.encode(), hashlib.sha256).hexdigest()
    authorization = (
        f"AWS4-HMAC-SHA256 Credential={store['access_key']}/{scope}, "
        f"SignedHeaders={names}, Signature={signature}"
    )
    request = urllib.request.Request(
        f"http://{host}{path}", method=method,
        headers={
            "Host": host,
            "X-Amz-Content-Sha256": payload_hash,
            "X-Amz-Date": now,
            "Authorization": authorization,
        },
        data=b"" if method == "PUT" else None,
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return response.status
    except urllib.error.HTTPError as error:
        if error.code == 404:
            return 404
        raise RuntimeError(f"S3 {method} {bucket} returned HTTP {error.code}") from None


def ensure_bucket(port, store):
    bucket = store["name"]
    if signed_request("HEAD", port, bucket, store) == 404:
        signed_request("PUT", port, bucket, store)
        print(f"Created bucket {bucket}")
    else:
        print(f"Bucket {bucket} is ready")


def attach_pulp_network():
    networks = container("network", "ls", "--format", "{{.Name}}")
    return NETWORK in networks.stdout.splitlines()


def start(selection):
    services = load_services(selection)
    pulp_network_exists = attach_pulp_network() if selection in ("both", "pulp") else False
    for label, store, port, console_port in services:
        name = f"content-sources-rustfs-{label}"
        if label == "pulp" and pulp_network_exists:
            if CONTAINER == "docker":
                network_args = ["--network", NETWORK, "--network-alias", "minio"]
            else:
                network_args = ["--network", f"{NETWORK}:alias=minio"]
        else:
            network_args = []
        if label == "pulp" and container_exists(name):
            mode = container("inspect", "--format", "{{.HostConfig.NetworkMode}}", name).stdout.strip()
            if pulp_network_exists and mode != NETWORK:
                container("stop", name)
                container("rm", name)
                print(f"Recreating {name} on {NETWORK}; its data volume is retained")
        if not container_exists(name):
            env = os.environ.copy()
            env.update({
                "RUSTFS_ACCESS_KEY": store["access_key"],
                "RUSTFS_SECRET_KEY": store["secret_key"],
                "RUSTFS_REGION": store["region"],
            })
            container(
                "run", "-d", "--name", name, "--restart", "unless-stopped",
                "-p", f"127.0.0.1:{port}:9000",
                "-p", f"127.0.0.1:{console_port}:9001",
                "-v", f"{name}-data:/data",
                *network_args,
                "-e", "RUSTFS_ACCESS_KEY", "-e", "RUSTFS_SECRET_KEY",
                "-e", "RUSTFS_REGION", "-e", "RUSTFS_CONSOLE_ENABLE=true",
                IMAGE, "/data", env=env,
            )
            print(f"Started {name}")
        elif container("inspect", "--format", "{{.State.Running}}", name).stdout.strip() != "true":
            container("start", name)
            print(f"Started {name}")
        else:
            print(f"{name} is already running")
        wait_ready(port)
        ensure_bucket(port, store)
    if selection in ("both", "pulp"):
        if pulp_network_exists:
            print(f"Pulp container available as minio:9000 on {NETWORK}")
        else:
            print(f"Container network {NETWORK} is absent. Run this script again after compose is up to add the minio alias.")
        print("Pulp S3 on host: http://localhost:9004; console: http://localhost:9005")
        print("For a host-run backend using Pulp object storage, change clients.pulp.custom_repo_objects.url to http://localhost:9004.")
    if selection in ("both", "lightwell"):
        print("Lightwell S3: http://localhost:9002; console: http://localhost:9003")


def status(selection):
    for label, _, port, _ in load_services(selection):
        name = f"content-sources-rustfs-{label}"
        state = "absent"
        if container_exists(name):
            state = container("inspect", "--format", "{{.State.Status}}", name).stdout.strip()
        print(f"{name}: {state} (host port {port})")


def stop(selection):
    for label, _, _, _ in load_services(selection):
        name = f"content-sources-rustfs-{label}"
        if container_exists(name):
            container("stop", name)
            print(f"Stopped {name}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Manage local RustFS for content-sources-backend.")
    parser.add_argument("command", nargs="?", choices=("start", "status", "stop"), default="start")
    parser.add_argument("--service", choices=("lightwell", "pulp", "both"), default="both")
    parser.add_argument("--docker", action="store_true", help="Use docker instead of auto-detected runtime (podman preferred)")
    args = parser.parse_args()
    CONTAINER = detect_container(force_docker=args.docker)
    try:
        {"start": start, "status": status, "stop": stop}[args.command](args.service)
    except (OSError, KeyError, ValueError, RuntimeError) as error:
        parser.exit(1, f"Error: {error}\n")

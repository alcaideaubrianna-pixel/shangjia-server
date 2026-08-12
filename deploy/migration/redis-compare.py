#!/usr/bin/env python3
import argparse
import socket
import ssl
import sys
from urllib.parse import urlparse


class Redis:
    def __init__(self, host, port, password="", db=0, tls=False):
        sock = socket.create_connection((host, port), timeout=30)
        if tls:
            sock = ssl.create_default_context().wrap_socket(sock, server_hostname=host)
        sock.settimeout(60)
        self.sock = sock
        self.file = sock.makefile("rb")
        if password:
            self.send("AUTH", password)
        if db:
            self.send("SELECT", db)

    def close(self):
        self.file.close()
        self.sock.close()

    def send(self, *args):
        body = [f"*{len(args)}\r\n".encode()]
        for arg in args:
            if isinstance(arg, bytes):
                value = arg
            else:
                value = str(arg).encode()
            body.append(f"${len(value)}\r\n".encode() + value + b"\r\n")
        self.sock.sendall(b"".join(body))
        return self.read()

    def read(self):
        prefix = self.file.read(1)
        if not prefix:
            raise RuntimeError("Redis 连接被关闭")
        line = self.file.readline().rstrip(b"\r\n")
        if prefix == b"+":
            return line
        if prefix == b"-":
            raise RuntimeError(line.decode(errors="replace"))
        if prefix == b":":
            return int(line)
        if prefix == b"$":
            size = int(line)
            if size < 0:
                return None
            value = self.file.read(size)
            self.file.read(2)
            return value
        if prefix == b"*":
            size = int(line)
            return [self.read() for _ in range(size)]
        raise RuntimeError(f"未知 Redis 响应：{prefix!r}")


def parse_url(value):
    parsed = urlparse(value)
    if parsed.scheme not in ("redis", "rediss"):
        raise ValueError("Redis 地址必须使用 redis:// 或 rediss://")
    return (
        parsed.hostname,
        parsed.port or 6379,
        parsed.password or "",
        int(parsed.path.strip("/") or 0),
        parsed.scheme == "rediss",
    )


def fingerprint(redis, progress_name):
    cursor = b"0"
    entries = {}
    while True:
        cursor, keys = redis.send("SCAN", cursor, "COUNT", 500)
        for key in keys:
            payload = redis.send("DUMP", key)
            ttl = redis.send("PTTL", key)
            if payload is None:
                continue
            entries[key] = (int(ttl), payload)
        if len(entries) and len(entries) % 5000 == 0:
            print(f"{progress_name} 已检查 {len(entries)} 个 Key", flush=True)
        if cursor in (b"0", 0, "0"):
            break
    return entries


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-host", required=True)
    parser.add_argument("--source-port", type=int, required=True)
    parser.add_argument("--source-password", default="")
    parser.add_argument("--source-db", type=int, default=0)
    parser.add_argument("--target-url", required=True)
    parser.add_argument("--ttl-tolerance-ms", type=int, default=5000)
    args = parser.parse_args()
    source = Redis(args.source_host, args.source_port, args.source_password, args.source_db)
    destination = Redis(*parse_url(args.target_url))
    try:
        source.send("PING")
        destination.send("PING")
        source_entries = fingerprint(source, "源 Redis")
        target_entries = fingerprint(destination, "目标 Redis")
        source_keys = set(source_entries)
        target_keys = set(target_entries)
        missing = sorted(source_keys - target_keys)
        extra = sorted(target_keys - source_keys)
        changed = []
        for key in sorted(source_keys & target_keys):
            source_ttl, source_payload = source_entries[key]
            target_ttl, target_payload = target_entries[key]
            if source_payload != target_payload or abs(source_ttl - target_ttl) > args.ttl_tolerance_ms:
                changed.append(key)
        print(f"source_keys={len(source_entries)}")
        print(f"target_keys={len(target_entries)}")
        print(f"missing_keys={len(missing)}")
        print(f"extra_keys={len(extra)}")
        print(f"changed_keys={len(changed)}")
        if missing or extra or changed:
            if missing:
                print(f"缺少 Key（最多 20 个）：{[key.decode(errors='replace') for key in missing[:20]]}", file=sys.stderr)
            if extra:
                print(f"多出 Key（最多 20 个）：{[key.decode(errors='replace') for key in extra[:20]]}", file=sys.stderr)
            if changed:
                print(f"内容或 TTL 不一致 Key（最多 20 个）：{[key.decode(errors='replace') for key in changed[:20]]}", file=sys.stderr)
            print("Redis 一致性校验失败：Key 集合、内容或 TTL 超出容差", file=sys.stderr)
            return 2
        print("Redis 一致性校验通过")
        return 0
    finally:
        source.close()
        destination.close()


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"Redis 一致性校验失败：{exc}", file=sys.stderr)
        raise SystemExit(1)

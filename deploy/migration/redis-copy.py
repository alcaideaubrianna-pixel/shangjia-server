#!/usr/bin/env python3
import argparse
import socket
import ssl
import sys
import time
from urllib.parse import urlparse

class Redis:
    def __init__(self, host, port, password="", db=0, tls=False):
        self.sock = socket.create_connection((host, port), timeout=30)
        if tls:
            self.sock = ssl.create_default_context().wrap_socket(self.sock, server_hostname=host)
        self.sock.settimeout(60)
        self.file = self.sock.makefile("rb")
        self.send("AUTH", password) if password else None
        if db:
            self.send("SELECT", db)

    def close(self):
        self.file.close()
        self.sock.close()

    def send(self, *args):
        data = [f"*{len(args)}\r\n".encode()]
        for arg in args:
            if isinstance(arg, str):
                arg = arg.encode()
            elif isinstance(arg, int):
                arg = str(arg).encode()
            data.append(f"${len(arg)}\r\n".encode() + arg + b"\r\n")
        self.sock.sendall(b"".join(data))
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

def parse_target(url):
    parsed = urlparse(url)
    if parsed.scheme not in ("redis", "rediss"):
        raise ValueError("TARGET_REDIS_URL 必须是 redis:// 或 rediss://")
    password = parsed.password or ""
    host = parsed.hostname
    port = parsed.port or 6379
    db = int(parsed.path.strip("/") or 0)
    return host, port, password, db, parsed.scheme == "rediss"

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-host", required=True)
    parser.add_argument("--source-port", type=int, required=True)
    parser.add_argument("--source-password", default="")
    parser.add_argument("--source-db", type=int, default=0)
    parser.add_argument("--target-url", required=True)
    parser.add_argument("--progress-every", type=int, default=500)
    args = parser.parse_args()
    target = parse_target(args.target_url)
    source = Redis(args.source_host, args.source_port, args.source_password, args.source_db)
    destination = Redis(*target)
    cursor = b"0"
    copied = 0
    started = time.time()
    try:
        source.send("PING")
        destination.send("PING")
        while True:
            result = source.send("SCAN", cursor, "COUNT", 500)
            cursor, keys = result
            for key in keys:
                payload = source.send("DUMP", key)
                if payload is None:
                    continue
                ttl = source.send("PTTL", key)
                if ttl < 0:
                    ttl = 0
                destination.send("RESTORE", key, ttl, payload, "REPLACE")
                copied += 1
                if copied % args.progress_every == 0:
                    elapsed = max(time.time() - started, 0.001)
                    print(f"Redis 已复制 {copied} 个 Key，速度 {copied / elapsed:.1f} key/s", flush=True)
            if cursor in (b"0", 0, "0"):
                break
        elapsed = max(time.time() - started, 0.001)
        print(f"Redis 复制完成：{copied} 个 Key，耗时 {elapsed:.1f}s", flush=True)
    finally:
        source.close()
        destination.close()

if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"Redis 复制失败：{exc}", file=sys.stderr)
        sys.exit(1)

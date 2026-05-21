import socket
import sys

class Redis:
    def __init__(self, host, port):
        self.sock = socket.create_connection((host, int(port)), timeout=30)
        self.file = self.sock.makefile('rb')

    def close(self):
        self.file.close()
        self.sock.close()

    def cmd(self, *parts):
        buf = bytearray()
        buf.extend(b'*%d\r\n' % len(parts))
        for part in parts:
            if isinstance(part, str):
                part = part.encode()
            elif isinstance(part, int):
                part = str(part).encode()
            elif not isinstance(part, (bytes, bytearray)):
                raise TypeError(type(part))
            buf.extend(b'$%d\r\n' % len(part))
            buf.extend(part)
            buf.extend(b'\r\n')
        self.sock.sendall(buf)
        return self._read()

    def _readline(self):
        line = self.file.readline()
        if not line:
            raise EOFError('redis connection closed')
        if not line.endswith(b'\r\n'):
            raise ValueError('bad line ending')
        return line[:-2]

    def _read(self):
        prefix = self.file.read(1)
        if not prefix:
            raise EOFError('redis connection closed')
        if prefix == b'+':
            return self._readline()
        if prefix == b'-':
            raise RuntimeError(self._readline().decode(errors='replace'))
        if prefix == b':':
            return int(self._readline())
        if prefix == b'$':
            n = int(self._readline())
            if n == -1:
                return None
            data = self.file.read(n)
            crlf = self.file.read(2)
            if crlf != b'\r\n':
                raise ValueError('bad bulk ending')
            return data
        if prefix == b'*':
            n = int(self._readline())
            if n == -1:
                return None
            return [self._read() for _ in range(n)]
        raise ValueError(f'bad redis prefix {prefix!r}')

def expect_ok(resp, op):
    if resp != b'OK':
        raise RuntimeError(f'{op} expected OK, got {resp!r}')

def main():
    if len(sys.argv) != 7:
        print('usage: redis_migrate_resp.py src_host src_port src_password dst_host dst_port dst_password', file=sys.stderr)
        return 2
    src_host, src_port, src_pass, dst_host, dst_port, dst_pass = sys.argv[1:]
    src = Redis(src_host, src_port)
    dst = Redis(dst_host, dst_port)
    try:
        expect_ok(src.cmd('AUTH', src_pass), 'source AUTH')
        expect_ok(dst.cmd('AUTH', dst_pass), 'target AUTH')
        expect_ok(src.cmd('SELECT', 1), 'source SELECT')
        expect_ok(dst.cmd('SELECT', 1), 'target SELECT')
        expect_ok(dst.cmd('FLUSHDB'), 'target FLUSHDB')
        cursor = b'0'
        restored = 0
        skipped = 0
        while True:
            reply = src.cmd('SCAN', cursor, 'COUNT', 200)
            cursor = reply[0]
            keys = reply[1]
            for key in keys:
                ttl = src.cmd('PTTL', key)
                if ttl == -2:
                    skipped += 1
                    continue
                dump = src.cmd('DUMP', key)
                if dump is None:
                    skipped += 1
                    continue
                restore_ttl = ttl if ttl > 0 else 0
                expect_ok(dst.cmd('RESTORE', key, restore_ttl, dump, 'REPLACE'), 'target RESTORE')
                restored += 1
            if cursor == b'0':
                break
        src_count = src.cmd('DBSIZE')
        dst_count = dst.cmd('DBSIZE')
        print(f'restored={restored} skipped={skipped} source_dbsize={src_count} target_dbsize={dst_count}')
        return 0 if dst_count == src_count else 1
    finally:
        src.close()
        dst.close()

raise SystemExit(main())

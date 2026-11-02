"""Relay the isolated phone TCP connection and optionally delay one boss start."""
import argparse
import asyncio
import contextlib
import datetime
import ipaddress
import json
import os
from pathlib import Path
import signal
import stat
import struct
import sys


ROOT = Path(__file__).resolve().parents[1]
PRIVATE_ROOT = ROOT / '.local-env/performance117-ui'
LOG_PATH = ROOT / 'analysis/round-two-performance-final/phone-tcp.jsonl'
LISTEN = ('127.0.0.1', 18863)
UPSTREAM = ('127.0.0.1', 18862)
MAX_PACKET = 4 * 1024 * 1024  # hyjs-server/constUtil/game.go
MAX_RECORDS = 5000
MAX_SECONDS = 20 * 60
DELAY_SECONDS = 18
BEIJING = datetime.timezone(datetime.timedelta(hours=8))


def check_endpoints():
    for host, port in (LISTEN, UPSTREAM):
        if not ipaddress.ip_address(host).is_loopback or host != '127.0.0.1':
            raise ValueError('Both endpoints must be IPv4 loopback')
        if not 1 <= port <= 65535:
            raise ValueError('Invalid fixed endpoint port')


def flag_path(value):
    private = PRIVATE_ROOT.resolve(strict=True)
    if not private.is_dir() or not private.is_relative_to(ROOT.resolve()):
        raise ValueError('The project-owned performance117 private directory is required')
    supplied = Path(value).expanduser()
    if not supplied.is_absolute():
        supplied = ROOT / supplied
    parent = supplied.parent.resolve(strict=True)
    if not parent.is_relative_to(private) or supplied.name in ('', '.', '..'):
        raise ValueError('Delay flag must be a file under .local-env/performance117-ui')
    result = parent / supplied.name
    try:
        info = result.lstat()
    except FileNotFoundError:
        return result
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
        raise ValueError('An existing delay flag must be an owned regular file')
    return result


def boss_start_accepted(payload):
    """Read only top-level protobuf fields and require the final code to be 1."""
    def read_varint(offset):
        value = 0
        for index in range(10):
            if offset >= len(payload):
                raise ValueError('Truncated varint')
            byte = payload[offset]
            offset += 1
            if index == 9 and byte > 1:
                raise ValueError('Varint overflows uint64')
            value |= (byte & 0x7f) << (7 * index)
            if not byte & 0x80:
                return value, offset
        raise ValueError('Varint exceeds ten bytes')

    offset = 0
    code = None
    try:
        while offset < len(payload):
            tag, offset = read_varint(offset)
            field, wire = tag >> 3, tag & 7
            if field == 0 or field > (1 << 29) - 1:
                return False
            if wire == 0:
                value, offset = read_varint(offset)
                if field == 6:
                    code = value
            elif wire == 1:
                offset += 8
            elif wire == 2:
                length, offset = read_varint(offset)
                offset += length
            elif wire == 5:
                offset += 4
            else:
                return False
            if offset > len(payload):
                return False
    except ValueError:
        return False
    return code == 1


class MetadataLog:
    def __init__(self):
        LOG_PATH.parent.mkdir(parents=True, exist_ok=True)
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0)
        self.fd = os.open(LOG_PATH, flags, 0o600)
        self.count = 0
        self.failed = False

    def write(self, direction, opcode, size, delay_ms):
        if self.count >= MAX_RECORDS or self.failed:
            return
        row = {'time': datetime.datetime.now(BEIJING).isoformat(timespec='milliseconds'),
               'direction': direction, 'opcode': opcode, 'bytes': size, 'delay_ms': delay_ms}
        data = (json.dumps(row, separators=(',', ':')) + '\n').encode('utf-8')
        try:
            while data:
                written = os.write(self.fd, data)
                if written <= 0:
                    raise OSError('metadata write failed')
                data = data[written:]
            self.count += 1
        except OSError:
            self.failed = True
            print('Metadata logging stopped; frame forwarding continues.', file=sys.stderr, flush=True)

    def close(self):
        os.close(self.fd)


class Proxy:
    def __init__(self, flag, metadata):
        self.flag = flag
        self.metadata = metadata
        self.delayed = False
        self.stopping = False
        self.handlers = set()

    def consume_flag(self):
        if self.delayed:
            return False
        try:
            # Revalidate at use time; the owner may create the flag after startup.
            checked = flag_path(self.flag)
            info = checked.lstat()
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
                raise ValueError('Delay flag is not an owned regular file')
            checked.unlink()
            self.delayed = True
            return True
        except FileNotFoundError:
            return False
        except (OSError, ValueError):
            print('Delay flag could not be safely consumed; forwarding continues.',
                  file=sys.stderr, flush=True)
            return False

    async def relay(self, reader, writer, direction):
        while True:
            try:
                header = await reader.readexactly(4 if direction == 'client_to_server' else 8)
            except asyncio.IncompleteReadError as error:
                if error.partial:
                    raise
                if writer.can_write_eof():
                    writer.write_eof()
                    await writer.drain()
                else:
                    writer.close()
                return
            if direction == 'client_to_server':
                size = struct.unpack('>I', header)[0]
                if size < 8 or size > MAX_PACKET - 4:
                    raise ValueError('Invalid client frame size')
                payload = await reader.readexactly(size)
                frame = header + payload
                opcode = struct.unpack('>I', frame[8:12])[0]
            else:
                size, opcode = struct.unpack('>II', header)
                if size > MAX_PACKET - 8:
                    raise ValueError('Invalid server frame size')
                payload = await reader.readexactly(size)
                frame = header + payload
            delay_ms = 0
            if (direction == 'server_to_client' and opcode == 30084 and
                    boss_start_accepted(payload) and self.consume_flag()):
                started = asyncio.get_running_loop().time()
                await asyncio.sleep(DELAY_SECONDS)
                delay_ms = round((asyncio.get_running_loop().time() - started) * 1000)
            writer.write(frame)
            await writer.drain()
            self.metadata.write(direction, opcode, len(frame), delay_ms)

    def connected(self, reader, writer):
        if self.stopping:
            writer.close()
            return
        task = asyncio.create_task(self.handle(reader, writer))
        self.handlers.add(task)
        task.add_done_callback(self.handlers.discard)

    async def handle(self, reader, writer):
        upstream_writer = None
        relays = []
        try:
            upstream_reader, upstream_writer = await asyncio.open_connection(*UPSTREAM)
            peer = upstream_writer.get_extra_info('peername')
            if not peer or peer[0] != UPSTREAM[0] or peer[1] != UPSTREAM[1]:
                raise ValueError('Unexpected upstream endpoint')
            relays = [asyncio.create_task(self.relay(reader, upstream_writer, 'client_to_server')),
                      asyncio.create_task(self.relay(upstream_reader, writer, 'server_to_client'))]
            await asyncio.gather(*relays)
        except (OSError, ValueError, asyncio.IncompleteReadError):
            pass
        finally:
            for task in relays:
                if not task.done():
                    task.cancel()
            if relays:
                await asyncio.gather(*relays, return_exceptions=True)
            for connection in (upstream_writer, writer):
                if connection is not None:
                    connection.close()
                    with contextlib.suppress(Exception, asyncio.CancelledError):
                        await asyncio.wait_for(connection.wait_closed(), timeout=1)


async def serve(flag):
    loop = asyncio.get_running_loop()
    deadline = loop.time() + MAX_SECONDS
    stopped = asyncio.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, stopped.set)
    timer = loop.call_at(deadline, stopped.set)
    metadata = None
    server = None
    proxy = None
    try:
        metadata = MetadataLog()  # O_EXCL requires a fresh evidence file.
        proxy = Proxy(flag, metadata)
        server = await asyncio.start_server(proxy.connected, *LISTEN)
        if not server.sockets or any(sock.getsockname()[:2] != LISTEN for sock in server.sockets):
            raise RuntimeError('Listener is not bound to the expected loopback endpoint')
        print('TCP QA proxy ready on 127.0.0.1:18863 -> 127.0.0.1:18862', flush=True)
        await stopped.wait()
    finally:
        timer.cancel()
        if proxy is not None:
            proxy.stopping = True
        if server is not None:
            server.close()
            await server.wait_closed()
        if proxy is not None:
            tasks = list(proxy.handlers)
            for task in tasks:
                task.cancel()
            if tasks:
                await asyncio.gather(*tasks, return_exceptions=True)
        if metadata is not None:
            metadata.close()
        for sig in (signal.SIGINT, signal.SIGTERM):
            loop.remove_signal_handler(sig)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--delay-flag', type=Path, required=True,
                        help='owned flag under project .local-env/performance117-ui')
    args = parser.parse_args()
    try:
        check_endpoints()
        flag = flag_path(args.delay_flag)
        asyncio.run(serve(flag))
    except (OSError, ValueError, RuntimeError) as error:
        print('TCP QA proxy could not start: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

"""Client-derived protobuf contracts and transport framing.

Evidence: analysis/protocol_methods.il.txt, PacketFactory methods.
The HTTP and TCP response prefixes both contain protobuf body length;
the TCP request prefix instead excludes its own four-byte length field.
"""
from dataclasses import dataclass
from pathlib import Path
import hashlib
import hmac
import json
import struct

from google.protobuf import descriptor_pb2, descriptor_pool, message_factory

SCHEMA = json.loads((Path(__file__).parent / "protocol/schema.json").read_text())
OPCODES = {int(k):v for k,v in SCHEMA["opcodes"].items()}
BY_NAME = {v:k for k,v in OPCODES.items()}
MAX_PACKET = 4 * 1024 * 1024
# Client protocol constant, recovered from PacketFactory.GetSig.
SIGNING_SUFFIX = "sao2016@wyht"
EMPTY_SESSION = b" " + b"\0" * 15


def _classes():
    file = descriptor_pb2.FileDescriptorProto(name="game.proto", package="hyjs", syntax="proto2")
    field_type = descriptor_pb2.FieldDescriptorProto
    for name, definition in SCHEMA["messages"].items():
        message = file.message_type.add(name=name)
        for spec in definition["fields"]:
            field = message.field.add(name=spec["name"], number=spec["number"])
            field.label = (field_type.LABEL_REPEATED if spec.get("repeated") else
                           field_type.LABEL_REQUIRED if spec["required"] else field_type.LABEL_OPTIONAL)
            scalar = getattr(field_type, "TYPE_" + spec["type"].upper(), None)
            if scalar is None:
                field.type = field_type.TYPE_MESSAGE
                field.type_name = ".hyjs." + spec["type"]
            else:
                field.type = scalar
            if spec["packed"]:
                field.options.packed = True
    pool = descriptor_pool.DescriptorPool()
    pool.Add(file)
    return {name:message_factory.GetMessageClass(pool.FindMessageTypeByName("hyjs." + name))
            for name in SCHEMA["messages"]}


CLASSES = _classes()


def message(name, **fields):
    return CLASSES[name](**fields)


def signature(player_id, zone_id, timestamp, payload):
    prefix = f"{player_id}{zone_id}{timestamp}{SIGNING_SUFFIX}".encode("ascii")
    return hashlib.md5(prefix + payload).digest()


@dataclass(frozen=True)
class Request:
    opcode: int
    player_id: int
    zone_id: int
    session: bytes
    payload: bytes
    timestamp: int = 0

    def decode(self):
        name = OPCODES.get(self.opcode)
        if name is None:
            raise ValueError(f"Unknown opcode {self.opcode}")
        result = CLASSES[name].FromString(self.payload)
        if not result.IsInitialized():
            raise ValueError("Missing required protobuf fields")
        return result


def parse_http(raw):
    if not 68 <= len(raw) <= MAX_PACKET:
        raise ValueError("Invalid HTTP packet size")
    length, version, opcode, player_id, zone_id, size = struct.unpack_from(">iiiqii", raw)
    if version != 10000 or length != len(raw) or size != len(raw)-68:
        raise ValueError("HTTP header length/version mismatch")
    session, sig = raw[28:44], raw[44:60]
    timestamp = struct.unpack_from(">q", raw, 60)[0]
    body = raw[68:]
    if not hmac.compare_digest(sig, signature(player_id, zone_id, timestamp, body)):
        raise ValueError("Invalid client packet signature")
    return Request(opcode, player_id, zone_id, session, body, timestamp)


def encode_http_request(opcode, body, *, player_id=0, zone_id=0, session=EMPTY_SESSION, timestamp=0):
    payload = body.SerializeToString()
    if len(session) != 16:
        raise ValueError("HTTP session key must contain 16 bytes")
    return (struct.pack(">iiiqii", 68+len(payload), 10000, opcode, player_id, zone_id, len(payload))
            + session + signature(player_id, zone_id, timestamp, payload)
            + struct.pack(">q", timestamp) + payload)


def parse_tcp(raw):
    if not 28 <= len(raw) <= MAX_PACKET:
        raise ValueError("Invalid TCP packet size")
    length, version, opcode, zone_id, player_id, size = struct.unpack_from(">iiiiqi", raw)
    if version != 1 or length != len(raw)-4 or size != len(raw)-28:
        raise ValueError("TCP header length/version mismatch")
    return Request(opcode, player_id, zone_id, b"", raw[28:])


def encode_response(opcode, body):
    payload = body.SerializeToString()
    return struct.pack(">ii", len(payload), opcode) + payload


def decode_responses(raw):
    """HTTP may concatenate several response frames in a single body."""
    result = []
    offset = 0
    while offset < len(raw):
        if len(raw)-offset < 8:
            raise ValueError("Truncated response header")
        size, opcode = struct.unpack_from(">ii", raw, offset)
        if size < 0 or size > MAX_PACKET or offset+8+size > len(raw):
            raise ValueError("Truncated or invalid response payload")
        name = OPCODES.get(opcode)
        if name is None:
            raise ValueError(f"Unknown response opcode {opcode}")
        payload = CLASSES[name].FromString(raw[offset+8:offset+8+size])
        if not payload.IsInitialized():
            raise ValueError("Missing required response field")
        result.append((opcode, payload))
        offset += 8+size
    return result

"""Opt-in local correction of the original RCAISelf planar movement gate.

The shipped gate checks world Vector3.x/y although joystick movement is in x/z.
Only the y field operand becomes z. Exact method hashes and original metadata
identities must match; any drift or second application is rejected.
"""
import argparse
import hashlib
import json
from pathlib import Path
import struct

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ORIGINAL_MVID = '22d2d1ef-5802-443d-9c1e-315513506718'
ORIGINAL_ARTIFACT_SHA = 'ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f'
TICK_SHA = 'b19e48c645391575c6d1bd7087914c04974528cc894730c4dd6b9fbf46fa26bd'
JOYSTICK_SHA = 'a49004fc06b68a14c87f4a2697a8ed6246dd01ef5053b13c0c7f9ac766c33d22'
MOVE_FIELD, PRESSING_FIELD = 0x040038DF, 0x040038DD
VECTOR_X, VECTOR_Y, VECTOR_Z = 0x0A000021, 0x0A000022, 0x0A000027


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def method_body(pe, row):
    body = read_method_body_from_bytes(pe.get_data(row.Rva))
    code = pe.get_data(row.Rva + body.header_size, body.code_size)
    return body, code


def execute_original_gate(body, pressing, vector):
    """Interpret the actual retained CIL branch slice; no Unity game runs."""
    instructions = {i.offset: i for i in body.instructions}
    successors = {left.offset: right.offset for left, right in zip(body.instructions, body.instructions[1:])}
    fields = {MOVE_FIELD: vector, PRESSING_FIELD: pressing,
              VECTOR_X: vector[0], VECTOR_Y: vector[1], VECTOR_Z: vector[2]}
    stack, pc = [], 0x2B
    for _ in range(32):
        if pc == 0x60:
            return 'move'
        if pc == 0xF3:
            return 'stop'
        instruction = instructions[pc]
        op, operand = instruction.opcode.name, instruction.operand
        target = successors[pc]
        if op == 'ldarg.0':
            stack.append('self')
        elif op in ('ldfld', 'ldflda'):
            stack.pop()
            stack.append(fields[operand.value])
        elif op == 'ldc.r4':
            stack.append(operand)
        elif op == 'brfalse':
            if not stack.pop():
                target = operand
        elif op in ('bne.un', 'beq'):
            right, left = stack.pop(), stack.pop()
            if (left != right) if op == 'bne.un' else (left == right):
                target = operand
        else:
            raise ValueError('Unexpected opcode in original planar gate: ' + op)
        pc = target
    raise ValueError('Original gate did not terminate within its declared slice')


def verify_gate_cases(original, revised):
    cases = [
        ('pure-positive-world-z', True, (0.0, 0.0, 0.3), 'stop', 'move'),
        ('pure-negative-world-z', True, (0.0, 0.0, -0.3), 'stop', 'move'),
        ('world-x', True, (0.3, 0.0, 0.0), 'move', 'move'),
        ('world-xz-diagonal', True, (0.2, 0.0, 0.2), 'move', 'move'),
        ('zero-vector', True, (0.0, 0.0, 0.0), 'stop', 'stop'),
        ('released-finger', False, (0.2, 0.0, 0.2), 'stop', 'stop'),
        ('negative-zero-world-x', True, (-0.0, 0.0, 0.3), 'stop', 'move'),
    ]
    results = []
    for name, pressing, vector, before, after in cases:
        actual_before = execute_original_gate(original, pressing, vector)
        actual_after = execute_original_gate(revised, pressing, vector)
        require((actual_before, actual_after) == (before, after), 'Planar gate regression: ' + name)
        results.append({'name': name, 'original': actual_before, 'corrected': actual_after})
    return results


def patch(raw):
    pe = dnfile.dnPE(data=raw)
    require(pe.net is not None, 'Managed assembly required')
    module = pe.net.mdtables.Module.rows[0]
    require(str(module.Name) == 'Assembly-CSharp.dll' and str(module.Mvid) == ORIGINAL_MVID,
            'Original assembly name/MVID differs')
    owners = [t for t in pe.net.mdtables.TypeDef if str(t.TypeNamespace) == 'Room' and str(t.TypeName) == 'RCAISelf']
    require(len(owners) == 1, 'Original RCAISelf identity differs')
    owner = owners[0]
    methods = {str(m.row.Name): m for m in owner.MethodList}
    tick, tick_code = method_body(pe, methods['Tick'].row)
    joystick, joystick_code = method_body(pe, methods['OnJoystickMove'].row)
    require(digest(tick_code) == TICK_SHA and digest(joystick_code) == JOYSTICK_SHA,
            'Original Tick/OnJoystickMove hash differs; patch is unsupported or already applied')
    require(tick.header_size == 12 and tick.code_size == 589 and not tick.exception_handlers,
            'Original Tick header/code/EH differs')
    move = next(f for f in owner.FieldList if str(f.row.Name) == 'moveDirection')
    require(0x04000000 | move.row_index == MOVE_FIELD and move.row.Signature.value == bytes.fromhex('061109'),
            'Original world Vector3 moveDirection field differs')
    vector_ref = pe.net.mdtables.TypeRef.rows[1]
    require((str(vector_ref.TypeNamespace), str(vector_ref.TypeName)) == ('UnityEngine', 'Vector3'),
            'Original moveDirection signature no longer resolves to UnityEngine.Vector3')
    for token, name in ((VECTOR_X, 'x'), (VECTOR_Y, 'y'), (VECTOR_Z, 'z')):
        member = pe.net.mdtables.MemberRef.rows[(token & 0xFFFFFF) - 1]
        parent = member.Class.row
        require((str(parent.TypeNamespace), str(parent.TypeName), str(member.Name), member.Signature.value) ==
                ('UnityEngine', 'Vector3', name, bytes.fromhex('060c')), 'Original Vector3 field reference differs')
    instructions = {i.offset: i for i in tick.instructions}
    expected = {0x37: ('ldflda', MOVE_FIELD), 0x3C: ('ldfld', VECTOR_X),
                0x4C: ('ldflda', MOVE_FIELD), 0x51: ('ldfld', VECTOR_Y)}
    for offset, (opcode, operand) in expected.items():
        instruction = instructions[offset]
        require(instruction.opcode.name == opcode and instruction.operand.value == operand,
                'Original movement gate shape differs')
    require(instructions[0x46].opcode.name == 'bne.un' and instructions[0x46].operand == 0x60 and
            instructions[0x5B].opcode.name == 'beq' and instructions[0x5B].operand == 0xF3,
            'Original movement gate control flow differs')
    require(not any(i.opcode.name in ('ldfld', 'ldflda') and getattr(i.operand, 'value', None) == VECTOR_Z
                    for i in tick.instructions), 'Original Tick already reads world z')
    body_offset = pe.get_offset_from_rva(methods['Tick'].row.Rva)
    operand_offset = body_offset + 0x52
    require(raw[operand_offset:operand_offset + 4] == struct.pack('<I', VECTOR_Y), 'Original operand bytes differ')
    revised = bytearray(raw)
    revised[operand_offset:operand_offset + 4] = struct.pack('<I', VECTOR_Z)
    revised = bytes(revised)
    differences = [i for i, (before, after) in enumerate(zip(raw, revised)) if before != after]
    require(len(revised) == len(raw) and differences == [operand_offset], 'Patch must change exactly one operand byte')
    # Every byte outside this single Room method operand stays identical,
    # including metadata, code headers, exception tables and other methods.
    engine_methods = []
    for original_owner in pe.net.mdtables.TypeDef:
        if not str(original_owner.TypeNamespace).startswith('Battle.Engine'):
            continue
        for entry in original_owner.MethodList:
            if not entry.row.Rva:
                continue
            body, code = method_body(pe, entry.row)
            offset = pe.get_offset_from_rva(entry.row.Rva) + body.header_size
            require(revised[offset:offset + body.code_size] == code, 'Original battle engine instructions changed')
            engine_methods.append((str(original_owner.TypeNamespace) + '.' + str(original_owner.TypeName),
                                   str(entry.row.Name), digest(code)))
    require(len(engine_methods) == 547, 'Original engine method inventory differs')
    revised_body = read_method_body_from_bytes(revised[body_offset:])
    revised_code = revised[body_offset + tick.header_size:body_offset + tick.header_size + tick.code_size]
    gate_cases = verify_gate_cases(tick, revised_body)
    proof = {
        'adapter': 'local-planar-joystick-xz-gate-v1',
        'originalReferenceSHA256': ORIGINAL_ARTIFACT_SHA,
        'inputSHA256': digest(raw), 'outputSHA256': digest(revised),
        'mvid': ORIGINAL_MVID, 'method': 'Room.RCAISelf.Tick',
        'methodToken': hex(0x06000000 | methods['Tick'].row_index),
        'originalMethodILSHA256': TICK_SHA, 'correctedMethodILSHA256': digest(revised_code),
        'unchangedOnJoystickMoveILSHA256': JOYSTICK_SHA,
        'fieldOperandBodyOffset': '0x52', 'fieldOperandFileOffset': operand_offset,
        'originalOperand': 'UnityEngine.Vector3.y (0x0a000022)',
        'correctedOperand': 'UnityEngine.Vector3.z (0x0a000027)',
        'changedBytes': 1, 'otherBytesAndMetadataAndHeadersAndEHUnchanged': True,
        'originalEngineMethods': len(engine_methods), 'originalEngineInstructionsAndEHUnchanged': True,
        'originalEngineMethodDigest': digest(json.dumps(engine_methods, separators=(',', ':')).encode()),
        'gateCILCases': gate_cases, 'unityExecuted': False, 'deviceExecuted': False,
    }
    return revised, proof


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--proof', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists() or args.proof.exists() or args.output.resolve() == args.input.resolve():
        raise ValueError('Use new output/proof paths; preserve original inputs and previous evidence')
    revised, proof = patch(args.input.read_bytes())
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.proof.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_bytes(revised)
    args.proof.write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


if __name__ == '__main__':
    main()

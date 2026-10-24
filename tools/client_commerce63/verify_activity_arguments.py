"""Verify actual compiled reflection argument arity against the pinned native MethodDef."""
import argparse
import hashlib
import json
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes


def verify(main_path, helper_path):
    main, helper = [dnfile.dnPE(str(p)) for p in (main_path, helper_path)]
    def methods(pe, typename):
        owner = next(t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeName) == typename)
        return {str(m.row.Name): m.row for m in owner.MethodList}
    def code(pe, method):
        return list(read_method_body_from_bytes(pe.get_data(method.Rva, 65536)).instructions)
    def member(pe, instruction, table):
        token = getattr(instruction.operand, 'value', 0)
        return getattr(pe.net.mdtables, table).rows[(token & 0xffffff)-1]
    original = methods(main, 'UIActivityEveryDaySign')['ShowOrHideRedPoint']
    # ECMA signature: instance, one parameter, void result, System.Object argument.
    if original.Signature.value != bytes.fromhex('2001011c'):
        raise ValueError('Native red-point signature changed')
    original_code = code(main, original)
    if any(i.opcode.name in ('ldarg.1', 'ldarga.1', 'starg.1', 'ldarg', 'ldarg.s', 'ldarga', 'ldarga.s', 'starg', 'starg.s') for i in original_code):
        raise ValueError('Native callback starts consuming its object argument')
    calls = [str(member(main, i, 'MethodDef' if i.operand.value >> 24 == 6 else 'MemberRef').Name) for i in original_code if i.opcode.name in ('call','callvirt')]
    if calls.count('GetStateByType') != 3 or calls.count('SetActive') != 3 or calls.count('op_Equality') != 1 or len(calls) != 7:
        raise ValueError('Native callback has side effects beyond flag projection')
    compiled = methods(helper, 'DailySignPresentation')
    init = code(helper, compiled['.cctor'])
    stores = [n for n,i in enumerate(init) if i.opcode.name == 'stsfld' and str(member(helper,i,'Field').Name) == 'RedPointArguments']
    if len(stores) != 1:
        raise ValueError('Expected one callback argument-array initialization')
    index = stores[0]
    if init[index-2].opcode.name != 'ldc.i4.1' or init[index-1].opcode.name != 'newarr' or str(member(helper,init[index-1],'TypeRef').TypeName) != 'Object':
        raise ValueError('Compiled callback argument array must contain exactly one object')
    late = code(helper, compiled['LateUpdate'])
    uses = [n for n,i in enumerate(late) if i.opcode.name == 'ldsfld' and i.operand.value >> 24 == 4 and str(member(helper,i,'Field').Name) == 'RedPointArguments']
    if len(uses) != 1 or late[uses[0]+1].opcode.name != 'callvirt' or str(member(helper,late[uses[0]+1],'MemberRef').Name) != 'Invoke':
        raise ValueError('Reflection call no longer uses the verified argument array')
    return {'nativeMethod':'UI.UIActivityEveryDaySign::ShowOrHideRedPoint(System.Object)', 'nativeSignatureHex':original.Signature.value.hex(), 'nativeParameterCount':1, 'compiledArgumentArrayLength':1, 'nativeArgumentUnused':True, 'nativeOnlyReadsThreeFlagsAndSetsVisibility':True, 'compiledInvokeUsesVerifiedArray':True, 'mainSHA256':hashlib.sha256(Path(main_path).read_bytes()).hexdigest(), 'helperSHA256':hashlib.sha256(Path(helper_path).read_bytes()).hexdigest(), 'UnityExecuted':False}

if __name__ == '__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--main',type=Path,required=True)
    parser.add_argument('--helper',type=Path,required=True)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    report=verify(args.main,args.helper)
    args.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(report,ensure_ascii=False))

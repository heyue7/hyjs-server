"""Execute the emitted sign callback CIL with explicit UI/network stubs."""
import argparse
import json
from pathlib import Path
import sys
import dnfile
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded


def symbol(pe, token):
    table = {4: "Field", 6: "MethodDef", 10: "MemberRef", 2: "TypeDef"}[token >> 24]
    row = getattr(pe.net.mdtables, table).rows[(token & 0xffffff)-1]
    return str(row.TypeName if table == "TypeDef" else row.Name)


def run(pe, code, success, vip, other, luxury):
    points = {47: int(other) if luxury else 1, 48: 1 if luxury else int(other), 15: 1}
    page = {"currentDay": 1, "datas": [1], "signCount": {}, "superState": 1}
    actor = page if luxury else {"sign": page, "vipLevel": vip, "item": {"doubleNeedVipLevel": 2}, "evt": {"onClick": []}, "getMask": {}}
    args = [actor, {"kBody": {"get": {}}}, success]
    stack, local, events = [], {}, []
    pc = 0
    for _ in range(250):
        op, value = code[pc]; pc += 1
        if op.startswith("ldarg."): stack.append(args[int(op[-1])])
        elif op.startswith("stloc."): local[int(op[-1])] = stack.pop()
        elif op.startswith("ldloc."): stack.append(local[int(op[-1])])
        elif op in ("ldstr", "ldc.r4", "ldc.i4", "ldc.i4.s"): stack.append(value)
        elif op.startswith("ldc.i4."): stack.append(int(op[-1]))
        elif op == "ldsfld": stack.append(points if symbol(pe, value) == "redTips" else {})
        elif op == "ldfld": stack.append(stack.pop()[symbol(pe, value)])
        elif op == "stfld":
            item = stack.pop(); stack.pop()[symbol(pe, value)] = item
        elif op in ("box", "castclass"): pass
        elif op == "ldftn": stack.append(value)
        elif op in ("br", "br.s"): pc = value
        elif op in ("brfalse", "brfalse.s", "brtrue", "brtrue.s"):
            truth = bool(stack.pop())
            if truth == op.startswith("brtrue"): pc = value
        elif op in ("bge", "ble", "bne.un"):
            right, left = stack.pop(), stack.pop()
            if {"bge": left >= right, "ble": left <= right, "bne.un": left != right}[op]: pc = value
        elif op == "newobj":
            # Only the original two delegate constructors accept operands.
            if len(stack) >= 2 and isinstance(stack[-1], int) and stack[-1] >> 24 == 6:
                stack.pop(); stack.pop(); stack.append({"delegate": True})
            elif stack and isinstance(stack[-1], dict) and stack[-1].get("delegate"):
                stack.pop(); stack.append({"event": True})
            else: stack.append({"request": "CSRedPoint"})
        elif op in ("call", "callvirt"):
            name = symbol(pe, value)
            if name in ("get_get", "get_doubleNeedVipLevel"):
                stack.append(stack.pop()[name[4:]])
            elif name == "get_Count": stack.append(len(stack.pop()))
            elif name == "Concat":
                third, second, first = stack.pop(), stack.pop(), stack.pop(); stack.append(str(first)+str(second)+str(third))
            elif name == "set_text": stack.pop(); stack.pop()
            elif name == "AddResultObj": stack.pop(); stack.pop(); events.append("apply-original-gain")
            elif name == "GetText": stack.pop(); stack.append("签到成功")
            elif name == "CreatePopupTextBox": stack.pop(); stack.pop(); events.append("original-success-notice")
            elif name == "set_EffectActived": stack.pop(); stack.pop()
            elif name == "SetSuperButtonState": stack.pop()
            elif name == "SetPointState":
                state, key, target = stack.pop(), stack.pop(), stack.pop(); target[key] = state; events.append("clear-point-"+str(key))
            elif name == "GetStateByType":
                key, target = stack.pop(), stack.pop(); stack.append(target[key] == 1)
            elif name == "ReduceActivitys": stack.pop(); events.append("reduce-original-activity-count")
            elif name == "SetActive": stack.pop(); stack.pop()
            elif name == "Clear": stack.pop().clear()
            elif name == "Add": item, target = stack.pop(), stack.pop(); target.append(item)
            elif name == "Destroy": stack.pop()
            elif name == "SendSocket":
                assert stack.pop() == {"request": "CSRedPoint"} and stack.pop() == 20117
                events.append("query-authoritative-redpoints")
            else: raise AssertionError("Unmodeled call "+name)
        elif op == "ret":
            assert not stack, stack
            return events
        else: raise AssertionError("Unmodeled opcode "+op)
    raise AssertionError("callback loop")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--managed", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    pe = dnfile.dnPE(str(args.managed))
    names = ["UI.UIActivitySignIcon::cb", "UI.UIActivityEveryDaySign::<OnClickSuper>m__1D6"]
    bodies = {name: decoded(pe, method)[1] for name, rid, method in methods(pe) if name in names}
    cases = []
    for luxury, name in enumerate(names):
        for success in (False, True):
            for other in (False, True):
                for vip in (0, 2):
                    events = run(pe, bodies[name], success, vip, other, bool(luxury))
                    assert events.count("query-authoritative-redpoints") == int(success)
                    if success:
                        assert events[-1] == "query-authoritative-redpoints"
                        assert events.index("clear-point-"+str(48 if luxury else 47)) < len(events)-1
                        assert ("clear-point-15" in events) == (not other)
                    else: assert events == []
                    cases.append(dict(callback=name, success=success, otherSignReady=other, vip=vip, events=events, passed=True))
    report = dict(scope="actual emitted CIL with explicit UI/network stubs; no Unity or phone execution", cases=cases, passed=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")
    print(json.dumps(dict(cases=len(cases), passed=True)))

if __name__ == "__main__": main()

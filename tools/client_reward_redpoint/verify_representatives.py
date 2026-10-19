"""Execute the emitted reward callback CIL with explicit API stubs, without Unity/device claims."""
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
    table = {4: 'Field', 6: 'MethodDef', 10: 'MemberRef', 2: 'TypeDef'}[token >> 24]
    row = getattr(pe.net.mdtables, table).rows[(token & 0xffffff) - 1]
    return str(row.TypeName if table == 'TypeDef' else row.Name)


def run(pe, code, args):
    stack, events, locals_ = [], [], {}
    pc = 0
    for _ in range(150):
        op, arg = code[pc]
        pc += 1
        if op.startswith('ldarg.'):
            stack.append(args[int(op[-1])])
        elif op.startswith('stloc.'):
            locals_[int(op[-1])] = stack.pop()
        elif op.startswith('ldloc.'):
            stack.append(locals_[int(op[-1])])
        elif op in ('ldstr', 'ldc.r4', 'ldc.i4'):
            stack.append(arg)
        elif op.startswith('ldc.i4.'):
            stack.append(int(op[-1]))
        elif op == 'ldsfld':
            stack.append({'static': symbol(pe, arg)})
        elif op == 'ldfld':
            stack.append(stack.pop()[symbol(pe, arg)])
        elif op == 'isinst':
            value = stack.pop()
            stack.append(value if value.get('_type') == symbol(pe, arg) else None)
        elif op in ('brfalse', 'brfalse.s'):
            if not stack.pop():
                pc = arg
        elif op == 'newobj':
            assert symbol(pe, arg) == '.ctor'
            stack.append({'_type': 'CSRedPoint'})
        elif op in ('call', 'callvirt'):
            name = symbol(pe, arg)
            if name == 'get_result':
                stack.append(stack.pop()['result'])
            elif name == 'AddResultObj':
                stack.pop()
                stack.pop()
                events.append('apply-original-gain')
            elif name == 'GetText':
                assert stack.pop() == 'get_success'
                stack.append('领取成功')
            elif name == 'CreatePopupTextBox':
                stack.pop()
                stack.pop()
                events.append('original-success-notice')
            elif name == 'Clear':
                stack.pop().clear()
                events.append('clear-visible-list')
            elif name == 'RemoveAt':
                index = stack.pop()
                stack.pop().pop(index)
                events.append('remove-one-visible-item')
            elif name == 'UpdateScrollView':
                items = stack.pop()
                stack.pop()
                events.append('refresh-visible-list')
                if not items:
                    events.append('original-clear-redpoint')
            elif name == 'SetPointState':
                assert stack.pop() == 0 and stack.pop() == 0
                stack.pop()
                events.append('original-clear-redpoint')
            elif name == 'SendSocket':
                assert stack.pop() == {'_type': 'CSRedPoint'} and stack.pop() == 20117
                events.append('query-authoritative-redpoints')
            else:
                raise AssertionError('Unmodeled call ' + name)
        elif op == 'ret':
            assert not stack, stack
            return events
        else:
            raise AssertionError('Unmodeled opcode ' + op)
    raise AssertionError('Unexpected callback loop')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--managed', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    pe = dnfile.dnPE(str(args.managed))
    names = ['UI.RewardCenterPage::GetAllCallback', 'UI.RewardCenterPage::RemoveAndUpdate']
    bodies = {name: decoded(pe, method)[1] for name, rid, method in methods(pe) if name in names}
    results = []
    for success in [False, True]:
        actor = {'rewardItemInfo': [11, 12]}
        packet = {'kBody': {'_type': 'SCAddRewardsToPlayer', 'result': {}}}
        events = run(pe, bodies[names[0]], [actor, packet, success])
        assert events.count('query-authoritative-redpoints') == int(success)
        if success:
            assert events[-2:] == ['original-clear-redpoint', 'query-authoritative-redpoints']
        else:
            assert events == [] and actor['rewardItemInfo'] == [11, 12]
        results.append({'case': 'get-all-success' if success else 'get-all-failed', 'events': events, 'passed': True})
    for items in [[11, 12], [11]]:
        actor = {'rewardItemInfo': list(items)}
        events = run(pe, bodies[names[1]], [actor, 0])
        assert actor['rewardItemInfo'] == items[1:] and events[-1] == 'query-authoritative-redpoints'
        assert events.count('query-authoritative-redpoints') == 1
        results.append({'case': 'single-partial' if len(items) > 1 else 'single-last-visible', 'events': events, 'passed': True})
    report = {'scope': 'actual emitted callback CIL with explicit API stubs; not Unity, network or phone execution', 'cases': results, 'passed': True}
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'cases': len(results), 'passed': True}))


if __name__ == '__main__':
    main()

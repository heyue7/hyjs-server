"""Compile fixed feedback84 and run isolated SQLite/protocol tests without MySQL."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-id', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z0-9-]{1,48}', args.run_id):
        parser.error('Use a short run id')
    out = ROOT / 'analysis/feedback84' / args.run_id
    out.mkdir(parents=True, exist_ok=False)
    env = {k: v for k, v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL','TZ'}}
    env.update(GOMAXPROCS='2', GOMEMLIMIT='768MiB', GOCACHE='/private/tmp/hyjs-feedback84-go-cache',
               HYJS_TEST_FIXTURE_ROOT='/Users/lirui/LR/hyjs/analysis', HYJS_P6_GENERIC_EXPORT_DIR=str(out / 'native-exports'))
    report = {'scope': 'SQLite and signed HTTP; no MySQL allocation', 'results': []}
    def run(label, command, cwd, timeout):
        started = time.monotonic()
        process = subprocess.Popen(['rtk','proxy',*command], cwd=cwd, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
        timed_out = False
        try:
            output, _ = process.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(process.pid, signal.SIGKILL)
            output, _ = process.communicate()
        (out / (label+'.log')).write_text(output)
        row = {'stage':label,'exitCode':process.returncode,'timedOut':timed_out,'elapsedSeconds':round(time.monotonic()-started,3)}
        report['results'].append(row)
        (out/'result.json').write_text(json.dumps(report,indent=2)+'\n')
        print(json.dumps(row),flush=True)
        if process.returncode or timed_out:
            print('\n'.join(output.splitlines()[-60:]),flush=True)
            return False
        return True
    build = ROOT / 'build/feedback84'
    build.mkdir(parents=True,exist_ok=True)
    if not run('compile-service',['go','test','-p','1','-vet=off','-c','-o',str(build/'service.test'),'./service'],ROOT/'hyjs-server',90):return 1
    if not run('compile-handler',['go','test','-p','1','-vet=off','-c','-o',str(build/'handler.test'),'./handler'],ROOT/'hyjs-server',90):return 1
    pattern = '^(TestFeedback84|TestP11Plunder84|TestP12Wanted84Rejected|TestP12Wanted84Committed|TestP12Wanted84Late|TestSweep84|TestWarriorBreach84)'
    ok = run('sqlite-service',[str(build/'service.test'),'-test.v','-test.run',pattern,'-test.count=1','-test.timeout=150s'],ROOT/'hyjs-server/service',160)
    ok = run('http-handler',[str(build/'handler.test'),'-test.v','-test.run','^TestFeedback84WantedSignedHTTPRecoveryAndErrorCodes$','-test.count=1','-test.timeout=45s'],ROOT/'hyjs-server/handler',55) and ok
    report['pass'] = ok
    report['binarySHA256'] = {p.name:hashlib.file_digest(p.open('rb'),'sha256').hexdigest() for p in build.glob('*.test')}
    (out/'result.json').write_text(json.dumps(report,indent=2)+'\n')
    return 0 if ok else 1

if __name__ == '__main__':
    raise SystemExit(main())

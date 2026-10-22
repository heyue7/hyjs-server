"""Build the last published server with the new lossless field storage reader."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BASE = '513411023283e44f44dc0822f7132b15d0db86f2'


def build(output):
    output = Path(output).resolve()
    def run(*args, **kwargs):
        return subprocess.run(['rtk', 'proxy', *map(str,args)], check=True, capture_output=True, **kwargs)
    with tempfile.TemporaryDirectory(prefix='hyjs-storage-rollback-') as folder:
        work = Path(folder)
        archive = work / 'source.tar'
        archive.write_bytes(run('git','archive',BASE,'hyjs-server','analysis/tables',cwd=ROOT).stdout)
        with tarfile.open(archive) as source: source.extractall(work,filter='data')
        server = work / 'hyjs-server'
        # Legacy P6 already reads gzip. Only pending field fights add a new saved
        # representation; keep its reader/writer compatible on a code rollback.
        for name in ['model/game.go','service/fieldBossService.go','service/fieldBoss33_test.go']:
            shutil.copy2(ROOT/'hyjs-server'/name,server/name)
        profile = server / 'service/profileService.go'
        text = profile.read_text()
        anchor = '\tvar (\n\t\tp, err = json.Marshal(player)'
        assert text.count(anchor)==1
        text = text.replace(anchor,'\tif err := compactFieldBossState(game); err != nil { return err }\n'+anchor)
        profile.write_text(text)
        run('gofmt','-w',profile)
        run('go','test','./service','-run','^(TestFieldBoss33OriginalDataSpawnCheckColdAndOriginalReplay|TestFieldBoss33RemnantVictoryRewardsOnceColdAndRespawn|TestAdminAuthBootstrapSessionCSRFAndRoles|TestGlobalAccountsLegacyMigrationAndWalletOrders)$','-count=1',cwd=server)
        env = dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0',GOTOOLCHAIN='auto')
        run('go','build','-trimpath','-buildvcs=false','-o',output,'.',cwd=server,env=env)
        report = dict(baseBusinessRevision=BASE,SHA256=hashlib.sha256(output.read_bytes()).hexdigest(),
                      newAndLegacyCredentialLoginTested=True,losslessFieldStorageCompatible=True,
                      scope='Published baseline plus pending field input codec; no movement/activity/no-op write optimization')
        output.with_suffix('.json').write_text(json.dumps(report,indent=2)+'\n')
        return report


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    print(json.dumps(build(parser.parse_args().output)))

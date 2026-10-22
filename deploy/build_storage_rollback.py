"""Build the published server with compatibility for the folded guild setting."""
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
BASE = '6532c195e9ab84494431268c8516bd10c3f4787e'


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
        # This published baseline already reads compressed pending field fights.
        # Keep only the guild setting migration/read/write compatibility, so a
        # rollback still sees choices made after the legacy table was retired.
        for name in ['db/p13Guild.go', 'db/p13UnionFightSetting.go',
                     'db/sql/93_p13_guilds.sql', 'db/sql/95_p13_union_fight_settings.sql',
                     'service/p13GuildService.go', 'service/p13UnionFightService.go',
                     'service/adminGuildService.go', 'service/p13GuildSchema45_test.go']:
            shutil.copy2(ROOT/'hyjs-server'/name,server/name)
        run('go','test','./service','-run','^(TestP13GuildSchema45.*|TestFieldBoss33OriginalDataSpawnCheckColdAndOriginalReplay|TestFieldBoss33RemnantVictoryRewardsOnceColdAndRespawn|TestAdminAuthBootstrapSessionCSRFAndRoles|TestGlobalAccountsLegacyMigrationAndWalletOrders)$','-count=1',cwd=server)
        env = dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0',GOTOOLCHAIN='auto')
        run('go','build','-trimpath','-buildvcs=false','-o',output,'.',cwd=server,env=env)
        report = dict(baseBusinessRevision=BASE,SHA256=hashlib.sha256(output.read_bytes()).hexdigest(),
                      newAndLegacyCredentialLoginTested=True,losslessFieldStorageCompatible=True,
                      foldedGuildAutoApplyCompatible=True,
                      scope='Published 1.0.44 server plus guild setting schema compatibility; field, client and map UI changes excluded')
        output.with_suffix('.json').write_text(json.dumps(report,indent=2)+'\n')
        return report


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    print(json.dumps(build(parser.parse_args().output)))

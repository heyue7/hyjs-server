"""Original worldseed current-node and full main identity lifecycle in isolated InnoDB.

Calls actual original worldseed/growth APIs; all seed inputs are finite prerequisites.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
OWNED = ['hyjs-server/service/gemService.go','hyjs-server/service/p8GemThirteen_test.go','analysis/p8/gem-six-compose-original.il.txt','analysis/p8/gem-thirteen-original-audit.json','tests/test_p8_gem_thirteen_mysql.py']
DEPENDENCIES = ['analysis/p5/award-collection-manifest.json','analysis/p5/refund-material-manifest.json','analysis/p5/world-seed-receipt-manifest.json','analysis/p8/gem-receipt-manifest.json','analysis/p5/empty-gem-sale-manifest.json']

@unittest.skipUnless(os.environ.get('HYJS_P8_GEM_THIRTEEN_MYSQL_TEST') == '1', 'Explicit isolated original star natural source test')
class GemThirteenMySQLTests(unittest.TestCase):
    def test_actual_original_gem_thirteen(self):
        began = time.monotonic()
        database = 'hyjs_test_p1_gem13_' + secrets.token_hex(6)
        user = 'p8gem13_' + secrets.token_hex(5)
        password = secrets.token_hex(24)
        def admin(sql):
            self.assertTrue(database.startswith('hyjs_test_p1_gem13_') and user.startswith('p8gem13_'))
            result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=sql,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr.replace(password,'[redacted]'))
            return result.stdout
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-gem13-') as temp:
            temp = Path(temp)
            base = Path(os.environ.get('HYJS_P8_GEM_THIRTEEN_BASE', ROOT/'build/candidates/joint-preview-caches-v78-20261014/hyjs-server')).resolve()
            self.assertIn((ROOT/'build/candidates').resolve(),base.parents)
            source = temp/'hyjs-server'
            shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
            (temp/'analysis').symlink_to(ROOT/'analysis',target_is_directory=True)
            for manifest_path in DEPENDENCIES:
                manifest = json.loads((ROOT/manifest_path).read_text())
                for entry in manifest['owned']:
                    rel = entry['path']
                    if not rel.startswith('hyjs-server/'): continue
                    original = ROOT/manifest['snapshot']/rel
                    self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256'])
                    target = source/rel.removeprefix('hyjs-server/')
                    target.parent.mkdir(parents=True,exist_ok=True)
                    shutil.copy2(original,target)
            for rel in OWNED:
                if rel.startswith('hyjs-server/'):
                    target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(ROOT/rel,target)
            shas = {str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
            binary = temp/'gem-thirteen.test'
            compile_result = subprocess.run(['rtk','proxy','go','test','-c','-o',str(binary),'./service'],cwd=source,capture_output=True,timeout=180)
            self.assertEqual(compile_result.returncode,0,(compile_result.stdout+compile_result.stderr).decode(errors='replace')[-4000:])
            domain = subprocess.run(['rtk','proxy',str(binary),'-test.run=^(TestP8OriginalRainbowThirteenSixCombineLifecycle|TestP8GemReceiptActualMovementGrowthDecomposeAndCold|TestP5EmptyGemSaleOriginalUnmountRefundMixedAndCold)$','-test.count=1','-test.timeout=3m'],cwd=source/'service',capture_output=True,timeout=200)
            self.assertEqual(domain.returncode,0,(domain.stdout+domain.stderr).decode(errors='replace')[-5000:])
            # Compile before taking the global lifecycle lock. Credentials stay
            # on this dedicated schema; no production sessions are accessed.
            with open('/tmp/hyjs-isolated-mysql.lock','a') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                admin(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                try:
                    output = temp/'business.json'
                    env = dict(os.environ,HYJS_P1_MYSQL_DATABASE=database,HYJS_P1_MYSQL_HOST='127.0.0.1:33068',HYJS_P1_MYSQL_USER=user,HYJS_P1_MYSQL_PASSWORD=password,HYJS_P5_WORLD_SEED_REPORT=str(output))
                    result = subprocess.run(['rtk','proxy',str(binary),'-test.run=^(TestP8OriginalRainbowThirteenSixCombineLifecycle|TestP8GemReceiptActualMovementGrowthDecomposeAndCold|TestP5EmptyGemSaleOriginalUnmountRefundMixedAndCold)$','-test.count=1','-test.timeout=3m'],cwd=source/'service',env=env,capture_output=True,timeout=200)
                    self.assertEqual(result.returncode,0,(result.stdout+result.stderr).decode(errors='replace').replace(password,'[redacted]')[-5000:])
                    report = {'status':'PASS_ORIGINAL_API_INNODB_GEM_THIRTEEN_PHONE_PENDING','actions':'actual36grade7→6grade10→1grade13;eightcurrentonce/cold;insufficientsource/skill9/nomanufacture13fail;actualmountsixoriginalprops/unmount/coldphysicalreturn/simplecompilereceiptpreserved/oldmountreject','finitePrerequisites':'36heldoriginal079/craft10historicalqualification/oneactual037/lv100;no natural36sourceorXPclaim','productionModified':False,'phoneUI':False,'HTTPFrames':False}
                    engines = admin(f"SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema='{database}' AND TABLE_NAME IN ('profiles','p5_mutation_receipts');")
                    self.assertTrue(engines.strip() and all(row.endswith('\tInnoDB') for row in engines.strip().splitlines()),engines)
                    report.update({'date':'2026-10-15','productionModified':False,'HTTPFrames':False,'phoneUI':False,'frozenGoSHA256':shas,'ownedSHA256':{rel:hashlib.sha256((ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':str(base.relative_to(ROOT)),'seconds':round(time.monotonic()-began,3),'actualEngines':engines.strip().splitlines()})
                finally:
                    admin(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
                    left = admin(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
                    self.assertEqual(left.split(),['0','0','0'])
                report['cleanupCounts'] = [0,0,0]
                (ROOT/'analysis/p8/gem-thirteen-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__ == '__main__': unittest.main()

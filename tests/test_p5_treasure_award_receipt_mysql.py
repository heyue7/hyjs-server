"""Original copy award full identity lifecycle in isolated InnoDB.

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
OWNED = ['hyjs-server/service/treasureService.go','hyjs-server/service/p5GrowthReceiptService.go','hyjs-server/service/p5TreasureAwardReceipt_test.go','tests/test_p5_treasure_award_receipt_mysql.py']
DEPENDENCIES = ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-receipt-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json','analysis/p8/revive-receipt-manifest.json']

@unittest.skipUnless(os.environ.get('HYJS_P5_TREASURE_AWARD_RECEIPT_MYSQL_TEST') == '1', 'Explicit isolated original star natural source test')
class TreasureAwardReceiptMySQLTests(unittest.TestCase):
    def test_actual_copy_award_lifecycle(self):
        began = time.monotonic()
        database = 'hyjs_test_p1_treaward_' + secrets.token_hex(6)
        user = 'p5treaward_' + secrets.token_hex(5)
        password = secrets.token_hex(24)
        def admin(sql):
            self.assertTrue(database.startswith('hyjs_test_p1_treaward_') and user.startswith('p5treaward_'))
            result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=sql,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr.replace(password,'[redacted]'))
            return result.stdout
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-treaward-') as temp:
            temp = Path(temp)
            base = Path(os.environ.get('HYJS_P5_TREASURE_AWARD_RECEIPT_BASE', ROOT/'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
            self.assertIn((ROOT/'build/candidates').resolve(),base.parents)
            source = temp/'hyjs-server'
            shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
            (temp/'analysis').symlink_to(ROOT/'analysis',target_is_directory=True)
            for manifest_path in DEPENDENCIES:
                manifest = json.loads((ROOT/manifest_path).read_text())
                for entry in manifest.get('owned',manifest.get('files',[])):
                    rel = entry['path']
                    if not rel.startswith('hyjs-server/'): continue
                    original = ROOT/entry['snapshot'] if 'snapshot' in entry else ROOT/manifest['snapshot']/rel
                    self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256'])
                    target = source/rel.removeprefix('hyjs-server/')
                    target.parent.mkdir(parents=True,exist_ok=True)
                    shutil.copy2(original,target)
            for rel in OWNED:
                if rel.startswith('hyjs-server/'):
                    target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(ROOT/rel,target)
            shas = {str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
            binary = temp/'treasure-award-receipt.test'
            compile_result = subprocess.run(['rtk','proxy','go','test','-c','-o',str(binary),'./service'],cwd=source,capture_output=True,timeout=180)
            self.assertEqual(compile_result.returncode,0,(compile_result.stdout+compile_result.stderr).decode(errors='replace')[-4000:])
            domain = subprocess.run(['rtk','proxy',str(binary),'-test.run=^(TestP8AllSixOriginalTreasureAwardReceiptLifecycle|TestP8OriginalRainbowThirteenSixCombineLifecycle)$','-test.count=1','-test.timeout=3m'],cwd=source/'service',capture_output=True,timeout=200)
            self.assertEqual(domain.returncode,0,(domain.stdout+domain.stderr).decode(errors='replace')[-5000:])
            # Compile before taking the global lifecycle lock. Credentials stay
            # on this dedicated schema; no production sessions are accessed.
            with open('/tmp/hyjs-isolated-mysql.lock','a') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                admin(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                try:
                    output = temp/'business.json'
                    env = dict(os.environ,HYJS_P1_MYSQL_DATABASE=database,HYJS_P1_MYSQL_HOST='127.0.0.1:33068',HYJS_P1_MYSQL_USER=user,HYJS_P1_MYSQL_PASSWORD=password,HYJS_P5_WORLD_SEED_REPORT=str(output))
                    result = subprocess.run(['rtk','proxy',str(binary),'-test.run=^(TestP8AllSixOriginalTreasureAwardReceiptLifecycle|TestP8OriginalRainbowThirteenSixCombineLifecycle)$','-test.count=1','-test.timeout=3m'],cwd=source/'service',env=env,capture_output=True,timeout=200)
                    self.assertEqual(result.returncode,0,(result.stdout+result.stderr).decode(errors='replace').replace(password,'[redacted]')[-5000:])
                    report = {'status':'PASS_ORIGINAL_API_INNODB_TREASURE_AWARD_RECEIPT_LIFECYCLE_PHONE_PENDING','actions':'actualoriginalCombine/115 boxcurrentcold→originalStrength foodconsumed/fullbodygrowth→oldCombine/Box hotcoldreject→actualsale fulltracked50krefund/newfoodGUID/1000base→deletedoldawardscoldreject', 'finitePrerequisites':'lvl100/260000coin/three originalfrag001 pieces/held115one; no newGUID/artificialgrownTreasure/alteredweights/rawtreasures prepared','productionModified':False,'phoneUI':False,'HTTPFrames':False}
                    engines = admin(f"SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema='{database}' AND TABLE_NAME IN ('profiles','p5_mutation_receipts');")
                    self.assertTrue(engines.strip() and all(row.endswith('\tInnoDB') for row in engines.strip().splitlines()),engines)
                    report.update({'date':'2026-10-15','productionModified':False,'HTTPFrames':False,'phoneUI':False,'frozenGoSHA256':shas,'ownedSHA256':{rel:hashlib.sha256((ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':str(base.relative_to(ROOT)),'seconds':round(time.monotonic()-began,3),'actualEngines':engines.strip().splitlines()})
                finally:
                    admin(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
                    left = admin(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
                    self.assertEqual(left.split(),['0','0','0'])
                report['cleanupCounts'] = [0,0,0]
                (ROOT/'analysis/p5/treasure-award-receipt-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__ == '__main__': unittest.main()

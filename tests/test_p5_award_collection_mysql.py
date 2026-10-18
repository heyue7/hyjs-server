"""Original awarded instance receipt lifecycle and collection batch in isolated InnoDB.

Calls actual original itemuse/growth/sale APIs and checks cold rejection and
simple replay. Collection qualification/history/rollback/batch are checked in SQL.
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
OWNED = ['hyjs-server/service/collectionService.go', 'hyjs-server/service/p5CollectionBatch_test.go', 'hyjs-server/service/p5GrowthReceiptService.go', 'hyjs-server/service/p5AwardReceipt_test.go', 'analysis/p5/award-receipt-original.il.txt', 'analysis/p5/award-collection-original-audit.json', 'tests/test_p5_award_collection_mysql.py']
DEPENDENCIES = ['analysis/p5/gold-egg-manifest.json', 'analysis/p5/copy-special-manifest.json', 'analysis/p5/copy-reset-manifest.json', 'analysis/p5/copy-box-manifest.json', 'analysis/p8/ingredient-supply-manifest.json', 'analysis/p15/red-three-supply-manifest.json', 'analysis/p5/crystal-supply-manifest.json']

@unittest.skipUnless(os.environ.get('HYJS_P5_AWARD_COLLECTION_MYSQL_TEST') == '1', 'Explicit isolated original star natural source test')
class AwardCollectionMySQLTests(unittest.TestCase):
    def test_actual_original_award_lifecycle_and_collection_batch(self):
        began = time.monotonic()
        database = 'hyjs_test_p1_award_' + secrets.token_hex(6)
        user = 'p5award_' + secrets.token_hex(5)
        password = secrets.token_hex(24)
        def admin(sql):
            self.assertTrue(database.startswith('hyjs_test_p1_award_') and user.startswith('p5award_'))
            result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=sql,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr.replace(password,'[redacted]'))
            return result.stdout
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-award-') as temp:
            temp = Path(temp)
            base = Path(os.environ.get('HYJS_P5_AWARD_COLLECTION_BASE', ROOT/'build/candidates/joint-catalogue-car-v65-20261014/hyjs-server')).resolve()
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
            binary = temp/'award-collection.test'
            compile_result = subprocess.run(['rtk','proxy','go','test','-c','-o',str(binary),'./service'],cwd=source,capture_output=True,timeout=180)
            self.assertEqual(compile_result.returncode,0,(compile_result.stdout+compile_result.stderr).decode(errors='replace')[-4000:])
            # Compile before taking the global lifecycle lock. Credentials stay
            # on this dedicated schema; no production sessions are accessed.
            with open('/tmp/hyjs-isolated-mysql.lock','a') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                admin(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                try:
                    output = temp/'business.json'
                    env = dict(os.environ,HYJS_P1_MYSQL_DATABASE=database,HYJS_P1_MYSQL_HOST='127.0.0.1:33068',HYJS_P1_MYSQL_USER=user,HYJS_P1_MYSQL_PASSWORD=password,HYJS_P5_AWARD_COLLECTION_REPORT=str(output))
                    result = subprocess.run(['rtk','proxy',str(binary),'-test.run=^(TestP5AwardReceiptOriginalBoxGrowthSaleAndSimpleReplay|TestP5CollectionBatchOriginalEligibilityHistoryAndRollback)$','-test.count=1','-test.timeout=3m'],cwd=source/'service',env=env,capture_output=True,timeout=200)
                    self.assertEqual(result.returncode,0,(result.stdout+result.stderr).decode(errors='replace').replace(password,'[redacted]')[-5000:])
                    report = {'status':'PASS_ORIGINAL_API_INNODB_PHONE_PENDING','originalAwardBoxes':['item067 choice1 hero011','item051 actualrandomorangeequipment','item041 simpleonlyvip7box'],'actualActions':'originalboxuse unchangedreplay;actualheroexp001+coin400 growth/olditemusecoldreject;actualWarriorSell+refund/retiredoldreject;actualEquipStrengthen+coin/sale+refund/retiredoldreject;actualsimpleitem001consume/oldboxreceiptpreserved','descriptorBoundary':'SCCopyBox carriesID/GUID/num only, originalUI adds descriptors to willBaseDrops; growthsameID remainsvalid, removedchangedIDinvalid. Descriptor unitchecked againstactualheld/soldgear;no newCopyBoxHTTPframe claim','treasureBoundary':'fullSCItemUse trea001 rawunithelper validates held/grown body; originalsourceinstanceandactualtreasure growthnotnewlyclaimed','finitePrerequisites':'level100/coin10m/diamond1000/originalheldboxes067,051,041+oneheroexp001; no awardWar/EquipGUID injected;finite reset/sale growth qualifications,notnewnaturalsupply','collectionGate':'first originalHero row/non99 exactpreserved (no prefixexclusion),5000absentmaterialIDs/dup/nil/unknown/eligible arbitraryID;new3singlebatch;repeat0inserts/sameAcquiredAt/history/outertxrollback','productionModified':False,'phoneUI':False,'HTTPFrames':False}
                    engines = admin(f"SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema='{database}' AND TABLE_NAME IN ('profiles','p5_collections','p5_mutation_receipts');")
                    self.assertTrue(engines.strip() and all(row.endswith('\tInnoDB') for row in engines.strip().splitlines()),engines)
                    report.update({'date':'2026-10-14','productionModified':False,'HTTPFrames':False,'phoneUI':False,'frozenGoSHA256':shas,'ownedSHA256':{rel:hashlib.sha256((ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':str(base.relative_to(ROOT)),'seconds':round(time.monotonic()-began,3),'actualEngines':engines.strip().splitlines()})
                finally:
                    admin(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
                    left = admin(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
                    self.assertEqual(left.split(),['0','0','0'])
                report['cleanupCounts'] = [0,0,0]
                (ROOT/'analysis/p5/award-collection-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__ == '__main__': unittest.main()

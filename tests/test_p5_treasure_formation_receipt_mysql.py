"""Each regression fixture starts in a fresh independent database."""
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

ROOT=Path(__file__).resolve().parents[1]
CASES=['TestP5TreasureFormationReceiptActualOldFrameSwapOffSaleAndCold']

@unittest.skipUnless(os.environ.get('HYJS_P5_TREASURE_FORMATION_RECEIPT_MYSQL_TEST')=='1','Explicit isolated fresh-schema fixture regression')
class TreasureFormationReceiptMySQLTests(unittest.TestCase):
    def test_each_fixture_independent_sqlite_and_innodb(self):
        began=time.monotonic();manifest=json.loads((ROOT/'analysis/p5/treasure-formation-receipt-manifest.json').read_text());base=ROOT/'build/candidates/joint-gems-guard-v84-20261015';token=secrets.token_hex(5);user='p5treawear_'+token;password=secrets.token_hex(24);databases=['hyjs_test_p1_treawear_'+token+'_'+str(i) for i in range(1)]
        def admin(sql):
            result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=sql,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr.replace(password,'[redacted]'));return result.stdout
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-treawear-') as work:
            work=Path(work);source=work/'hyjs-server';shutil.copytree(base/'hyjs-server',source,ignore=shutil.ignore_patterns('logs'));(work/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for dep_file in ['analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/formation-receipt-manifest.json','analysis/p5/helper-receipt-manifest.json','analysis/p5/skill-order-receipt-manifest.json','analysis/p5/equipment-formation-receipt-manifest.json']:
                dep=json.loads((ROOT/dep_file).read_text())
                for entry in dep['owned']:
                    if entry['path'].startswith('hyjs-server/') and not entry['path'].endswith('_test.go'):
                        original=ROOT/dep['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,work/entry['path'])
            for entry in manifest['owned']:
                if entry['path'].startswith('hyjs-server/'):
                    original=ROOT/manifest['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,work/entry['path'])
            binary=work/'fixture.test';result=subprocess.run(['rtk','proxy','go','test','-c','-o',str(binary),'./service'],cwd=source,capture_output=True,timeout=180);self.assertEqual(result.returncode,0,(result.stdout+result.stderr).decode(errors='replace')[-3000:]);source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
            # Prove the previous frozen production call site replays a retired slot.
            replaced=['service/p5Service.go','service/p5FormationNextService.go','service/treasureService.go'];current_files={path:(source/path).read_bytes() for path in replaced}
            for path in replaced:shutil.copy2(base/'hyjs-server'/path,source/path)
            old_binary=work/'old-projection.test';old_build=subprocess.run(['rtk','proxy','go','test','-c','-o',str(old_binary),'./service'],cwd=source,capture_output=True,timeout=180);self.assertEqual(old_build.returncode,0,(old_build.stdout+old_build.stderr).decode(errors='replace')[-3000:])
            reproduction=subprocess.run(['rtk','proxy',str(old_binary),'-test.run=^'+CASES[0]+'$','-test.count=1'],cwd=source/'service',capture_output=True,timeout=140);old_output=(reproduction.stdout+reproduction.stderr).decode(errors='replace');self.assertNotEqual(reproduction.returncode,0);self.assertIn('stale treasure frame re-executed old worn GUID',old_output)
            for path,raw in current_files.items():(source/path).write_bytes(raw)
            sqlite=[]
            for name in CASES:
                result=subprocess.run(['rtk','proxy',str(binary),'-test.run=^'+name+'$','-test.count=1','-test.timeout=2m'],cwd=source/'service',capture_output=True,timeout=140);self.assertEqual(result.returncode,0,(result.stdout+result.stderr).decode(errors='replace')[-3000:]);sqlite.append({'test':name,'status':'PASS_INDEPENDENT_FRESH_SQLITE'})
            with open('/tmp/hyjs-isolated-mysql.lock','a') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX);admin("CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"';")
                rows=[]
                try:
                    for database,name in zip(databases,CASES[:1]):
                        admin(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                        env=dict(os.environ,HYJS_P1_MYSQL_DATABASE=database,HYJS_P1_MYSQL_HOST='127.0.0.1:33068',HYJS_P1_MYSQL_USER=user,HYJS_P1_MYSQL_PASSWORD=password);start=time.monotonic();result=subprocess.run(['rtk','proxy',str(binary),'-test.run=^'+name+'$','-test.count=1','-test.timeout=2m'],cwd=source/'service',env=env,capture_output=True,timeout=140);self.assertEqual(result.returncode,0,(result.stdout+result.stderr).decode(errors='replace').replace(password,'[redacted]')[-3000:]);engines=admin(f"SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema='{database}' AND TABLE_NAME IN ('profiles','p5_mutation_receipts');");self.assertTrue(engines.strip() and all(x.endswith('\tInnoDB') for x in engines.strip().splitlines()));rows.append({'test':name,'status':'PASS_INDEPENDENT_FRESH_INNODB','seconds':round(time.monotonic()-start,3),'engines':engines.strip().splitlines()})
                finally:
                    for database in databases:admin(f"DROP DATABASE IF EXISTS `{database}`;")
                    admin(f"DROP USER IF EXISTS '{user}'@'%';");left=admin("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name IN ("+','.join("'"+x+"'" for x in databases)+"); SELECT COUNT(*) FROM mysql.user WHERE user='"+user+"'; SELECT COUNT(*) FROM mysql.db WHERE Db IN ("+','.join("'"+x+"'" for x in databases)+");");self.assertEqual(left.split(),['0','0','0'])
            report={'date':'2026-10-15','status':'PASS_ORIGINAL_CURRENT_EQUIPMENT_FRAME_GUARD_SQLITE_AND_FRESH_INNODB','baseline':str(base.relative_to(ROOT)),'ownedSHA256':{x['path']:x['sha256'] for x in manifest['owned'] if x['path'].endswith('.go')},'frozenGoSHA256':source_shas,'independentSQLite':sqlite,'independentInnoDB':rows,'seconds':round(time.monotonic()-began,3),'cleanupCounts':[0,0,0],'productionModified':False,'previousFrozenProductionReproduced':old_output.strip(),'wholeSlotProjectionGuard':True,'protocolHTTPBusinessChanged':True}
            (ROOT/'analysis/p5/treasure-formation-receipt-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()

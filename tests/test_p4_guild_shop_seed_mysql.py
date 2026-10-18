"""Original shop8 dual-cost canonical SQL seed in a fresh disposable schema."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import unittest

ROOT=Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P4_GUILD_SHOP_SEED_TEST')=='1','Explicit guild shop8 canonical SQL test')
class GuildShopSeedMySQLTests(unittest.TestCase):
    def test_original8_cost_fields_month_freeze_seed_preserves_operator(self):
        began=time.monotonic()
        database='hyjs_test_p4_guildseed_'+secrets.token_hex(6)
        user='guildseed_'+secrets.token_hex(5)
        password=secrets.token_hex(24)
        def sql(text):
            self.assertTrue(database.startswith('hyjs_test_p4_guildseed_')and user.startswith('guildseed_'))
            result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr.replace(password,'[redacted]'))
            return result.stdout
        def own(text):return sql('USE `'+database+'`;SET NAMES utf8mb4;'+text)
        path=ROOT/'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql'
        seed=path.read_text()
        report={}
        with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
            fcntl.flock(lock,fcntl.LOCK_EX)
            try:
                sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4;CREATE USER '{user}'@'%' IDENTIFIED BY '{password}';GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                own((ROOT/'hyjs-server/db/sql/83_p4_commerce_rules.sql').read_text())
                zones=[]
                for zone in ['+00:00','+08:00']:
                    own("SET time_zone='"+zone+"';"+seed)
                    row=own('SELECT id,shop_type,kind,enabled,start_at,end_at,HEX(rules),HEX(source) FROM p4_commerce_rules WHERE id LIKE "p4:guild:%";').strip().split('\t')
                    self.assertEqual(row[1:4],['8','guild','0'])
                    rules=json.loads(bytes.fromhex(row[6]))
                    self.assertEqual(len(rules['products']),1)
                    p=rules['products'][0]
                    self.assertEqual((p['rewards'],p['cost'],p['contributionCost'],p['guildLevel'],p['dailyLimit'],p['totalLimit']),('item_005,50','item_311,1',1,1,1,3))
                    self.assertIn('原私服商品数量/价格/投放窗口/额度未知',bytes.fromhex(row[7]).decode())
                    zones.append({'zone':zone,'id':row[0],'start':int(row[4]),'end':int(row[5])})
                self.assertEqual((zones[0]['id'],zones[0]['start'],zones[0]['end']),(zones[1]['id'],zones[1]['start'],zones[1]['end']))
                key=zones[0]['id']
                own(f'UPDATE p4_commerce_rules SET source="operator-choice",enabled=1 WHERE id="{key}";')
                own(seed)
                self.assertEqual(own(f'SELECT enabled,source FROM p4_commerce_rules WHERE id="{key}";').strip(),'1\toperator-choice')
                self.assertEqual(own(f'SELECT ENGINE FROM information_schema.tables WHERE table_schema="{database}" AND table_name="p4_commerce_rules";').strip(),'InnoDB')
                report={'status':'PASS_CANONICAL_GUILD_SHOP8_DUAL_COST_DEFAULT_OFF_TIMEZONE_OPERATOR','seconds':round(time.monotonic()-began,3),'seedSHA256':hashlib.sha256(path.read_bytes()).hexdigest(),'zones':zones,'operatorPreserved':True,'productionModified':False}
            finally:
                sql(f"DROP DATABASE IF EXISTS `{database}`;DROP USER IF EXISTS '{user}'@'%';")
                left=sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}';SELECT COUNT(*) FROM mysql.user WHERE user='{user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
                self.assertEqual(left.split(),['0','0','0'])
                report['cleanupCounts']=[0,0,0]
        (ROOT/'analysis/p4/guild-shop-sql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()

"""Canonical source seed, timezone invariance and operator preservation in one disposable DB."""
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import time
import unittest

from test_p4_fireworks_supply_mysql import FireworksSupplyMySQLTests

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P4_FIREWORKS_SEED_TEST') == '1', 'Explicit canonical SQL seed test')
class FireworksSeedMySQLTests(unittest.TestCase):
    def test_source_seed_operator_and_both_timezones(self):
        began = time.monotonic()
        fixture = FireworksSupplyMySQLTests()
        FireworksSupplyMySQLTests.database = 'hyjs_test_firework_sql_'+secrets.token_hex(6)
        FireworksSupplyMySQLTests.test_user = 'firework_sql_'+secrets.token_hex(4)
        FireworksSupplyMySQLTests.test_password = secrets.token_hex(24)
        ddl = (ROOT/'hyjs-server/db/sql/83_p4_commerce_rules.sql').read_text()
        path = ROOT/'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql'
        seed = path.read_text()
        report = {}
        with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
            fcntl.flock(lock,fcntl.LOCK_EX)
            try:
                fixture.root_sql(f"CREATE DATABASE `{fixture.database}` CHARACTER SET utf8mb4;CREATE USER '{fixture.test_user}'@'%' IDENTIFIED BY '{fixture.test_password}';GRANT ALL ON `{fixture.database}`.* TO '{fixture.test_user}'@'%';")
                fixture.sql(ddl)
                zones = []
                for zone in ['+00:00','+08:00']:
                    fixture.sql("SET NAMES utf8mb4;SET time_zone='"+zone+"';"+seed)
                    row = fixture.sql('SELECT id,enabled,start_at,end_at,claim_end_at,HEX(rules),HEX(source) FROM p4_commerce_rules WHERE id LIKE "limitExchange_p4_fireworks_%";').strip().split('\t')
                    self.assertEqual(len(row),7)
                    self.assertEqual(row[1],'0')
                    rules = json.loads(bytes.fromhex(row[5]))
                    product = rules['products'][0]
                    self.assertEqual((len(rules['products']),product['id'],product['cost'],product['dailyLimit'],product['totalLimit'],product['minLevel']),(1,'item_350','coin,2000000',1,3,70))
                    source = bytes.fromhex(row[6]).decode()
                    self.assertIn('原服投放/售价/窗口/限额未知',source)
                    zones.append({'zone':zone,'id':row[0],'start':int(row[2]),'end':int(row[3]),'claimEnd':int(row[4])})
                self.assertEqual({k:v for k,v in zones[0].items()if k!='zone'},{k:v for k,v in zones[1].items()if k!='zone'})
                local = datetime.datetime.now(datetime.timezone.utc).astimezone(datetime.timezone(datetime.timedelta(hours=8)))
                start = local.replace(day=1,hour=0,minute=0,second=0,microsecond=0)
                end = (start+datetime.timedelta(days=32)).replace(day=1)
                self.assertEqual((zones[0]['start'],zones[0]['end']),(int(start.timestamp()*1000),int(end.timestamp()*1000)))
                key=zones[0]['id']
                fixture.sql(f'UPDATE p4_commerce_rules SET enabled=1,source="operator-owned-before-publication" WHERE id="{key}";')
                fixture.sql("SET NAMES utf8mb4;"+seed)
                self.assertEqual(fixture.sql(f'SELECT enabled,source FROM p4_commerce_rules WHERE id="{key}";').strip(),'1\toperator-owned-before-publication')
                self.assertEqual(fixture.sql('SELECT ENGINE FROM information_schema.tables WHERE table_schema="'+fixture.database+'" AND table_name="p4_commerce_rules";').strip(),'InnoDB')
                report={'status':'PASS_CANONICAL_SQL_OFF_SOURCE_UTC8_MONTH_OPERATOR_PRESERVATION','seconds':round(time.monotonic()-began,3),'seedSHA256':hashlib.sha256(path.read_bytes()).hexdigest(),'zones':zones,'operatorPreserved':True,'engine':'InnoDB','productionModified':False}
            finally:
                fixture.root_sql(f"DROP DATABASE IF EXISTS `{fixture.database}`;DROP USER IF EXISTS '{fixture.test_user}'@'%';")
                left=fixture.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{fixture.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{fixture.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{fixture.database}';")
                self.assertEqual(left.split(),['0','0','0'])
                report['cleanupCounts']=[0,0,0]
        (ROOT/'analysis/p4/fireworks-supply-sql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()

from hyjs_managed_mysql import provision_case
"""Actual auto qualification certificate domain, isolated source/destination InnoDB schemas."""
import fcntl,hashlib,json,os,secrets,subprocess,time,unittest
from pathlib import Path
import test_p13_mysql as guild
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P14_QUALIFICATION_MYSQL_TEST')=='1','explicit isolated qualification domain')
class P14QualificationMySQLTests(unittest.TestCase):
 root_sql=classmethod(guild.P13MySQLTests.root_sql.__func__)
 @classmethod
 def setUpClass(cls):
  cls.source=Path(os.environ['HYJS_P14_QUALIFICATION_SOURCE']);assert cls.source.name.startswith('hyjs-p14-qualification-')
  cls.binary=cls.source/'service-race.test';assert cls.binary.is_file()
  cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(lambda:(fcntl.flock(cls.lock,fcntl.LOCK_UN),cls.lock.close()))
  pool, primary = provision_case(cls, 'p13_qual_source')
  cls.foreign_connection = pool.allocate('p13_qual_dest')
  cls.foreign = cls.foreign_connection.database
  cls.env['HYJS_P14_QUALIFICATION_FOREIGN_DATABASE'] = cls.foreign
 def test_actual_source_auto_strict_proof_eight_imports_and_cold(self):
  began=time.monotonic();r=subprocess.run(['rtk','proxy',str(self.binary),'-test.run=^TestP14QualificationActual','-test.count=1','-test.timeout=120s'],cwd=self.source/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=150)
  self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
  for name in [self.database,self.foreign]:
   tables=self.root_sql('SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA="'+name+'" AND TABLE_NAME IN ("p14_qualification_federation_policies","p14_qualification_deliveries","p14_foreign_qualifications");').strip().splitlines();self.assertEqual(len(tables),3);self.assertTrue(all(x.endswith('\tInnoDB') for x in tables))
  report={'status':'PASS_ACTUAL_AUTO_CERTIFICATE_DOMAIN_TWO_INNODB_SCHEMAS','seconds':round(time.monotonic()-began,3),'source':'actual three local players signup→cutoff/fullroster→original zero-round autoPass→immutable qualification certificate/outbox; no direct qualification rows seeded','destination':'separate schema localRealm2, defaultclosed/source HMAC/destination/playedRound>0 guards, eight concurrent exactimports one immutable source; realforeign originalRealm1 identities preserved, no invented registration/member; conflicting signed certificate denied, cold source same, corrupted storedblob denied','boundary':'domain only; original public rawframes and actual two-process HTTP outbox/restart remain pending root shared hooks; global P6authority false, new federationdefaultoff; no public match/score/guess/award','binarySHA256':hashlib.sha256(self.binary.read_bytes()).hexdigest(),'productionModified':False,'cleanup':'whole-life uniformfilelock, source/destination isolatedschemas/user/grants dropped after test'}
  (ROOT/'analysis/p14/qualification-domain-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()

"""Bounded expiry sweep on real uploaded WAVs, real InnoDB concurrent/cold lifecycle."""
import json,os,subprocess,time,unittest
from pathlib import Path
import test_p10_voice_mysql as voice
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P10_VOICE_SWEEP_MYSQL_TEST')=='1','explicit isolated voice sweep')
class P10VoiceSweepMySQLTests(voice.P10VoiceMySQLTests):
 test_1_actual_innodb_race_pcm_and_all_channel_permissions=None
 test_2_actual_http_tcp_cold_delete_blacklist_and_session=None
 def test_actual_pcm_more_than_one_batch_cancelled_concurrent_and_cold(self):
  began=time.monotonic()
  result=subprocess.run(['rtk','proxy',str(self.prebuilt/'service-race.test'),'-test.run=^TestP10VoiceSweepBounded','-test.count=1','-test.timeout=120s'],cwd=self.source/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=150)
  self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())
  self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_voice_assets WHERE state=3 AND LENGTH(bytes)=0;').strip(),'135')
  self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_voice_assets WHERE state=1 AND LENGTH(bytes)>0;').strip(),'1')
  self.assertEqual(self.sql('SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA="'+self.database+'" AND TABLE_NAME="p10_voice_assets";').strip(),'InnoDB')
  report={'status':'PASS_REAL_INNODB_RACE_BOUNDED_PCM_SWEEP','seconds':round(time.monotonic()-began,3),'source':'135 actual UploadP10Voice PCM objects, expiry advanced by explicit isolated fixture with recalculated source SHA; one future actual object','scope':'cancel before sweep preserves all bytes; two concurrent sweeps consume over128 objects in ID-only batches; lock recheck preserves future object and metadata/tombstone; cold idempotent retry, gateoff still purges expired bytes','memory':'discovery onlySELECT id LIMIT128, not allLONGBLOB; one current object read under lock at a time','limitations':'this increment is expiry maintenance only; original audio frames/ACL and microphone verification remain prior fixedVoice121/root device scope','binarySHA256':self.fixed_manifest['raceSHA256'],'productionModified':False,'cleanup':'lifecycle filelock, two isolatedschemas/testuser/grants removed; temporary prebuilt/fullsource removed after result'}
  (ROOT/'analysis/p10/voice-sweep-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()

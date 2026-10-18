"""Replay exact saved actual history for a short original81046 read regression.
No new signup, qualification, combat or series result is claimed by this test.
The source rows are byte-preserved actual v10 outputs restored in an owned DB.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import time
import unittest
import test_p14_final_rounds_mysql as actual
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get("HYJS_P14_HISTORY_READ_MYSQL_TEST")=="1","explicit actual saved-history read regression")
class P14ActualHistoryReadTests(actual.P14FinalRoundsNativeMySQLTests):
    __unittest_skip__=os.environ.get("HYJS_P14_HISTORY_READ_MYSQL_TEST")!="1"
    def tearDown(self):
        # The exact upstream archive is already immutable; do not export it again.
        # Registered class cleanups still stop each app and release this allocation.
        return

    def restore_actual_history(self, realm, folder, manifest):
        prefix="realm"+str(realm)+"-"
        tables=manifest["realms"][str(realm)]
        restored={}
        # No profile, login/session or peer signing key is present in this archive.
        # A new original-protocol observer is used only for the public read.
        for table, meta in tables.items():
            self.assertRegex(table,r"^(p14_[a-z0-9_]+|p6_battle_records)$")
            self.assertNotEqual(table,"p14_trusted_peers")
            name=meta["file"];self.assertTrue(name.startswith(prefix))
            raw=(folder/name).read_bytes()
            self.assertEqual(hashlib.sha256(raw).hexdigest(),manifest["files"][name])
            self.assertTrue(meta["complete"])
            rows=[json.loads(line) for line in raw.splitlines()]
            self.assertEqual(len(rows),meta["rows"])
            self.sql(realm,"DELETE FROM `"+table+"`;")
            batch=[];size=0
            for row in rows:
                columns=sorted(row)
                self.assertTrue(all(re.fullmatch(r"[a-z][a-z0-9_]*",key) for key in columns))
                self.assertTrue(all(value is None or re.fullmatch(r"[0-9A-Fa-f]*",value) for value in row.values()))
                values=["NULL" if row[key] is None else "UNHEX(\""+row[key]+"\")" for key in columns]
                statement="INSERT INTO `"+table+"` ("+",".join("`"+key+"`" for key in columns)+") VALUES ("+",".join(values)+");"
                if batch and size+len(statement)>4*1024*1024:
                    self.sql(realm,"".join(batch));batch=[];size=0
                batch.append(statement);size+=len(statement)
            if batch:self.sql(realm,"".join(batch))
            self.assertEqual(int(self.sql(realm,"SELECT COUNT(*) FROM `"+table+"`;").strip()),len(rows))
            restored[table]=len(rows)
        return restored

    def test_original_81046_from_exact_actual_history_and_cold(self):
        self.shutdownTimes=[]
        type(self).native_report={"status":"RUNNING_RESTORED_ACTUAL_HISTORY_QUERY","productionModified":False}
        folder=Path(os.environ["HYJS_P14_ACTUAL_HISTORY_SNAPSHOT"]).resolve()
        self.assertRegex(folder.name,r"^final-rounds-state-[0-9]+$")
        manifest=json.loads((folder/"manifest.json").read_text())
        self.assertIn("HEX(CAST(column AS BINARY))",manifest["encoding"])
        observers={};counts={};calls=[]
        for realm in (1,2):
            self.start(realm)
            login=self.login(realm,"actual-history-observer-"+str(realm)+"-"+secrets.token_hex(4))
            self.call(realm,"CSPlayerCreate",{"playerName":"历史只读观察"+str(realm),"heroId":"hero_047"},login,2,"SCPlayerCreate")
            observers[realm]=login
            self.stop(realm)
            counts[str(realm)]=self.restore_actual_history(realm,folder,manifest)
        source=json.loads((folder/"realm1-p14_final_round_series.jsonl").read_text().splitlines()[0])
        body=json.loads(bytes.fromhex(source["manifest"]))
        at=body["slots"][2]+1
        self.clock.write_text(str(at))
        original=[self.sql(realm,"SELECT manifest_hash,left_wins,right_wins,winner_server_id,winner_player_id FROM p14_final_round_series ORDER BY id;").strip() for realm in (1,2)]
        self.assertEqual(original[0],original[1])
        for cold in (False,True):
            for realm in (1,2):
                fixed=self.ports[realm] if cold else None
                if cold:self.stop(realm)
                self.start(realm,fixed)
                began=time.monotonic()
                reply=self.call(realm,"CSKFMatchFrightList",{"groupId":"A"},observers[realm],7000000+100*int(cold)+realm,"SCKFMatchFrightList")
                elapsed=round(time.monotonic()-began,3)
                view=reply.kfMatchAllRandList
                self.assertEqual((len(view.kFMatchFirstList),len(view.kFMatchSecondList),len(view.kFMatchThirdList)),(8,4,2))
                self.assertEqual([p.winTime for p in view.kFMatchThirdList],[3,0])
                self.assertEqual(self.sql(realm,"SELECT manifest_hash,left_wins,right_wins,winner_server_id,winner_player_id FROM p14_final_round_series ORDER BY id;").strip(),original[realm-1])
                calls.append({"realm":realm,"cold":cold,"seconds":elapsed,"sizes":[8,4,2],"thirdWins":[3,0]})
        result={"status":"PASS_ORIGINAL_81046_RESTORED_ACTUAL_HISTORY_COLD","scope":"short query regression from exact saved actual completed history; fresh public observers, no new signup/combat/result claim","actualSnapshot":str(folder),"snapshotManifestSHA256":hashlib.sha256((folder/"manifest.json").read_bytes()).hexdigest(),"restoredCounts":counts,"originalHTTP":calls,"nativeSHA256":self.meta["nativeSHA256"],"productionModified":False,"phoneVerified":False}
        (ROOT/"analysis/p14/final-history-read-validation.json").write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n")

if __name__=="__main__":unittest.main()

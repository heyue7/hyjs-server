"""Independent ID2 native extension. No current authority is presumed.

The same actual four qualifiers and untouched draw feed whichever distinct
realm/PID pair occurs. An independently certified same-realm scope must be
explicitly present and prepublished; otherwise the base run preserves pending.
"""
import json
import os
import re
import time
import unittest
from pathlib import Path
import test_p14_next_round_actual_mysql as actual

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get("HYJS_P14_SAME_REALM_CHAIN_MYSQL_TEST")=="1","explicit managed independent same-realm policy native")
class P14SameRealmNextRoundNativeMySQLTests(actual.P14NextRoundActualMySQLTests):
    runtime_relative_path="analysis/p14/same-realm-next-round-runtime.log"
    checkpoint_relative_path="analysis/p14/same-realm-next-round-checkpoint.json"
    __unittest_skip__=os.environ.get("HYJS_P14_SAME_REALM_CHAIN_MYSQL_TEST")!="1"

    @classmethod
    def setUpClass(cls):
        previous=ROOT/"analysis/p14/next-round-actual-native-validation.json"
        saved=previous.read_bytes() if previous.exists() else None
        def record():
            report=dict(getattr(cls,"native_report",None) or {"status":"FAILED_BEFORE_ID2_CHAIN"})
            report["independentSameRealmPolicyID"]=2
            report["managedAllocationRemoved"]=not bool(getattr(getattr(cls,"managed_mysql",None),"connections",{}))
            (ROOT/"analysis/p14/same-realm-next-round-native-validation.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
            if saved is None:previous.unlink(missing_ok=True)
            else:previous.write_bytes(saved)
        cls.addClassCleanup(record)
        super().setUpClass()

    def configure_additional_admission(self,now):
        text=(self.source/"hyjs-server/constUtil/p14KFSameRealm.go").read_text()
        proof=re.search(r'\bP14KFSameRealmNativeProof\s*=\s*"([^\"]*)"',text)[1]
        self.assertTrue(not proof or (len(proof)==64 and bytes.fromhex(proof)),"exact independently frozen proof")
        self.same_realm_proof=proof
        if not proof:return
        for zone in (1,2):
            for table,key in (("p14_final_settlement_policies","P14KFSameRealmSettlementSource"),("p14_final_result_policies","P14KFSameRealmResultSource"),("p14_final_continuation_policies","P14KFSameRealmContinuationSource"),("p14_next_result_policies","P14KFSameRealmNextResultSource")):
                source=self.source_constant("p14KFSameRealm.go",key)
                self.sql(zone,"UPDATE "+table+" SET enabled=1,version='independent-same-realm-native-v1',source='"+source+"',published_at="+str(now-1000)+" WHERE id=2;")
                self.assertEqual(self.sql(zone,"SELECT scope_proof FROM "+table+" WHERE id=2;").strip(),proof)
        self.checkpoint("INDEPENDENT_ID2_PREPUBLISHED_BEFORE_SIGNUP",sameRealmScopeProof=proof)

    def same_realm_admission_ready(self):
        return bool(getattr(self,"same_realm_proof",None))

if __name__=="__main__":unittest.main()

"""Managed true two-parent and genuine round10 series consumer."""
import json
import os
import unittest
from pathlib import Path
import test_p14_next_round_same_realm_mysql as actual
import p14_next_result_checks as result_checks
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get("HYJS_P14_NEXT_RESULT_MYSQL_TEST")=="1","explicit actual next-round series result chain")
class P14NextResultNativeMySQLTests(actual.P14SameRealmNextRoundNativeMySQLTests):
    __unittest_skip__=os.environ.get("HYJS_P14_NEXT_RESULT_MYSQL_TEST")!="1"
    checkpoint_relative_path="analysis/p14/next-result-native-checkpoint.json"
    runtime_relative_path="analysis/p14/next-result-native-runtime.log"

    @classmethod
    def setUpClass(cls):
        def record():
            report=dict(getattr(cls,"native_report",None) or {"status":"FAILED_BEFORE_NEXT_RESULT_CHAIN"})
            if report.get("nextSeriesChecks"):report["status"]=report["nextSeriesChecks"]["status"]
            report["managedAllocationRemoved"]=not bool(getattr(getattr(cls,"managed_mysql",None),"connections",{}))
            (ROOT/"analysis/p14/next-result-native-validation.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        cls.addClassCleanup(record)
        super().setUpClass()

    def configure_additional_admission(self,now):
        super().configure_additional_admission(now)
        for realm in (1,2):
            for table,file,key in (("p14_next_series_policies","p14NextSeries.go","P14NextSeriesSource"),("p14_next_result_policies","p14NextResult.go","P14NextResultSource")):
                self.sql(realm,"UPDATE "+table+" SET enabled=1,version='actual-prepublished-next-result-v1',source='"+self.source_constant(file,key)+"',published_at="+str(now-1000)+" WHERE id=1;")

    def expects_next_result(self):return True

    def after_next_round_input(self,players,identities,offer,exported):
        self.next_series_checks=result_checks.run_actual(self,players,identities,offer,exported)

if __name__=="__main__":unittest.main()

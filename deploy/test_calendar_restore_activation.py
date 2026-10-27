import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

spec=importlib.util.spec_from_file_location("calendar",Path(__file__).with_name("calendar_restore_activation.py"))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class CalendarActivationTest(unittest.TestCase):
 def test_cutover_and_same_request_retry_preserve_preview(self):
  calls=[]
  request="8e53bca0-6ccb-4c16-bb18-416a2925c4af"
  plan={"serverId":1,"digest":"a"*64,"windows":[1],"openingAt":1793030400000}
  result={"requestId":request,"digest":plan["digest"],"openingAt":plan["openingAt"]}
  def compose(*args):
   calls.append(args)
   value=plan if "--preview-activity-calendar" in args else result
   return SimpleNamespace(stdout=("diagnostic line\n"+json.dumps(value)+"\n").encode())
  def mysql(sql):
   return "0\n" if "offset_days" in sql else "1\n"
  with tempfile.TemporaryDirectory() as root:
   backup=Path(root)/"backups";backup.mkdir()
   for _ in range(2):
    out=m.activate_calendar_restore(mysql,compose,backup,"fixed-source",{"sha256":"backup"},request)
    self.assertEqual(out["digest"],plan["digest"])
   self.assertEqual(sum("--preview-activity-calendar" in a for a in calls),1)
   self.assertEqual(sum("--restore-activity-calendar" in a for a in calls),2)
 def test_backup_and_maintenance_guard_precede_any_command(self):
  with tempfile.TemporaryDirectory() as root:
   for backup,state in [(None,"1"),({"sha256":"x"},"0")]:
    with self.assertRaises(AssertionError):
     m.activate_calendar_restore(lambda q:state,lambda *a:self.fail("must not execute"),Path(root),"x",backup,"8e53bca0-6ccb-4c16-bb18-416a2925c4af")
 def test_incomplete_output_rejected(self):
  with self.assertRaises(RuntimeError):m.command_json(SimpleNamespace(stdout=b"ok without receipt"))

if __name__=="__main__":unittest.main()

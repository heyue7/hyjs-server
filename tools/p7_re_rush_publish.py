"""Generate reviewable Re Trial catalogue publication SQL; this tool never connects to a DB."""
import argparse
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
COLUMNS={'id':'id','kind':'kind','task_id':'taskId','floor':'floor','star':'star','condition_task_id':'conditionTaskId','min_level':'minLevel','enabled':'enabled','scene':'scene','max_round':'maxRound','win_key':'winKey','win_value':'winValue','first_reward':'firstReward','reward':'reward','recommended_power':'recommendedPower','source':'source'}

def literal(value):
    if isinstance(value,bool):return str(int(value))
    if isinstance(value,(int,float)):return str(value)
    return '0x'+str(value).encode().hex() if value else "''"

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--catalogue',type=Path,required=True)
    p.add_argument('--catalogue-sha256',required=True)
    p.add_argument('--output',type=Path,required=True)
    args=p.parse_args();raw=args.catalogue.read_bytes()
    if hashlib.sha256(raw).hexdigest()!=args.catalogue_sha256:raise ValueError('Catalogue SHA differs')
    entries=json.loads(raw)
    if len(entries)!=340:raise ValueError('Complete 306 trial choices + 34 elites required')
    trials={(e['rule']['floor'],e['rule']['star']) for e in entries if e['rule']['kind']==1}
    if trials!={(floor,star) for floor in range(1,103) for star in range(1,4)}:raise ValueError('Trial source range differs')
    elite=[e['rule'] for e in entries if e['rule']['kind']==2]
    if {e['taskId'] for e in elite}!={f'{i:02}' for i in range(1,35)}:raise ValueError('Original 34 elite task IDs differ')
    sources={e['rule']['source'] for e in entries}
    if len(sources)!=1:raise ValueError('One immutable source required')
    source=next(iter(sources))
    if 'fd703e1455b7585f95d6353df12c081819005284e05f7e01257782aafdb62fbd' not in source:raise ValueError('Re game source pin differs')
    if len({e['rule']['id'] for e in entries})!=340:raise ValueError('Duplicate catalogue IDs')
    lines=['-- Generated from actual Go/protobuf catalogue export '+args.catalogue_sha256,
           '-- Apply only with the qualified matching server source; take a catalogue backup first.',
           '-- Existing player states, histories, receipts and active battles are untouched.',
           "SET @re_lock=GET_LOCK('hyjs.publish.re.rush.v1',30);",'START TRANSACTION;',
           'SET @re_ready=(@re_lock=1 AND EXISTS(SELECT 1 FROM p7_re_rush_policies WHERE id=1 AND source='+literal(source)+') AND EXISTS(SELECT 1 FROM p7_rush_policies WHERE id=1 AND enabled=1));',
           'UPDATE p7_rush_stage_rules SET enabled=0 WHERE kind=1 AND source<>'+literal(source)+' AND @re_ready;']
    columns=[*COLUMNS,'enemies']
    for start in range(0,len(entries),100):
        statements=[]
        for entry in entries[start:start+100]:
            row=entry['rule'];enemy=bytes.fromhex(entry['enemiesHex'])
            if not enemy or not row['enabled']:raise ValueError('Enabled full protobuf enemy missing')
            values=[literal(row[key]) for key in COLUMNS.values()]+['0x'+enemy.hex()]
            statements.append('SELECT '+','.join(values)+' WHERE @re_ready')
        lines.append('INSERT INTO p7_rush_stage_rules ('+','.join(columns)+')\n'+'\nUNION ALL\n'.join(statements)+'\nON DUPLICATE KEY UPDATE '+','.join(c+'=VALUES('+c+')' for c in columns[1:])+';')
    lines += ['UPDATE p7_re_rush_policies SET enabled=1 WHERE id=1 AND @re_ready;',
              'SELECT @re_ready AS publication_precondition,(SELECT COUNT(*) FROM p7_rush_stage_rules WHERE source='+literal(source)+' AND enabled=1) AS enabled_re_rows;',
              'COMMIT;',"SELECT RELEASE_LOCK('hyjs.publish.re.rush.v1') AS publication_lock_released;"]
    if args.output.exists():raise ValueError('Use a new SQL output path')
    args.output.write_text('\n'.join(lines)+'\n')
    print(json.dumps({'sql':str(args.output),'sha256':hashlib.sha256(args.output.read_bytes()).hexdigest(),'trialChoices':306,'eliteStages':34,'DBConnected':False}))

if __name__=='__main__':main()

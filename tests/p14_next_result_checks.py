"""True175 origin -> real176 uploads -> certified177 natural series.

No qualification, source, score or result row is created here. The existing
managed two-app runner owns all temporary accounts and its private server clock.
"""
import json
import time
import p14_next_series_checks as slots


def run_actual(runner,players,identities,offer,exported):
    for realm in (1,2):
        runner.wait_sql(realm,"SELECT COUNT(*) FROM p14_next_series;","1")
    series_id=runner.sql(1,"SELECT id FROM p14_next_series;").strip()
    runner.assertRegex(series_id,r"^[0-9a-f]{64}$")
    row=json.loads(runner.sql(1,"SELECT CONVERT(manifest USING utf8mb4) FROM p14_next_series;").strip())
    pair=row["pair"];origins=sorted({pair["leftServerId"],pair["rightServerId"]})
    canonical=pair["leftServerId"]
    runner.assertIn(canonical,(1,2))
    runner.wait_sql(canonical,"SELECT COUNT(*) FROM p14_next_results WHERE series_id='"+series_id+"' AND game_index=1;","1",timeout=45)
    for realm in (1,2):runner.wait_sql(realm,"SELECT left_wins+right_wins FROM p14_next_series WHERE id='"+series_id+"';","1",timeout=45)
    history=[];sources=[]
    for index in range(2,6):
        state=int(runner.sql(1,"SELECT state FROM p14_next_series WHERE id='"+series_id+"';").strip())
        if state==3:break
        runner.clock.write_text(str(row["slots"][index-2]+1000))
        for realm,pid in ((pair["leftServerId"],pair["leftPlayerId"]),(pair["rightServerId"],pair["rightPlayerId"])):
            owner=identities[(realm,pid)]
            source=slots.capture_actual(runner,realm,owner,series_id,index,2000+index*100+realm*10+pid)
            sources.append(source)
            peer=3-realm
            where=" WHERE series_id='"+series_id+"' AND game_index="+str(index)+" AND server_id="+str(realm)+" AND player_id="+str(pid)
            runner.wait_sql(peer,"SELECT source_hash FROM p14_next_slot_formations"+where+";",source["sourceHash"])
        runner.clock.write_text(str(row["slots"][index-1]-300000))
        for realm in (1,2):runner.wait_sql(realm,"SELECT COUNT(*) FROM p14_next_slot_seals WHERE series_id='"+series_id+"' AND game_index="+str(index)+";",str(len(origins)))
        runner.clock.write_text(str(row["slots"][index-1]+1))
        for realm in (1,2):runner.wait_sql(realm,"SELECT left_wins+right_wins FROM p14_next_series WHERE id='"+series_id+"';",str(index),timeout=45)
        history.append({"gameIndex":index,"sourceHashes":[source["sourceHash"] for source in sources if source["gameIndex"]==index]})
    final_query="SELECT JSON_OBJECT('leftWins',left_wins,'rightWins',right_wins,'winnerServerId',winner_server_id,'winnerPlayerId',winner_player_id,'state',state,'currentGame',current_game) FROM p14_next_series WHERE id='"+series_id+"';"
    final=[json.loads(runner.sql(realm,final_query).strip()) for realm in (1,2)]
    runner.assertEqual(final[0],final[1]);runner.assertEqual(final[0]["state"],3)
    runner.assertIn(3,(final[0]["leftWins"],final[0]["rightWins"]))
    count=final[0]["leftWins"]+final[0]["rightWins"]
    runner.assertIn(count,(3,4,5))
    runner.assertIn((final[0]["winnerServerId"],final[0]["winnerPlayerId"]),((pair["leftServerId"],pair["leftPlayerId"]),(pair["rightServerId"],pair["rightPlayerId"])))
    for realm in (1,2):
        expected=count if realm==canonical else 0
        runner.assertEqual(int(runner.sql(realm,"SELECT COUNT(*) FROM p6_battle_records WHERE source='p14-next-round-kf-canonical';").strip()),expected)
        table="p14_next_results" if realm==canonical else "p14_next_result_receipts"
        runner.assertEqual(int(runner.sql(realm,"SELECT COUNT(*) FROM "+table+" WHERE series_id='"+series_id+"';").strip()),count)
        panel=runner.call(realm,"CSKFMatchFrightList",{"groupId":"A"},players[realm][0],2800+realm,"SCKFMatchFrightList")
        items=panel.kfMatchAllRandList.kFMatchSecondList
        runner.assertEqual([(x.serverId,x.playerId,x.winTime) for x in items],[(pair["leftServerId"],pair["leftPlayerId"],final[0]["leftWins"]),(pair["rightServerId"],pair["rightPlayerId"],final[0]["rightWins"])])
        if pair["leftPlayerId"]!=pair["rightPlayerId"]:
            reg=str(offer["seasonIndex"])+"_"+str(pair["leftServerId"])+"_"+str(pair["leftPlayerId"])
            logs=runner.call(realm,"CSKFReplayInfo",{"type":3,"subType":10,"regId":reg},players[realm][0],2850+realm,"SCKFReplayInfo")
            runner.assertEqual(len(logs.replays),count)
            runner.assertTrue(all(x.state==0 and x.warId for x in logs.replays))
        else:
            runner.assertEqual(runner.send(realm,"CSKFReplayInfo",{"type":3,"subType":10,"regId":str(offer["seasonIndex"])+"_"+str(pair["leftServerId"])+"_"+str(pair["leftPlayerId"])},players[realm][0],2850+realm)[0],10001)
        fixed=runner.ports[realm];runner.stop(realm);runner.start(realm,fixed)
        runner.assertEqual(json.loads(runner.sql(realm,final_query).strip()),final[0])
    return {"status":"PASS_REAL_CURRENT_SLOT_CERTIFIED_SERIES_SECOND_COLUMN_COLD","seriesId":series_id,"actualPair":pair,"games":count,"history":history,"final":final[0],"canonicalP6Origin":canonical,"peerHasNoForeignP6":True,"noNextBracketOrChampionClaim":True}

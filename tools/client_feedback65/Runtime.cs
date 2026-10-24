using System;
using System.Collections;
using System.Reflection;
using System.IO;
using ProtoBuf.Message;
using NetWork.Packets;
using Battle;
using UI;
using UnityEngine;

namespace HYJS.Feedback65
{
    public static class Runtime
    {
        sealed class WantedState
        {
            public object Player;
            public long PlayerId;
            public byte[] Session, Start;
            public string InstanceId, FightId;
            public int AttackType;
            public WantedMission Page;
            public bool Recoverable, InBattle, RequestPending;
        }
        static WantedState wanted;
        static object settlementOwner;
        static string settlementFight;
        static object settlementPlayer;
        static byte[] settlementSession;
        static byte[] Session() { return StaticData.sessionKey == null ? null : (byte[])StaticData.sessionKey.Clone(); }
        static bool SameSession(byte[] expected)
        {
            byte[] current = StaticData.sessionKey;
            if (expected == null || current == null || expected.Length != current.Length) return false;
            for (int i = 0; i < current.Length; i++) if (expected[i] != current[i]) return false;
            return true;
        }
        static bool Owned(WantedState state)
        {
            return state != null && state.Player == StaticData.playerData && StaticData.playerData != null &&
                state.PlayerId == StaticData.playerData.playerId && SameSession(state.Session);
        }
        static string Selected(object page)
        {
            IList list = Field(page, "bossList").GetValue(page) as IList;
            int index = (int)Field(page, "curBossIndex").GetValue(page);
            if (list == null || index < 0 || index >= list.Count) return null;
            RebelInfo info = list[index] as RebelInfo;
            return info == null ? null : info.id;
        }
        public static void WantedRequest(object page, int type)
        {
            WantedMission view = page as WantedMission;
            if (view == null || StaticData.playerData == null) { wanted = null; return; }
            wanted = new WantedState { Player = StaticData.playerData, PlayerId = StaticData.playerData.playerId,
                Session = Session(), Page = view, InstanceId = Selected(view), AttackType = type, RequestPending = true };
        }
        public static bool WantedStart(object page, object packet, bool success)
        {
            WantedState state = wanted;
            if (!Owned(state) || !System.Object.ReferenceEquals(state.Page, page) || !state.RequestPending) return true;
            state.RequestPending = false;
            if (!success) { wanted = null; return false; }
            if (state.InstanceId != Selected(page)) { wanted = null; return true; }
            Packet received = packet as Packet;
            SCRebelFightStart start = received == null ? null : received.kBody as SCRebelFightStart;
            if (start == null || String.IsNullOrEmpty(start.fightId)) return true;
            // Capture the real wire graph before SetData shares battlePlayer/monster references with the engine.
            try
            {
                using (MemoryStream stream = new MemoryStream())
                {
                    ProtoBuf.Serializer.Serialize<SCRebelFightStart>(stream, start);
                    state.Start = stream.ToArray();
                }
            }
            catch (Exception error) { wanted = null; Debug.LogError("Wanted recovery snapshot failed: " + error.Message); return false; }
            state.FightId = start.fightId;
            state.Recoverable = false;
            state.InBattle = true;
            return false;
        }
        public static void WantedSettlement(object owner)
        {
            settlementOwner = owner;
            settlementFight = StaticData.curBattleData == null ? null : StaticData.curBattleData.fightId;
            settlementPlayer = StaticData.playerData;
            settlementSession = Session();
        }
        public static bool WantedResult(object owner, object packet, bool success)
        {
            // Suppress stale callbacks before they can mutate global result/reward state.
            if (owner != settlementOwner || settlementPlayer != StaticData.playerData || !SameSession(settlementSession) ||
                StaticData.curBattleData == null || StaticData.curBattleData.battleType != BattleType.WANTED ||
                settlementFight != StaticData.curBattleData.fightId) return true;
            if (success)
            {
                Packet received = packet as Packet;
                if (received != null && received.kBody is SCRebelFightEnd)
                {
                    if (Owned(wanted) && wanted.FightId == settlementFight) wanted = null;
                    settlementOwner = null;
                    return false;
                }
            }
            WantedState state = wanted;
            bool canResume = Owned(state) && state.Start != null && state.FightId == settlementFight;
            if (canResume) { state.Recoverable = true; state.InBattle = false; }
            settlementOwner = null;
            // Never call End(false): that enters WantedBalance with an absent or stale result packet.
            // ChangeUIBack synchronously reopens the existing page before HideLoading.
            // Its original DoOpen parses tab as Int32; repair it before that reopen.
            MercenaryTaskPage currentWantedPage = PageManager.Instance.currentPage as MercenaryTaskPage;
            if (currentWantedPage != null && currentWantedPage.options != null) currentWantedPage.options["tab"] = "1";
            // The original result-panel exit clears this presentation flag before returning.
            BattleManager.isBattleUI = false;
            ManagerController.Instance.BattleToRoom();
            BoxManager.CreateMsgBox(canResume ? "结算未完成，可继续本次挑战，次数不会重复扣除" : "结算未完成，请稍后重试");
            return true;
        }
        public static bool WantedResume(object page, int type)
        {
            WantedState state = wanted;
            if (!Owned(state)) { wanted = null; return false; }
            if (state.InBattle || state.RequestPending) return true;
            if (!state.Recoverable) return false;
            WantedMission view = page as WantedMission;
            if (view == null || !view.gameObject.activeInHierarchy) return true;
            if (Selected(view) != state.InstanceId || type != state.AttackType)
            {
                BoxManager.CreateMsgBox("请先继续未完成的悬赏挑战");
                return true;
            }
            if (NetAPI.IsSendingHttp()) return true;
            SCRebelFightStart start;
            using (MemoryStream stream = new MemoryStream(state.Start, false)) start = ProtoBuf.Serializer.Deserialize<SCRebelFightStart>(stream);
            if (start == null || start.fightId != state.FightId) return true;
            state.Recoverable = false;
            state.InBattle = true;
            state.Page = view;
            settlementOwner = null;
            StaticData.curBattleData.SetData(start);
            ManagerController.Instance.RoomToBattle(0);
            return true;
        }
        static FieldInfo Field(object page, string name)
        {
            return page.GetType().GetField(name, BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public);
        }
        public static void ResetSelection(object page)
        {
            if (page == null) return;
            IList clones = (IList)Field(page, "goList").GetValue(page);
            if (clones != null)
            {
                foreach (object value in clones)
                {
                    GameObject clone = value as GameObject;
                    if (clone != null) UnityEngine.Object.Destroy(clone);
                }
                clones.Clear();
            }
            IDictionary ids = (IDictionary)Field(page, "heroIdList").GetValue(page);
            if (ids != null) ids.Clear();
            Field(page, "selectId").SetValue(page, 0);
        }
    }
}

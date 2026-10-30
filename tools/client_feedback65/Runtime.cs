using System;
using System.Collections;
using System.Reflection;
using System.IO;
using ProtoBuf.Message;
using NetWork.Layer;
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
            public byte[] Session, Start, End;
            public string InstanceId, FightId;
            public int AttackType, StartGeneration, EndGeneration;
            public WantedMission Page;
            public bool Recoverable, InBattle, RequestPending, Reacquiring, EndPending, Rejected, Completed, FailureShown, RetryScheduled, ListReceived, MissingFromList, MissingCheckScheduled;
        }
        static WantedState wanted;
        static int wantedGeneration;
        const string WantedRetryableEndCode = "90005";
        const string WantedRecoveryPrefix = "wanted-recover:";
        const string WantedRecoveryCommitted = "本次悬赏挑战已结算";
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
            WantedState current = wanted;
            if (current != null && current.Reacquiring)
            {
                current.RequestPending = Owned(current) && current.Recoverable && current.Rejected &&
                    current.Page == view && current.AttackType == type && current.InstanceId == Selected(view);
                if (current.RequestPending) current.StartGeneration = ++wantedGeneration;
                return;
            }
            wanted = new WantedState { Player = StaticData.playerData, PlayerId = StaticData.playerData.playerId,
                Session = Session(), Page = view, InstanceId = Selected(view), AttackType = type, RequestPending = true, StartGeneration = ++wantedGeneration };
        }
        public static bool WantedPrepareStart(object page, object request)
        {
            WantedState state = wanted;
            CSRebelFightStart start = request as CSRebelFightStart;
            if (state == null || !state.Reacquiring) return true;
            if (!Owned(state) || !state.RequestPending || !System.Object.ReferenceEquals(state.Page, page) || start == null ||
                start.rebelId != state.InstanceId || start.type != state.AttackType ||
                String.IsNullOrEmpty(state.FightId) || WantedRecoveryPrefix.Length + state.FightId.Length > 64)
            {
                state.RequestPending = false;
                state.Reacquiring = false;
                return false;
            }
            // A named paid fight prevents a concurrent committed End from
            // turning this retry into a new charged Start.
            start.rebelId = WantedRecoveryPrefix + state.FightId;
            return true;
        }
        public static HttpHandler WantedWrapStart(HttpHandler callback)
        {
            WantedState state = wanted;
            if (!Owned(state) || !state.RequestPending) return callback;
            int generation = state.StartGeneration;
            return delegate(Packet packet, bool success)
            {
                if (wanted != state || !Owned(state) || !state.RequestPending || state.StartGeneration != generation) return;
                callback(packet, success);
            };
        }
        public static bool WantedStartSent(bool sent)
        {
            WantedState state = wanted;
            if (!sent && Owned(state) && state.RequestPending)
            {
                state.RequestPending = false;
                if (state.Reacquiring) state.Reacquiring = false;
                else wanted = null;
            }
            return sent;
        }
        public static bool WantedStart(object page, object packet, bool success)
        {
            WantedState state = wanted;
            if (!Owned(state) || !System.Object.ReferenceEquals(state.Page, page) || !state.RequestPending) return true;
            state.RequestPending = false;
            if (!success)
            {
                if (state.Reacquiring)
                {
                    state.Reacquiring = false;
                    Packet failure = packet as Packet;
                    if (WantedCommittedResponse(failure))
                    {
                        state.Completed = true;
                        wanted = null;
                        settlementOwner = null;
                        if (state.Page != null && state.Page.gameObject.activeInHierarchy)
                            state.Page.StartCoroutine(WantedRefreshAfterReceipt(state));
                    }
                    return true;
                }
                wanted = null;
                return false;
            }
            if (state.InstanceId != Selected(page))
            {
                if (state.Reacquiring) state.Reacquiring = false;
                else wanted = null;
                return true;
            }
            Packet received = packet as Packet;
            SCRebelFightStart start = received == null ? null : received.kBody as SCRebelFightStart;
            if (start == null || String.IsNullOrEmpty(start.fightId)) { state.Reacquiring = false; return true; }
            if (state.Reacquiring)
            {
                state.Reacquiring = false;
                try
                {
                    using (MemoryStream stream = new MemoryStream())
                    {
                        ProtoBuf.Serializer.Serialize<SCRebelFightStart>(stream, start);
                        if (start.fightId != state.FightId || !WantedSameBytes(state.Start, stream.ToArray())) return true;
                    }
                }
                catch (Exception error) { Debug.LogError("Wanted paid Start validation failed: " + error.Message); return true; }
                state.End = null;
                state.EndPending = false;
                state.Rejected = false;
                state.FailureShown = false;
                state.Recoverable = false;
                state.InBattle = true;
                settlementOwner = null;
                return false;
            }
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
            WantedState state = wanted;
            if (!Owned(state) || !state.InBattle || state.FightId != settlementFight) return;
            state.EndGeneration = ++wantedGeneration;
            state.End = null;
            state.EndPending = true;
            state.Rejected = false;
            state.FailureShown = false;
        }
        public static void WantedCaptureEnd(object owner, object request)
        {
            WantedState state = wanted;
            CSRebelFightEnd end = request as CSRebelFightEnd;
            if (!Owned(state) || !state.EndPending || owner != settlementOwner || end == null || end.fightId != state.FightId) return;
            try { state.End = WantedEncodeEnd(end); }
            catch (Exception error) { state.End = null; Debug.LogError("Wanted End snapshot failed: " + error.Message); }
        }
        static byte[] WantedEncodeEnd(CSRebelFightEnd end)
        {
            using (MemoryStream stream = new MemoryStream())
            {
                ProtoBuf.Serializer.Serialize<CSRebelFightEnd>(stream, end);
                return stream.ToArray();
            }
        }
        static bool WantedSameBytes(byte[] a, byte[] b)
        {
            if (a == null || b == null || a.Length != b.Length) return false;
            for (int i = 0; i < a.Length; i++) if (a[i] != b[i]) return false;
            return true;
        }
        public static HttpHandler WantedWrapEnd(HttpHandler callback)
        {
            WantedState state = wanted;
            if (!Owned(state) || !state.EndPending) return callback;
            int generation = state.EndGeneration;
            HttpHandler wrapped = delegate(Packet packet, bool success)
            {
                if (wanted != state || !Owned(state) || state.Completed || state.EndGeneration != generation) return;
                callback(packet, success);
            };
            return wrapped;
        }
        public static bool WantedEndSent(bool sent)
        {
            WantedState state = wanted;
            if (!sent && Owned(state) && state.EndPending) WantedFailure(state, null);
            return sent;
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
                    WantedState acceptedState = wanted;
                    if (Owned(acceptedState) && acceptedState.FightId == settlementFight)
                    {
                        if (!acceptedState.InBattle) { WantedCompleteOutsideBattle(acceptedState, received.kBody as SCRebelFightEnd); return true; }
                        acceptedState.Completed = true;
                        wanted = null;
                    }
                    settlementOwner = null;
                    return false;
                }
            }
            WantedState state = wanted;
            if (Owned(state) && state.FightId == settlementFight)
            {
                if (WantedCommittedResponse(packet as Packet))
                {
                    bool inBattle = state.InBattle;
                    state.Completed = true;
                    state.EndPending = false;
                    state.InBattle = false;
                    wanted = null;
                    settlementOwner = null;
                    if (inBattle) WantedLeaveBattle();
                    else if (state.Page != null && state.Page.gameObject.activeInHierarchy)
                        state.Page.StartCoroutine(WantedRefreshAfterReceipt(state));
                    return true;
                }
                WantedFailure(state, packet as Packet);
                return true;
            }
            settlementOwner = null;
            WantedLeaveBattle();
            BoxManager.CreateMsgBox("结算未完成，请稍后重试");
            return true;
        }
        static bool WantedCommittedResponse(Packet packet)
        {
            ErrorMessage error = packet == null ? null : packet.kBody as ErrorMessage;
            return packet != null && packet.nOpCode == 10001 && error != null && error.code == "90003" && error.msg == WantedRecoveryCommitted;
        }
        static void WantedFailure(WantedState state, Packet packet)
        {
            if (wanted != state || !Owned(state) || state.Completed) return;
            ErrorMessage error = packet == null ? null : packet.kBody as ErrorMessage;
            if (error != null && (error.code == "21" || error.code == "90002"))
            {
                wanted = null;
                settlementOwner = null;
                return;
            }
            state.EndPending = false;
            state.Rejected = error != null && packet.nOpCode == 10001 && error.code == WantedRetryableEndCode;
            state.Recoverable = state.Start != null && state.FightId == settlementFight;
            if (state.InBattle)
            {
                state.InBattle = false;
                WantedLeaveBattle();
            }
            if (state.Rejected && state.MissingFromList) WantedScheduleMissing(state);
            if (state.FailureShown) return;
            state.FailureShown = true;
            BoxManager.CreateMsgBox(state.Rejected && state.Recoverable
                ? "结算未完成，可继续本次挑战，次数不会重复扣除"
                : "结算尚未确认，请点击原挑战按钮重试结算");
        }
        static void WantedLeaveBattle()
        {
            // Never call End(false): that enters WantedBalance with an absent or stale result packet.
            // ChangeUIBack synchronously reopens the existing page before HideLoading.
            // Its original DoOpen parses tab as Int32; repair it before that reopen.
            MercenaryTaskPage currentWantedPage = PageManager.Instance.currentPage as MercenaryTaskPage;
            if (currentWantedPage != null && currentWantedPage.options != null) currentWantedPage.options["tab"] = "1";
            // The original result-panel exit clears this presentation flag before returning.
            BattleManager.isBattleUI = false;
            ManagerController.Instance.BattleToRoom();
        }
        public static bool WantedResume(object page, int type)
        {
            WantedState state = wanted;
            if (!Owned(state)) { wanted = null; return false; }
            if (state.InBattle || state.RequestPending || state.EndPending) return true;
            if (!state.Recoverable) return false;
            WantedMission view = page as WantedMission;
            if (view == null || !view.gameObject.activeInHierarchy) return true;
            if (Selected(view) != state.InstanceId || type != state.AttackType)
            {
                BoxManager.CreateMsgBox("请先继续未完成的悬赏挑战");
                return true;
            }
            if (NetAPI.IsSendingHttp()) return true;
            if (!state.Rejected) { WantedRetryEnd(state, false); return true; }
            MethodInfo send = view.GetType().GetMethod("SendEnterBattle", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (send == null) return true;
            state.EndGeneration = ++wantedGeneration;
            state.EndPending = false;
            state.Page = view;
            state.Reacquiring = true;
            state.RequestPending = true;
            try { send.Invoke(view, new object[] { type }); }
            catch (Exception error)
            {
                state.RequestPending = false;
                state.Reacquiring = false;
                Debug.LogError("Wanted recovery Start failed: " + error.Message);
            }
            return true;
        }
        public static void WantedPageShown(object page)
        {
            WantedState state = wanted;
            WantedMission view = page as WantedMission;
            if (!Owned(state) || !state.Recoverable || state.EndPending || view == null || !view.gameObject.activeInHierarchy) return;
            state.Page = view;
            if (state.Rejected || state.End == null || state.RetryScheduled) return;
            state.RetryScheduled = true;
            state.ListReceived = false;
            view.StartCoroutine(WantedRetryAfterList(state, state.EndGeneration));
        }
        public static void WantedListReceived(object page, object packet, bool success)
        {
            WantedState state = wanted;
            Packet received = packet as Packet;
            SCGetRebels list = received == null ? null : received.kBody as SCGetRebels;
            if (!Owned(state) || !System.Object.ReferenceEquals(state.Page, page) || !success || list == null) return;
            if (state.RetryScheduled) state.ListReceived = true;
            if (!state.Recoverable || state.InBattle || state.RequestPending || list.rebels == null) return;
            bool found = false;
            foreach (RebelInfo boss in list.rebels)
                if (boss != null && boss.id == state.InstanceId) { found = true; break; }
            state.MissingFromList = !found;
            if (state.Rejected && state.MissingFromList) WantedScheduleMissing(state);
        }
        static void WantedScheduleMissing(WantedState state)
        {
            WantedMission view = state.Page;
            if (state.MissingCheckScheduled || view == null || !view.gameObject.activeInHierarchy) return;
            state.MissingCheckScheduled = true;
            view.StartCoroutine(WantedRetireMissing(state, state.EndGeneration));
        }
        static IEnumerator WantedRetireMissing(WantedState state, int generation)
        {
            // SCPlayer in the same response is applied after the list callback.
            yield return null;
            state.MissingCheckScheduled = false;
            if (wanted == state && Owned(state) && state.EndGeneration == generation && state.Recoverable && state.Rejected &&
                state.MissingFromList && !state.InBattle && !state.RequestPending && !state.EndPending)
            {
                state.Completed = true;
                wanted = null;
                settlementOwner = null;
            }
        }
        static IEnumerator WantedRetryAfterList(WantedState state, int generation)
        {
            // The original list reply carries SCPlayer after its list callback.
            // Wait for that callback and one more frame so its snapshot is applied.
            yield return null;
            float deadline = Time.realtimeSinceStartup + 10f;
            while (wanted == state && Owned(state) && state.EndGeneration == generation && !state.ListReceived &&
                   Time.realtimeSinceStartup < deadline) yield return null;
            if (state.ListReceived) yield return null;
            while (wanted == state && Owned(state) && state.EndGeneration == generation &&
                   NetAPI.IsSendingHttp() && Time.realtimeSinceStartup < deadline) yield return null;
            state.RetryScheduled = false;
            if (wanted == state && Owned(state) && state.EndGeneration == generation && state.ListReceived && state.Recoverable &&
                !state.Rejected && !state.EndPending && !NetAPI.IsSendingHttp()) WantedRetryEnd(state, true);
        }
        static void WantedRetryEnd(WantedState state, bool automatic)
        {
            if (wanted != state || !Owned(state) || state.End == null)
            {
                BoxManager.CreateMsgBox("结算数据无法读取，请重新登录后查看进度");
                return;
            }
            CSRebelFightEnd end;
            try
            {
                using (MemoryStream stream = new MemoryStream(state.End, false)) end = ProtoBuf.Serializer.Deserialize<CSRebelFightEnd>(stream);
                if (end == null || end.fightId != state.FightId || !WantedSameBytes(state.End, WantedEncodeEnd(end)))
                {
                    BoxManager.CreateMsgBox("结算数据无法读取，请重新登录后查看进度");
                    return;
                }
            }
            catch (Exception) { BoxManager.CreateMsgBox("结算数据无法读取，请重新登录后查看进度"); return; }
            int generation = state.EndGeneration = ++wantedGeneration;
            state.EndPending = true;
            if (!automatic) state.FailureShown = false;
            bool completed = false;
            HttpHandler handler = delegate(Packet packet, bool success)
            {
                if (wanted != state || !Owned(state) || state.Completed || completed || state.EndGeneration != generation) return;
                completed = true;
                state.EndPending = false;
                if (success && packet != null && packet.kBody is SCRebelFightEnd)
                    WantedCompleteOutsideBattle(state, packet.kBody as SCRebelFightEnd);
                else WantedFailure(state, packet);
            };
            if (!NetAPI.SendHttp(55011, end, handler)) WantedFailure(state, null);
        }
        static void WantedCompleteOutsideBattle(WantedState state, SCRebelFightEnd accepted)
        {
            if (wanted != state || !Owned(state) || state.Completed || accepted == null) return;
            state.Completed = true;
            state.EndPending = false;
            wanted = null;
            settlementOwner = null;
            // The list query may already have applied the committed SCPlayer
            // snapshot. Refresh from the server instead of applying End deltas.
            if (state.Page != null && state.Page.gameObject.activeInHierarchy)
                state.Page.StartCoroutine(WantedRefreshAfterReceipt(state));
            BoxManager.CreateMsgBox("本次悬赏已结算");
        }
        static IEnumerator WantedRefreshAfterReceipt(WantedState state)
        {
            yield return null;
            float deadline = Time.realtimeSinceStartup + 10f;
            while (Owned(state) && NetAPI.IsSendingHttp() && Time.realtimeSinceStartup < deadline) yield return null;
            if (!Owned(state) || NetAPI.IsSendingHttp() || state.Page == null || !state.Page.gameObject.activeInHierarchy) yield break;
            MethodInfo show = state.Page.GetType().GetMethod("Show", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (show != null) show.Invoke(state.Page, null);
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

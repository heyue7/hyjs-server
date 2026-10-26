using System;
using System.Collections;
using System.IO;
using System.Reflection;
using ProtoBuf.Message;
using NetWork.Layer;
using NetWork.Packets;
using Battle;
using Room;
using UI;
using UnityEngine;

namespace HYJS.Feedback66
{
    public static class TrialRuntime
    {
        sealed class State
        {
            public object Player, Entry, EndOwner;
            public HttpHandler StartHandler, EndHandler;
            public long PlayerId;
            public byte[] Session, StartRequest, PaidStart, End;
            public int Mode, Star, StartGeneration, EndGeneration;
            public string Task, Fight;
            public bool StartPending, StartRetryable, StartCallbackDone, InBattle, EndPending, NativeDone, Recoverable, Rejected, RewardApplied;
        }
        static State state;
        static int generation;
        const string Conflict = "战斗实例或录像内容与已冻结记录冲突";
        static byte[] Session() { return StaticData.sessionKey == null ? null : (byte[])StaticData.sessionKey.Clone(); }
        static bool SameSession(byte[] expected)
        {
            byte[] actual = StaticData.sessionKey;
            if (expected == null || actual == null || expected.Length != actual.Length) return false;
            for (int i = 0; i < actual.Length; i++) if (actual[i] != expected[i]) return false;
            return true;
        }
        static bool Owned(State value)
        {
            return value != null && StaticData.playerData != null && value.Player == StaticData.playerData &&
                value.PlayerId == StaticData.playerData.playerId && SameSession(value.Session);
        }
        static bool Current(State value) { return state == value && Owned(value); }
        static int BattleMode(int mode) { return mode == 1 ? 9 : 10; }
        static bool BattleOwned(State value)
        {
            return Current(value) && StaticData.curBattleData != null &&
                (int)StaticData.curBattleData.battleType == BattleMode(value.Mode) && StaticData.curBattleData.fightId == value.Fight;
        }
        static FieldInfo Field(object value, string name)
        {
            return value.GetType().GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
        }
        static object FieldValue(object value, string name)
        {
            if (value == null) return null;
            FieldInfo field = Field(value, name);
            return field == null ? null : field.GetValue(value);
        }
        static object StartEntry(object callback, int mode)
        {
            return mode == 1 ? FieldValue(callback, "<>f__this") : callback;
        }
        static byte[] Encode<T>(T value)
        {
            using (MemoryStream stream = new MemoryStream())
            {
                ProtoBuf.Serializer.Serialize<T>(stream, value);
                return stream.ToArray();
            }
        }
        static T Decode<T>(byte[] bytes)
        {
            using (MemoryStream stream = new MemoryStream(bytes, false)) return ProtoBuf.Serializer.Deserialize<T>(stream);
        }
        static bool SameBytes(byte[] a, byte[] b)
        {
            if (a == null || b == null || a.Length != b.Length) return false;
            for (int i = 0; i < a.Length; i++) if (a[i] != b[i]) return false;
            return true;
        }
        static bool MatchesEntry(State value, object entry, int mode)
        {
            if (value.Mode != mode) return false;
            string task = Convert.ToString(FieldValue(entry, mode == 1 ? "taskId" : "currentSelectBossId"));
            int star = mode == 1 ? Convert.ToInt32(FieldValue(entry, "star")) : 0;
            return task == value.Task && star == value.Star;
        }
        static void Message(string text) { BoxManager.CreateMsgBox(text); }
        // Both typed9 and typed10 use CopyBalance.Exit -> BattleToRush in the pinned APK.
        static void LeaveBattle(State value)
        {
            if (!BattleOwned(value) || !value.InBattle) return;
            value.InBattle = false;
            BattleManager.isBattleUI = false;
            ManagerController.Instance.BattleToRush();
        }
        static void Failure(State value, Packet packet)
        {
            if (!Current(value)) return;
            ErrorMessage error = packet == null ? null : packet.kBody as ErrorMessage;
            if (error != null && (error.code == "21" || error.code == "90002"))
            {
                state = null;
                return;
            }
            value.EndPending = false;
            value.Recoverable = true;
            value.Rejected = error != null && packet.nOpCode == 10001 && error.code == "90003" &&
                (error.msg == Conflict || error.msg != null && error.msg.StartsWith("录像操作不符合原技能时序:", StringComparison.Ordinal));
            LeaveBattle(value);
            Message(value.Rejected && value.PaidStart != null
                ? "战斗录像校验未通过，请点击挑战重新进行本次战斗，次数不会重复扣除"
                : "结算未确认，请点击原挑战按钮重试本次结算");
        }
        public static bool Click(object entry, int mode)
        {
            State value = state;
            if (!Owned(value)) { state = null; return false; }
            if (value.StartPending || value.InBattle || value.EndPending) return true;
            if (value.StartRetryable)
            {
                if (!MatchesEntry(value, entry, mode))
                {
                    Message("请先在原关卡继续未完成的试炼挑战");
                    return true;
                }
                // The same original entry issues a new generation. The server resumes its
                // matching pending Start; callbacks captured by the previous generation expire.
                return NetAPI.IsSendingHttp();
            }
            if (!value.Recoverable) return false;
            if (!MatchesEntry(value, entry, mode))
            {
                Message("请先在原关卡继续未完成的试炼挑战");
                return true;
            }
            if (NetAPI.IsSendingHttp()) return true;
            if (value.Rejected && value.PaidStart != null) Resume(value, entry);
            else RetryEnd(value);
            return true;
        }
        // A committed End may already have advanced the server's floor. Retry it
        // from the original main button before opening the next floor's Ready page.
        public static bool PageClick(object entry, int mode)
        {
            State value = state;
            if (!Owned(value)) { state = null; return false; }
            if (mode != 1 || value.Mode != mode || !value.Recoverable) return false;
            if (value.StartPending || value.InBattle || value.EndPending || NetAPI.IsSendingHttp()) return true;
            if (value.Rejected && value.PaidStart != null && value.Entry != null) Resume(value, value.Entry);
            else if (!value.Rejected) RetryEnd(value);
            else return false;
            return true;
        }
        public static void Request(object entry, object request, int mode)
        {
            CSRushFightStart start = request as CSRushFightStart;
            if (start == null || StaticData.playerData == null || start.rushType != mode) return;
            state = new State { Player = StaticData.playerData, PlayerId = StaticData.playerData.playerId,
                Session = Session(), Entry = entry, Mode = mode, Task = start.taskId, Star = start.star,
                StartGeneration = ++generation, StartPending = true };
            try { state.StartRequest = Encode<CSRushFightStart>(start); }
            catch (Exception error) { Debug.LogError("Trial Start snapshot failed: " + error.Message); }
        }
        public static HttpHandler WrapStart(HttpHandler callback)
        {
            State value = state;
            int requestGeneration = value == null ? 0 : value.StartGeneration;
            HttpHandler handler = delegate(Packet packet, bool success)
            {
                if (!Current(value) || value.StartCallbackDone || (!value.StartPending && !value.StartRetryable) ||
                    value.StartGeneration != requestGeneration) return;
                callback(packet, success);
            };
            if (value != null) value.StartHandler = handler;
            return handler;
        }
        public static bool StartSent(bool sent)
        {
            if (!sent && state != null && state.StartPending) state = null;
            return sent;
        }
        public static bool Start(object callbackOwner, object packet, bool success, int mode)
        {
            State value = state;
            if (!Current(value) || value.StartCallbackDone || (!value.StartPending && !value.StartRetryable) || value.Mode != mode ||
                !System.Object.ReferenceEquals(value.Entry, StartEntry(callbackOwner, mode))) return true;
            value.StartCallbackDone = true;
            value.StartPending = false;
            value.StartRetryable = false;
            if (!success) { state = null; return false; }
            Packet received = packet as Packet;
            SCRushFightStart start = received == null ? null : received.kBody as SCRushFightStart;
            if (start == null || String.IsNullOrEmpty(start.fightId) || start.rushType != mode) { state = null; return true; }
            try { value.PaidStart = Encode<SCRushFightStart>(start); }
            catch (Exception error)
            {
                state = null;
                Debug.LogError("Trial recovery snapshot failed: " + error.Message);
                Message("挑战数据未能保存，请重新进入试炼");
                return true;
            }
            value.Fight = start.fightId;
            value.InBattle = true;
            return false;
        }
        public static bool BeginEnd(object owner, int mode)
        {
            State value = state;
            if (!BattleOwned(value) || value.Mode != mode || !value.InBattle || value.EndPending) return true;
            value.EndOwner = owner;
            value.EndGeneration = ++generation;
            value.NativeDone = false;
            value.EndPending = true;
            value.Recoverable = false;
            value.Rejected = false;
            return false;
        }
        public static void CaptureEnd(object owner, object request, int mode)
        {
            State value = state;
            CSRushFightEnd end = request as CSRushFightEnd;
            if (!BattleOwned(value) || value.EndOwner != owner || value.Mode != mode || end == null || end.fightId != value.Fight) return;
            try { value.End = Encode<CSRushFightEnd>(end); }
            catch (Exception error) { value.End = null; Debug.LogError("Trial End snapshot failed: " + error.Message); }
        }
        public static HttpHandler WrapEnd(HttpHandler callback)
        {
            State value = state;
            int requestGeneration = value == null ? 0 : value.EndGeneration;
            HttpHandler handler = delegate(Packet packet, bool success)
            {
                if (!Current(value) || value.NativeDone || value.EndGeneration != requestGeneration) return;
                callback(packet, success);
            };
            if (value != null) value.EndHandler = handler;
            return handler;
        }
        public static bool EndSent(bool sent)
        {
            if (!sent && Current(state) && state.EndPending) Failure(state, null);
            return sent;
        }
        public static bool Result(object owner, object packet, bool success, int mode)
        {
            State value = state;
            if (!Current(value) || value.NativeDone || value.EndOwner != owner || value.Mode != mode) return true;
            Packet received = packet as Packet;
            SCRushFightEnd end = received == null ? null : received.kBody as SCRushFightEnd;
            value.NativeDone = true;
            value.EndPending = false;
            if (success && end != null)
            {
                bool original = value.InBattle && BattleOwned(value);
                if (original) { state = null; return false; }
                CompleteOutsideBattle(value, end);
                return true;
            }
            Failure(value, received);
            return true;
        }
        // Main HTTP timeout does not invoke its callback. Only the currently owned exact End is observed.
        public static void TransportError(object packet, object callback, int error)
        {
            State value = state;
            Packet sent = packet as Packet;
            CSRushFightStart start = sent == null ? null : sent.kBody as CSRushFightStart;
            if (Current(value) && value.StartPending && !value.StartCallbackDone && start != null && sent.nOpCode == 54501 &&
                start.rushType == value.Mode && start.taskId == value.Task && start.star == value.Star &&
                System.Object.ReferenceEquals(value.StartHandler, callback))
            {
                try { if (!SameBytes(value.StartRequest, Encode<CSRushFightStart>(start))) return; }
                catch (Exception) { return; }
                // Main HTTP timeout keeps its original handler for the timeout-box resend.
                // Permit that same generation's late response until an explicit new request replaces it.
                value.StartPending = false;
                value.StartRetryable = true;
                return;
            }
            CSRushFightEnd end = sent == null ? null : sent.kBody as CSRushFightEnd;
            if (!Current(value) || !value.EndPending || end == null || sent.nOpCode != 54503 || end.fightId != value.Fight ||
                !System.Object.ReferenceEquals(value.EndHandler, callback)) return;
            try { if (!SameBytes(value.End, Encode<CSRushFightEnd>(end))) return; }
            catch (Exception) { return; }
            // Keep this generation eligible for the original timeout-box resend. A newer retry invalidates it.
            Failure(value, null);
        }
        static void RetryEnd(State value)
        {
            if (!Current(value)) return;
            if (value.End == null) { Message("结算数据无法读取，请重新登录后查看进度"); return; }
            CSRushFightEnd end;
            try { end = Decode<CSRushFightEnd>(value.End); }
            catch (Exception) { Message("结算数据无法读取，请重新登录后查看进度"); return; }
            if (end == null || end.fightId != value.Fight) return;
            try
            {
                if (!SameBytes(value.End, Encode<CSRushFightEnd>(end)))
                {
                    Message("结算数据无法完整恢复，请重新登录后查看进度");
                    return;
                }
            }
            catch (Exception) { Message("结算数据无法读取，请重新登录后查看进度"); return; }
            int requestGeneration = value.EndGeneration = ++generation;
            value.NativeDone = true;
            value.EndPending = true;
            value.Rejected = false;
            bool completed = false;
            HttpHandler handler = delegate(Packet packet, bool success)
            {
                if (!Current(value) || completed || value.EndGeneration != requestGeneration) return;
                completed = true;
                value.EndPending = false;
                if (success && packet != null && packet.kBody is SCRushFightEnd)
                {
                    CompleteOutsideBattle(value, packet.kBody as SCRushFightEnd);
                    return;
                }
                Failure(value, packet);
            };
            value.EndHandler = handler;
            bool sent = NetAPI.SendHttp(54503, end, handler);
            if (!sent) Failure(value, null);
        }
        static void Resume(State value, object entry)
        {
            if (!Current(value) || !value.Rejected || value.PaidStart == null) return;
            SCRushFightStart start;
            try { start = Decode<SCRushFightStart>(value.PaidStart); }
            catch (Exception) { Message("挑战数据无法读取，请重新登录后查看进度"); return; }
            if (start == null || start.fightId != value.Fight || start.rushType != value.Mode) return;
            // End wrappers for the previous attempt become stale before the global graph changes.
            value.EndGeneration = ++generation;
            value.NativeDone = true;
            value.EndPending = false;
            value.Recoverable = false;
            value.Rejected = false;
            value.End = null;
            value.Entry = entry;
            value.InBattle = true;
            if (value.Mode == 1)
            {
                RoomRush room = RoomManager.currentRoom as RoomRush;
                if (room == null) { value.InBattle = false; value.Recoverable = true; value.Rejected = true; return; }
                room.MainActorRush();
                room.isReadyForBattle = false;
                StaticData.curBattleData.SetData(start, true);
                room.isReadyForBattle = true;
                Page page = FieldValue(entry, "curPage") as Page;
                if (page != null && PageManager.Instance.currentPage == page) page.Close();
            }
            else
            {
                MethodInfo transition = entry.GetType().GetMethod("ToBattle", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
                MonoBehaviour page = entry as MonoBehaviour;
                if (transition == null || page == null) { value.InBattle = false; value.Recoverable = true; value.Rejected = true; return; }
                StaticData.curBattleData.SetData(start, false);
                page.StartCoroutine((IEnumerator)transition.Invoke(entry, null));
            }
        }
        static void CompleteOutsideBattle(State value, SCRushFightEnd accepted)
        {
            if (!Current(value) || accepted == null || value.RewardApplied) return;
            // Consume ownership before AddResultObj can trigger UI callbacks. In-battle success
            // keeps the original CopyBalance path; only detached success reaches this method.
            value.RewardApplied = true;
            state = null;
            // The original typed9/10 CopyBalance.AddBalanceReward applies all inventory deltas.
            // HTTPManager.Tick applies queued SCPlayer scalar snapshots after the End callback.
            if (accepted.resultObj != null) StaticData.playerData.AddResultObj(accepted.resultObj);
            RefreshPage();
            Message("本次挑战已结算");
        }
        static void RefreshPage()
        {
            Page page = PageManager.Instance.currentPage;
            if (!(page is RushPage) && !(page is RushEliteBossPage)) return;
            MethodInfo reopen = page.GetType().GetMethod("DoReopen", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (reopen != null) reopen.Invoke(page, null);
        }
    }
}

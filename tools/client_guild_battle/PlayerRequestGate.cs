using System;

namespace HYJS.GuildBattle {
// A single local admission intent. This never grants a battle or changes its result.
public sealed class PlayerRequestGate {
 private int scope, selfLife, targetLife, attempts;
 private long target;
 private double sentAt;
 private bool fighting, observedFightState;
 public bool HasIntent { get { return scope != 0; } }
 public long Target { get { return target; } }
 public void Reset() { scope = selfLife = targetLife = attempts = 0; target = 0; sentAt = 0; fighting = observedFightState = false; }
 private void Bind(int generation, int life) {
  if (scope == generation && selfLife == life) return;
  Reset(); scope = generation; selfLife = life;
 }
 public bool TrySend(int generation, int life, long candidate, int candidateLife, double now) {
  if (generation <= 0 || life < 0 || candidate <= 0 || candidateLife < 0 || double.IsNaN(now) || double.IsInfinity(now)) return false;
  Bind(generation,life);
  if (fighting) return false;
  if (attempts > 0) {
   // An already sent request cannot be cancelled by choosing another target.
   if (now < sentAt || now - sentAt < 2.0) return false;
   if (target != candidate || targetLife != candidateLife) attempts = 0;
   if (attempts >= 3) return false;
  }
  target = candidate; targetLife = candidateLife; sentAt = now; ++attempts;
  return true;
 }
 public void ObserveBegin(int generation, int life) { Bind(generation,life); fighting = true; observedFightState = false; }
 public void ObserveFightState(int generation, int life, bool inFight) {
  Bind(generation,life);
  if (!fighting) return;
  if (inFight) observedFightState = true;
  else if (observedFightState) Reset();
 }
 public void ObserveEnd(int generation, int life) {
  // An old End cannot reopen a newer request that is still awaiting Begin.
  if (scope == generation && selfLife == life && fighting) Reset();
 }
 public void OutsideRange(int generation, int life, long candidate, double now) {
  Bind(generation,life);
  if (!fighting && target == candidate && attempts > 0 && now - sentAt >= 2.0) {
   attempts = 0; target = 0;
  }
 }
 public void ExplicitClick(int generation, int life, long candidate, double now) {
  Bind(generation,life);
  if (!fighting && target == candidate && attempts >= 3 && now - sentAt >= 2.0) {
   attempts = 0; target = 0;
  }
 }
}
}

#!/usr/bin/env python3
"""Run the fixed player-concurrency cases on managed, isolated InnoDB schemas."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL

CASES = {
    'TestPlayerRank48SameZoneReadOnly': 'p1',
    'TestPlayerRank48ReadBoundaries': 'p1',
    'TestLeaderboardFreshnessCSRankCommittedAndRollbackMySQL': 'p1',
    'TestLeaderboardFreshnessP11RankRefreshesOtherProfileMySQL': 'p11',
    'TestP10FamousRefreshAndPersonalWorship': 'p10',
    'TestP10FamousMySQLReadUnderCallerWriteLock': 'p10',
    'TestP10FamousColdAndWarmQueriesUseCurrentPower': 'p10',
    'TestP10WorshipActualRankSourceCoinDailyThreeSharedTargetsConcurrencyAndCold': 'p10',
    'TestP10WorshipModelFollowsFirstFormationSlotAfterReload': 'p10',
    'TestP11ArenaShowsThreeHigherRanksAndReplaysOffers': 'p11',
    'TestP11ArenaFightThirdHigherRankSettlesOnce': 'p11',
    'TestP12WorldBossRankReadOnlyWindowAndCommittedDamage': 'p12',
    'TestP12WorldBossRankReadRejectsInvalidOwnershipAndSnapshots': 'p12',
    'TestP12WorldBossRankReadIdentityBatches': 'p12',
    'TestP12WorldBossRankReadMySQLSnapshotUnderWriteLocks': 'p12',
    'TestP12WorldBossAbsoluteOriginalWindowsScheduledActiveEndedAndNoDuplicate': 'p12',
    'TestP12WorldBossBattleConcurrentFinishCapsSharedHPAndSingleKiller': 'p12',
    'TestP12WorldBossBattleBoostActualInputAndRewardCenter': 'p12',
    'TestP13MonthlyUnverifiedWeekLedgerNeverRanks': 'p13',
    'TestP14PromotionRankFrozenLocalRuleRequiresEarlyPublicationAndRealCohort': 'p14',
    'TestP14PromotionRankOriginalThreeValueProjectionKeepsJointIdentity': 'p14',
    'TestP14PromotionRankContiguousProgressAndFinalSealCannotBeReopened': 'p14',
    'TestCopyRunRecoveryTransactionsMySQL': 'p1',
    'TestLuxuryDragonStorySweepCyclesAndPaidClaim': 'p1',
    'TestLuxuryDragonTriggerRollsBackWithStory': 'p1',
    'TestLuxuryDragonConcurrentRenewalMySQL': 'p1',
    'TestP3LuxuryOriginalChapterSequenceAndPaidCold': 'p1',
    'TestLocalPlayPaymentOfferAsOfConfirmedTime': 'p1',
    'TestP3PaymentDomain/luxury_offer_query_and_paid_window': 'p3',
    'TestSceneRound2MySQLIndependentRoomsAndMovement': 'p10',
    'TestSceneRound2MySQLLastRoomSlot': 'p10',
    'TestSceneRound2MySQLReverseTransfers': 'p10',
    'TestSceneRound2MySQLOldDisconnectKeepsReplacement': 'p10',
    'TestConnectionDrainRound2MySQLReplacementBarrier': 'p10',
    'TestP11PlunderSweepReuseRound2MySQLFiveRealChildren': 'p11',
    'TestP11ArenaRealRegistryUniqueRanksMatchSnapshotsAndNoResources': 'p11',
    'TestP11ArenaConcurrentRealRegistrationReceiptAndRestart': 'p11',
    'TestP11ArenaConcurrencyRound2MySQLIndependentAfterFirstReceipt': 'p11',
    'TestP11ArenaVirtualUntouchedLegacySeatMigratesOnlyOnce': 'p11',
    'TestGGOBossRound2MySQLNativeStartEndAndColdReceipt': 'p1',
    'TestGGOBossRound2MySQLSharedLastHitRollbackAndLines': 'p1',
    'TestP10LuckyDropRound2MySQLCapacityAndIsolation': 'p10',
    'TestFieldBossLines85RealFightColdAndIndependentRespawn': 'p1',
    'TestFieldBossLines86HandshakeFirstReply': 'p1',
    'TestP11ArenaVirtualLegacyPositionsAndLazyTopRank': 'p11',
    'TestP11ArenaVirtualOriginalGuideFightSlotConflictAndColdReceipt': 'p11',
    'TestP11ArenaConcurrencyRound2MySQLStablePolicyReaders': 'p11',
    'TestP11ArenaConcurrencyRound2MySQLVirtualRankArbitration': 'p11',
    'TestP11PlunderSweepRound2MySQLActualParticipantLocks': 'p11',
    'TestP11PlunderSweepActualIndependentChildrenEnergyStopAndCold/five-real-outcomes': 'p11',
    'TestP11PlunderSweepActualIndependentChildrenEnergyStopAndCold/actual-energy-stop': 'p11',
    'TestP11PlunderSweepActualIndependentChildrenEnergyStopAndCold/actual-fragment-stop': 'p11',
    'TestP11ArenaScopedRealWinnerPaymentRanksConcurrentColdAndQualification': 'p11',
    'TestP11ArenaDailyRealChallengerCutoffOfflineClaimAndColdOnce': 'p11',
    'TestPlayerConcurrencyMySQLP7PolicyIsolation': 'p7',
    'TestPlayerConcurrencyMySQLGuildEconomyIsolation': 'p13',
    'TestPlayerConcurrencyMySQLGuildMembershipIsolation': 'p13',
    'TestPlayerConcurrencyMySQLGuildMonthlyIdentityBoundary': 'p13',
    'TestP13GuildOriginalManagementPermissionsCooldownAndRestart': 'p13',
    'TestP13GuildConcurrentApprovalTransferUniqueMembershipAndReceipts': 'p13',
    'TestP13MonthlyPublishedBoundaryAndNoRetro': 'p13',
    'TestP13GuildGiftMonthlyLegacyDisabledRollbackAndCorruption': 'p13',
    'TestP13GuildOriginalEconomyConcurrentDonationDailyCapsAndAtomicFailure': 'p13',
    'TestP13GuildOriginalWeightedNameNoticeDeclarationBudgets': 'p13',
    'TestP13GuildTechRealFundsOriginalUpgradeOnceFrozenColdAndMembership': 'p13',
    'TestPlayerConcurrencyMySQLTeamIsolation': 'p13',
    'TestPlayerConcurrencyMySQLGrabIsolation': 'p11',
    'TestPlayerConcurrencyMySQLGrabSameSlot': 'p11',
    'TestPlayerConcurrencyMySQLGrabFrozenOffers': 'p11',
    'TestP11GrabOriginalEmptySlotAtomicOccupationBuyGiveupAndFullTimeRewards': 'p11',
    'TestP11GrabRandomActualSegmentsOriginalRewardUseDayCapCold': 'p11',
    'TestP11GrabRandomPrepublicationAndRealNoOccupation': 'p11',
    'TestP11GrabFightRealOwnerIndependentGateCostAndColdReceipt': 'p11',
    'TestP11GrabFightRulesNeverBorrowModeOrUnpublishedPolicy': 'p11',
    'TestP7OriginalEliteDirectoryVIPBudgetsQueriesAndDayReset': 'p7',
    'TestP13TeamOriginalManagementInvitationsLeaderPermissionsExitAndRecovery': 'p13',
    'TestP13TeamConcurrentAcceptUniqueMembershipExactReceiptAndAtomicInviteFailure': 'p13',
    'TestP13TeamFrozenFiveMemberCapacityConcurrentLastSlotAndLeaveRevokesOldInvite': 'p13',
    'TestPlayerConcurrencyMySQLPrivateFirstFreeze': 'p1',
    'TestAdminCharacterScheduleSharedMySQL': 'p4',
    'TestFeedback81MySQLConcurrentClaim': 'p4',
    'TestPlayerConcurrencyMySQLGuildLoginIsolation': 'p13',
    'TestCopyFormationContinuityMySQLInterleaving': 'p1',
    'TestRechargeDirectoryIndexMySQLBlockedCommitAndRollback': 'p1',
    'TestRechargeDirectoryMySQLConcurrentClaimsAndReplay': 'p4',
    'TestRechargeDirectoryMySQLP3ObservationBlocksDisable': 'p1',
    'TestPlayerConcurrencyMySQLPVPDoesNotBlockMovement': 'p12_pk',
    'TestPlayerConcurrencyMySQLSceneProjectionMigration': 'p1',
    'TestGGOPeriodPointRealMySQLCancelDoesNotBlockOtherClaim': 'p1',
    'TestPlayerConcurrencyMySQLGGOPoints': 'p1',
    'TestP1GatherRealTwoPlayersIgnoreOtherPeriodXMySQL': 'p4',
    'TestP1GatherFrozenMySQLConcurrentIsolation': 'p1',
    'TestP1GatherFirstFreezeMySQLConcurrentRetry': 'p1',
    'TestP1GatherDisableMySQLWaitsForAdmittedFact': 'p1',
    'TestP1GatherCutoffMySQLWaitsForAdmittedFact': 'p1',
    'TestPlayerConcurrencyMySQLActivityIsolation': 'p1',
    'TestPlayerConcurrencyMySQLSceneMovement': 'p10',
    'TestPlayerConcurrencyMySQLSlowSocket': 'p10',
    'TestSharedFieldBoss55ReadsRound2MySQLStableProjection': 'p1',
    'TestSharedFieldBoss55ReadsRound2MySQLCreateExpireCorrupt': 'p1',
    'TestSharedFieldBoss55ReadsRound2MySQLExpiredRechecksLockedRow': 'p1',
    'TestPlayerConcurrencyMySQLCommerceReaders': 'p4',
    'TestPlayerConcurrencyUnchangedCustomSave': 'p1',
    'TestP5GoldEggOriginalAll54AndOneUse': 'p1',
    'TestP4CopyBenefitsRealDropAtomicReplayConcurrentRestart': 'p4',
}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--case', choices=CASES, action='append')
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--binary', type=Path,
                        default=ROOT / 'build/player-concurrency-20261102/service.test')
    parser.add_argument('--keep-going', action='store_true')
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    binary = args.binary.expanduser().resolve()
    lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
    started = time.monotonic()
    while True:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            break
        except BlockingIOError:
            if time.monotonic() - started > 30:
                raise RuntimeError('shared MySQL regression window is held by another session')
            time.sleep(0.25)
    results = []
    pool = ManagedMySQL()
    try:
        for name in args.case or CASES:
            prefix = CASES[name]
            connection = pool.allocate(prefix + '_player_concurrency')
            began = time.monotonic()
            try:
                environment = {key: value for key, value in os.environ.items()
                               if not (key.startswith('HYJS_') and ('MYSQL' in key or 'DATABASE' in key))}
                environment["TZ"] = "Asia/Shanghai"
                environment.update(connection.env())
                for key in ['HOST', 'USER', 'PASSWORD', 'DATABASE']:
                    environment['HYJS_' + prefix.upper() + '_MYSQL_' + key] = environment['HYJS_MYSQL_' + key]
                result = subprocess.run(
                    ['rtk', 'proxy', str(binary), '-test.run=^' + name + '$', '-test.v', '-test.timeout=5m'],
                    cwd=ROOT / 'hyjs-server/service', env=environment, text=True,
                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=330)
                output = result.stdout.replace(connection.password, '[REDACTED]')
                (args.output / (name.replace('/', '__') + '.log')).write_text(output)
                passed = result.returncode == 0 and ('--- PASS: ' + name) in output and ('--- SKIP: ' + name) not in output
                results.append({'case': name, 'passed': passed, 'exitCode': result.returncode,
                                'seconds': round(time.monotonic() - began, 3)})
                print(json.dumps(results[-1]), flush=True)
                if not passed and not args.keep_going:
                    break
            finally:
                pool.release(connection.database)
    finally:
        pool.close()
        fcntl.flock(lock, fcntl.LOCK_UN)
        lock.close()
    (args.output / 'summary.json').write_text(json.dumps(results, indent=2) + '\n')
    return 0 if results and all(item['passed'] for item in results) else 1

if __name__ == '__main__':
    raise SystemExit(main())

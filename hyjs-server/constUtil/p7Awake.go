package constUtil

const (
	P7AwakePolicyID           int64 = 1
	P7AwakeClosed                   = "觉醒之地原难度任务/地图节点与权威战斗尚未恢复，挑战保持关闭"
	P7AwakeSource                   = "原5章节/星期开放/每日2次/30分钟/3次复活/体力0；难度任务、节点位置、探索分数与等级门槛由原服下发未恢复。70级仅禁用兼容门槛，不能据此开放挑战。"
	P7AwakeExplorePolicyID    int64 = 2
	P7AwakeExploreSource            = "local-awake-forest-explore-v1:chapter5/copy501;原场景NavMesh上的4个本地箱点box1-4及mon030本地怪点；每箱5分/怪80分；箱奖按原dropAward简单权重语法逐项独立抽取并入场冻结，资源仅退出结算一次发放；原服点位/数量/评分/投放概率未恢复；type16战斗独立证明前关闭"
	P7AwakeExploreCopy              = "501"
	P7AwakeExploreActive            = "active"
	P7AwakeExploreSettled           = "settled"
	P7AwakeExploreAbandoned         = "abandoned"
	P7AwakeBattlePolicyID     int64 = 3
	P7AwakeBattleSource             = "local-awake16-forest-v1:原typed16初始化自动001003/talent0/满HP-AP0/pose3-7/factor1/max40/map_forest_003；本地mon030一场原Copy10完整两monster033，真End完整ops/四终末HP独立核对后胜利80分；原怪rewardGroupId简单权重入场冻结；真100分按入场日限1次hero_exp_002和原强化所需5000coin评分奖供原P5强化，角色觉醒材料/同卡继续原自然来源，不补库存资格"
	P7AwakeBattleReplaySource       = "p7-awake16"
	P7AwakeFightPending             = "pending"
	P7AwakeFightAccepted            = "accepted"
	P7AwakeFightAbandoned           = "abandoned"
	P7AwakeCompleteEvent            = "awake-local-complete"
	P7AwakeCompleteDailyEvent       = "awake-local-daily-score"
	P7AwakeScoreReward              = "hero_exp_002,1;coin,5000"
	P7AwakeEnemyHex                 = "08e80710011a02313020002aac010a0b6d6f6e737465725f303333120a6d6f6e737465722d32301a0874616c5f313035362208313033335f312c2c2a630900000000004056401100000000008066401900000000000020402100000000000020404900000000000000005100000000000059405900000000000000006100000000000014c0690000000000000000710000000000000000790000000000c062403003380d420909000000000080664059000000000000594060012aac010a0b6d6f6e737465725f303333120a6d6f6e737465722d32311a0874616c5f313035362208313033335f312c2c2a630900000000004056401100000000004060401900000000000020402100000000000020404900000000000000005100000000000059405900000000000000006100000000000014c0690000000000000000710000000000000000790000000000c0624030033803420909000000000040604059000000000000594060013209090000000000000000"
	P7AwakeEnemySHA                 = "7c50901194aff29186c110f08e9d1069daec2ac93d6dce64a5bfd48e5c33c49e"
	P7AwakeExplorePreview           = "item_320;item_317;item_316;item_314;item_312"
	P7AwakeBattlePreview            = ";coin;hero_exp_002;item_323"
)

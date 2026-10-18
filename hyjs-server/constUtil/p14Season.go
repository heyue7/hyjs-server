package constUtil

const P14AuditionBeforeStage = int32(3)
const P14AuditionStage = int32(4)
const P14AuditionSource = "原70级/真实本服报名截止名册及阵容；本地严格把已发布原赛季日历与联合身份名册封存，后续改发布不追溯改变当季实例；阶段3/4海选资格不代替后续晋级/排名/胜负/奖励"

const P14AuditionBattleSource = "p14-audition"
const P14AuditionMatchSource = "原本服随机对战/5败淘汰/最后10人；本地每轮随机无放回配对，奇数留空且不记轮空胜；只有完整P6权威记录能推进下一轮"
const P14AuditionAuthorityPending = "海选整场战斗尚未取得原客户端权威对照，不能产生淘汰或晋级"
const P14AuditionLossLimit = int64(5)
const P14AuditionPromotionLimit = 10
const P14AuditionAutoSource = "原TableText.serverbattle_auto_win及UIServerBattleAudition.Recevie空本服战报autoPass分支；结合原最后10名晋级容量，本地报名1..10自动晋级，0人不创建资格；不生成对局或胜场"

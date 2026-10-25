package contentfilter

import (
	"strings"
	"testing"
)

func TestContainsBlockedVariants(t *testing.T) {
	for _, text := range []string{
		"成人视频", "色 情", "色\u200b情", "色\ufe0f情", "色🔥情", "色－情", "色情网站",
		"習近平", "习 近 平", "共產黨", "台獨", "法輪功", "法.轮#功",
		"ＰＯＲＮＨＵＢ", "P_o-R n Hub", "x-videos", "习近平",
		"屄", "屌", "肏", "ＡＶ女优", "porn123", "xxpornxx", "xjp001", "sm", "s.m", "anal",
	} {
		t.Run(text, func(t *testing.T) {
			if !Contains(text) {
				t.Fatal("blocked term escaped")
			}
		})
	}
}

func TestExclusionsDoNotAllowOtherBlockedTerms(t *testing.T) {
	if !Contains("集合看色情") {
		t.Fatal("excluded term exempted the whole message")
	}
}

func TestContainsNormalGameText(t *testing.T) {
	for _, text := range []string{"", "胡桃", "江湖剑士", "归来的剑士123", "今天一起打副本", "工会晚上八点集合", "公会战守护工厂", "勇者Dragon2026", "hello world", "cave", "Sussex", "smallcat", "analysis", "装备强化和宝石合成", "料理今天大成功", "刺激的副本", "限量礼包", "铃声和弦", "一大波怪物来了", "激情的公会战", "不穿装备挑战", "被干掉了", "把宝石插进去", "干死这个BOSS"} {
		t.Run(text, func(t *testing.T) {
			if Contains(text) {
				t.Fatal("ordinary text blocked")
			}
		})
	}
}

func BenchmarkContains(b *testing.B) {
	text := strings.Repeat("今天一起打副本", 5)
	for b.Loop() {
		Contains(text)
	}
}

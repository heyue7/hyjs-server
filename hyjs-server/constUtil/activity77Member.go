package constUtil

const Activity77MemberAudit = "activity77.member-daily-curve"

// Activity77MemberDailyCoin is the fixed daily coin amount for each VIP tier.
func Activity77MemberDailyCoin(vip int64) (int64, bool) {
	amounts := [...]int64{
		100000, 200000, 300000, 400000, 550000, 700000, 900000,
		1100000, 1300000, 1600000, 2000000, 2500000, 3000000,
	}
	if vip < 0 || vip >= int64(len(amounts)) {
		return 0, false
	}
	return amounts[vip], true
}

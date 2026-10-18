package service

// p15GreenFragmentZeroSale 仅放行已批准的六绿兼容碎片，不将缺价格当作免费配置。
func p15GreenFragmentZeroSale(row map[string]string) bool {
	if row["price"] != "0" || row["num"] != "10" || row["id"] != "frag_"+row["target"] {
		return false
	}
	switch row["target"] {
	case "hero_056", "hero_057", "hero_058", "hero_059", "hero_060", "hero_061":
		return true
	}
	return false
}

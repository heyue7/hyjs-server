package constUtil

// WarriorTraining85 rules are local server defaults; APK costs and caps remain authoritative.
const (
	WarriorTraining85Distribution1 = "[[60,1]]"
	WarriorTraining85Distribution2 = "[[90,1]]"
	WarriorTraining85Distribution3 = "[[140,1]]"
	WarriorTraining85Source1       = "本地服训练gradient-v3：按当前属性接近上限比例决定正负，幅度60基点；APK费用/上限/10级开放保留"
	WarriorTraining85Source2       = "本地服训练gradient-v3：按当前属性接近上限比例决定正负，幅度90基点；APK费用/上限/10级开放保留"
	WarriorTraining85Source3       = "本地服训练gradient-v3：按当前属性接近上限比例决定正负，幅度140基点；APK费用/上限/10级开放保留"
	WarriorTraining85GradientScale = int64(10000)
	WarriorTraining85GradientPeak  = int64(9500)
	WarriorTraining85GradientDrop  = uint64(7000)
	WarriorTraining85NegativeScale = int64(50000)
	WarriorTraining85JitterLow     = int64(80)
	WarriorTraining85JitterHigh    = int64(120)

	WarriorTraining85V1Distribution1 = "[[-20,20],[10,40],[20,40]]"
	WarriorTraining85V1Distribution2 = "[[-10,10],[20,40],[30,50]]"
	WarriorTraining85V1Distribution3 = "[[20,40],[40,60]]"
	WarriorTraining85V1GoSource1     = "本地服训练：上限基点-20/10/20，权重20/40/40；APK费用/上限/10级开放保留"
	WarriorTraining85V1GoSource2     = "本地服训练：上限基点-10/20/30，权重10/40/50；APK费用/上限/10级开放保留"
	WarriorTraining85V1GoSource3     = "本地服训练：上限基点20/40，权重40/60；APK费用/上限/10级开放保留"
	WarriorTraining85V1SQLSource1    = "本地服训练：每属性独立按上限的-0.2%/0.1%/0.2%抽取，权重20/40/40；APK费用/上限/10级开放保留。"
	WarriorTraining85V1SQLSource2    = "本地服训练：每属性独立按上限的-0.1%/0.2%/0.3%抽取，权重10/40/50；APK费用/上限/10级开放保留。"
	WarriorTraining85V1SQLSource3    = "本地服训练：每属性独立按上限的0.2%/0.4%抽取，权重40/60；APK费用/上限/10级开放保留。"
)

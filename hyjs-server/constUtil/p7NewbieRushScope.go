package constUtil

// Empty proof keeps this independent Trial family closed. Local CoreCLR/Go
// parity cannot populate it; a reviewed original Android anchor is required.
const (
	P7NewbieRushNativeProof     = ""
	P7NewbieRushDriverSource    = "local-p7-trial-manual-live-auto-newbie047025-v1; exact level32-main level1-companion and strict full End"
	P7NewbieRushMainSHA256      = "50f1949d62fe639828b62d140c64342d53fca3a72e17e4c5676fb9d75b84cf2f"
	P7NewbieRushCompanionSHA256 = "09f4c067a470fb65e57954a493f7d9f5d0ddd31c429c1bc03ee9f65776f1ec3f"
	P7NewbieRushNPCSHA256       = "97ec2ffad1cf9bc3946ecde31274df10b95d030dbfb40d98c9859951189c12f2"
	P7NewbieRushFloor4NPCSHA256 = "acd4e09f177f1a12b5f293151efda51444cdb1f989d4016a2355f14c790247ea"
	// Original Tick accumulation observed for the prepublished delay30.1 input.
	// This is the exact End value, with no clock rounding or tolerance.
	P7NewbieRushDelayedAutoRoundTime = 30.100000000000158
)

package constUtil

import "time"

const (
	P10VoiceSweepBatchSize       = 128
	P10VoiceSweepInterval        = time.Minute
	P10VoiceUploaded       int64 = 1
	P10VoicePublished      int64 = 2
	P10VoiceDeleted        int64 = 3
	P10VoicePrefix               = "hyjsvoice_"
	P10VoiceMaxBytes       int64 = 960044
	P10VoiceSource               = "local-original-chat-audio2-wav-v1: original isAudio2/max30sec/recordnap1sec; genuine PCM16 mono16k WAV in InnoDB, randomopaque filename onlyafter verifiedstoredbytes, peruploadnonce idempotent; originalGCloud key/upload/ASR/download notrestored andnotemulated; caption optional withoutfakeASR; originallogin audiosystem mirrorsactualpolicy and originalaudioautoget localdefaultoff; local genericChatInterval20 sharedtext/voice throttle; explicitprepublished retention/minclip/pending3/live50caps/visiblehistory250candidatewindow; everyplay/download rechecks source/globalgate/currentaccountsession/scope/currentguild/fieldroster/blacklist, no cachebypass; owncurrentSession delete clearsblob/tombstone; TTLpurge; CSAudioChatCheck localreturns onlyactuallatestcurrentSession/channeluploadedfile, no empty success or unmaterializedreservation; defaultdisabled"
	P10VoiceClosed               = "本地原语音适配尚未开放"
)

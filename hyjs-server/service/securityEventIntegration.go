package service

import (
	"context"
	"go.uber.org/zap"
	"hyjs-server/constUtil"
	"hyjs-server/elog"
	"hyjs-server/model"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var securityEventLastWarning atomic.Int64
var securityEventSuppressed atomic.Int64
var securityFailureLogBudget = struct {
	sync.Mutex
	Bucket loginBucket
}{}

// ObserveSecurityEvent is called only after business transactions finish. An
// audit outage never turns a committed login or mutation into a false failure.
func ObserveSecurityEvent(ctx context.Context, event model.SecurityEventInput) {
	if net.ParseIP(event.SourceIP) == nil {
		return
	}
	// Rejected-request floods must not cause one SQL log write per packet.
	// Successful sensitive operations retain their individual audit records.
	if event.Outcome != "success" {
		now := time.Now()
		securityFailureLogBudget.Lock()
		bucket := securityFailureLogBudget.Bucket
		if bucket.Last.IsZero() {
			bucket.Tokens = constUtil.SecurityFailureLogBurst
		} else {
			bucket.Tokens = min(constUtil.SecurityFailureLogBurst, bucket.Tokens+max(0, now.Sub(bucket.Last).Seconds())*constUtil.SecurityFailureLogRate)
		}
		bucket.Last = now
		allowed := bucket.Tokens >= 1
		if allowed {
			bucket.Tokens--
		}
		securityFailureLogBudget.Bucket = bucket
		securityFailureLogBudget.Unlock()
		if !allowed {
			count := securityEventSuppressed.Add(1)
			prior := securityEventLastWarning.Load()
			if now.Unix()-prior >= 60 && securityEventLastWarning.CompareAndSwap(prior, now.Unix()) {
				elog.Warn("high-frequency security events sampled", zap.Int64("suppressed", count))
			}
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if err := RecordSecurityEvent(ctx, event); err != nil {
		now := time.Now().Unix()
		prior := securityEventLastWarning.Load()
		if now-prior >= 60 && securityEventLastWarning.CompareAndSwap(prior, now) {
			elog.Warn("security event storage unavailable", zap.String("action", event.Action))
		}
	}
}

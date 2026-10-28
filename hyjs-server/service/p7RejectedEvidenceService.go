package service

import (
	"hyjs-server/config"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var p7RejectedEvidenceMu sync.Mutex

const p7RejectedEvidenceTTL = 20 * time.Minute
const p7RejectedEvidenceLimit = 4
const p7RejectedEvidenceMaxBytes = 64 * 1024

// Only the rejected typed End is retained, never an HTTP envelope, account,
// session or key. Server-private, bounded and automatically removed after 20m.
func p7RejectedEvidenceDirectory() string {
	if config.AppConfigIns == nil || config.AppConfigIns.LogConfig.LogFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(config.AppConfigIns.LogConfig.LogFile), "trial-rejected")
}

var p7RejectedEvidenceTimers = map[string]*time.Timer{}

func p7ResetRejectedEndEvidence() {
	dir := p7RejectedEvidenceDirectory()
	if dir == "" {
		return
	}
	p7RejectedEvidenceMu.Lock()
	defer p7RejectedEvidenceMu.Unlock()
	for path, timer := range p7RejectedEvidenceTimers {
		timer.Stop()
		delete(p7RejectedEvidenceTimers, path)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	entries, _ := os.ReadDir(dir)
	for _, item := range entries {
		if len(item.Name()) == 67 && strings.HasSuffix(item.Name(), ".pb") {
			_ = os.Remove(filepath.Join(dir, item.Name()))
		}
	}
}
func p7CaptureRejectedEnd(raw []byte) bool {
	dir := p7RejectedEvidenceDirectory()
	if dir == "" {
		return false
	}
	path, ok := p7WriteRejectedEvidence(dir, raw, time.Now())
	if !ok {
		return false
	}
	p7RejectedEvidenceMu.Lock()
	defer p7RejectedEvidenceMu.Unlock()
	for name, timer := range p7RejectedEvidenceTimers {
		if _, err := os.Lstat(name); os.IsNotExist(err) {
			timer.Stop()
			delete(p7RejectedEvidenceTimers, name)
		}
	}
	if path != "" {
		p7RejectedEvidenceTimers[path] = time.AfterFunc(p7RejectedEvidenceTTL, func() {
			p7RejectedEvidenceMu.Lock()
			defer p7RejectedEvidenceMu.Unlock()
			_ = os.Remove(path)
			delete(p7RejectedEvidenceTimers, path)
		})
	}
	return true
}
func p7WriteRejectedEvidence(dir string, raw []byte, now time.Time) (string, bool) {
	if len(raw) == 0 || len(raw) > p7RejectedEvidenceMaxBytes {
		return "", false
	}
	p7RejectedEvidenceMu.Lock()
	defer p7RejectedEvidenceMu.Unlock()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", false
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	type entry struct {
		path     string
		modified time.Time
	}
	var kept []entry
	for _, item := range entries {
		if !strings.HasSuffix(item.Name(), ".pb") || len(item.Name()) != 67 {
			continue
		}
		info, err := item.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(dir, item.Name())
		if now.Sub(info.ModTime()) >= p7RejectedEvidenceTTL {
			_ = os.Remove(path)
			continue
		}
		kept = append(kept, entry{path, info.ModTime()})
	}
	path := filepath.Join(dir, p6Digest(raw)+".pb")
	if info, err := os.Lstat(path); err == nil {
		return "", info.Mode().IsRegular()
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].modified.Before(kept[j].modified) })
	for len(kept) >= p7RejectedEvidenceLimit {
		if os.Remove(kept[0].path) != nil {
			return "", false
		}
		kept = kept[1:]
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", false
	}
	_, err = file.Write(raw)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(path)
		return "", false
	}
	return path, true
}

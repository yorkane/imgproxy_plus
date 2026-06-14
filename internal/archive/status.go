package archive

import (
	"encoding/json"
	"sync"
	"time"
)

type Status struct {
	Running      bool      `json:"running"`
	CurrentDir   string    `json:"current_dir,omitempty"`
	CurrentGroup string    `json:"current_group,omitempty"`
	Converted    int       `json:"converted"`
	TotalImages  int       `json:"total_images"`
	Errors       int       `json:"errors"`
	StartTime    time.Time `json:"start_time,omitempty"`
}

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Dir     string    `json:"dir,omitempty"`
	Group   string    `json:"group,omitempty"`
	Msg     string    `json:"msg"`
	Converted int     `json:"converted,omitempty"`
	CBZ      int      `json:"cbz,omitempty"`
	Duration string   `json:"duration,omitempty"`
}

var (
	statusMu sync.RWMutex
	currentStatus  Status
	recentLogs     []LogEntry
	maxLogs        = 200
)

func UpdateStatus(running bool, dir string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	currentStatus.Running = running
	if running {
		currentStatus.CurrentDir = dir
		currentStatus.StartTime = time.Now()
		currentStatus.Converted = 0
		currentStatus.TotalImages = 0
		currentStatus.Errors = 0
		currentStatus.CurrentGroup = ""
	} else {
		currentStatus.Running = false
	}
}

func SetGroupProgress(group string, converted, total int) {
	statusMu.Lock()
	defer statusMu.Unlock()
	currentStatus.CurrentGroup = group
	currentStatus.Converted = converted
	currentStatus.TotalImages = total
}

func AddError() {
	statusMu.Lock()
	defer statusMu.Unlock()
	currentStatus.Errors++
}

func GetStatus() Status {
	statusMu.RLock()
	defer statusMu.RUnlock()
	return currentStatus
}

func AppendLog(entry LogEntry) {
	statusMu.Lock()
	defer statusMu.Unlock()
	recentLogs = append(recentLogs, entry)
	if len(recentLogs) > maxLogs {
		recentLogs = recentLogs[len(recentLogs)-maxLogs:]
	}
}

func GetLogs() []LogEntry {
	statusMu.RLock()
	defer statusMu.RUnlock()
	result := make([]LogEntry, len(recentLogs))
	copy(result, recentLogs)
	return result
}

func LogEvent(level, msg, dir, group string, extra map[string]interface{}) {
	entry := LogEntry{
		Time:  time.Now(),
		Level: level,
		Dir:   dir,
		Group: group,
		Msg:   msg,
	}
	if v, ok := extra["converted"]; ok {
		entry.Converted, _ = v.(int)
	}
	if v, ok := extra["cbz"]; ok {
		entry.CBZ, _ = v.(int)
	}
	if v, ok := extra["duration"]; ok {
		if d, ok2 := v.(time.Duration); ok2 {
			entry.Duration = d.Round(time.Millisecond).String()
		}
	}
	AppendLog(entry)
}

func GetStatusJSON() ([]byte, error) {
	s := GetStatus()
	logs := GetLogs()
	return json.Marshal(struct {
		Status Status     `json:"status"`
		Logs   []LogEntry `json:"logs"`
	}{s, logs})
}

package logger

import (
	"fmt"
	"strings"
	"time"
)

var colorsSupported = false

// palette
const (
	colorReset   = "\033[0m"
	colorGreen   = "\033[38;2;166;227;161m" // #a6e3a1
	colorTeal    = "\033[38;2;148;226;213m" // #94e2d5
	colorYellow  = "\033[38;2;249;226;175m" // #f9e2af
	colorPeach   = "\033[38;2;250;179;135m" // #fab387
	colorRed     = "\033[38;2;243;139;168m" // #f38ba8
	colorSubtext = "\033[38;2;166;173;200m" // #a6adc8
	colorWhite   = "\033[38;2;205;214;244m" // #cdd6f4
	colorKey     = "\033[38;2;137;220;235m" // #89dceb
	colorVal     = "\033[38;2;166;227;161m" // #a6e3a1
)

type Logger struct {
	scope string
}

// make_unique logger factory
func Make(scope string) *Logger {
	return &Logger{scope: scope}
}

// parameters key=val without extra air
func formatFields(fields ...any) string {
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(fields); i += 2 {
		key := fmt.Sprintf("%v", fields[i])
		val := "nil"
		if i+1 < len(fields) {
			val = fmt.Sprintf("%v", fields[i+1])
		}
		if colorsSupported {
			b.WriteString(fmt.Sprintf(" %s%s%s=%s%s%s", colorKey, key, colorReset, colorVal, val, colorReset))
		} else {
			b.WriteString(fmt.Sprintf(" %s=%s", key, val))
		}
	}
	return b.String()
}

func (l *Logger) emit(level, levelColor, action string, fields ...any) {
	now := time.Now().Format("15:04:05")
	params := formatFields(fields...)

	if colorsSupported {
		fmt.Printf("%s%s%s %s[%s]%s %s%s%s %s%s%s%s\n",
			colorSubtext, now, colorReset,
			colorGreen, l.scope, colorReset,
			levelColor, level, colorReset,
			colorWhite, action, colorReset,
			params,
		)
	} else {
		fmt.Printf("%s [%s] %s: %s%s\n", now, l.scope, level, action, params)
	}
}

func (l *Logger) Info(action string, fields ...any)  { l.emit("info", colorTeal, action, fields...) }
func (l *Logger) Ok(action string, fields ...any)    { l.emit("ok", colorGreen, action, fields...) }
func (l *Logger) Warn(action string, fields ...any)  { l.emit("warn", colorPeach, action, fields...) }
func (l *Logger) Error(action string, fields ...any) { l.emit("err", colorRed, action, fields...) }

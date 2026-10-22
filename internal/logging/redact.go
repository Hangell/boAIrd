package logging

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

var (
	bearerPattern     = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
	keyPattern        = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{8,}|AIza[A-Za-z0-9_-]{20,})\b`)
	assignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password|authorization)(\s*[:=]\s*)[^\s,;]+`)
)

// NewRedactingHandler prevents secret-bearing attributes and common token forms from reaching logs.
func NewRedactingHandler(next slog.Handler) slog.Handler {
	return redactingHandler{next: next}
}

type redactingHandler struct{ next slog.Handler }

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, redactText(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		clean[i] = redactAttr(attr)
	}
	return redactingHandler{next: h.next.WithAttrs(clean)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if isSensitiveName(attr.Key) {
		attr.Value = slog.StringValue("[REDACTED]")
		return attr
	}
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		children := value.Group()
		for i := range children {
			children[i] = redactAttr(children[i])
		}
		attr.Value = slog.GroupValue(children...)
		return attr
	}
	if value.Kind() == slog.KindString {
		attr.Value = slog.StringValue(redactText(value.String()))
	} else if value.Kind() == slog.KindAny {
		attr.Value = slog.StringValue("[REDACTED]")
	}
	return attr
}

func isSensitiveName(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "-", "_"), " ", "_"))
	for _, marker := range []string{"secret", "token", "api_key", "apikey", "authorization", "password", "cookie", "private_key", "client_key", "credential", "header"} {
		if strings.Contains(name, marker) {
			if marker == "credential" && strings.HasSuffix(name, "_id") {
				return false
			}
			return true
		}
	}
	return false
}

func redactText(value string) string {
	value = bearerPattern.ReplaceAllString(value, "$1[REDACTED]")
	value = keyPattern.ReplaceAllString(value, "[REDACTED]")
	return assignmentPattern.ReplaceAllString(value, "$1$2[REDACTED]")
}

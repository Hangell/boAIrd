package api

import (
	"errors"
	"fmt"
	"net/http"

	"boAIrd/internal/notification"
)

type notificationPage struct {
	Items  []notification.Notice `json:"items"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

func registerNotificationRoutes(mux *http.ServeMux, manager NotificationManager) {
	mux.HandleFunc("GET /api/v1/projects/{id}/notifications", func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInteger(r, "limit", 50)
		if err != nil || limit < 1 || limit > 100 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
		offset, err := queryInteger(r, "offset", 0)
		if err != nil || offset < 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "offset must be non-negative")
			return
		}
		values, err := manager.List(r.Context(), r.PathValue("id"), limit, offset)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, notificationPage{Items: values, Limit: limit, Offset: offset})
	})
	mux.HandleFunc("GET /api/v1/projects/{id}/notifications/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, "stream_unavailable", "notification streaming is unavailable")
			return
		}
		stream, unsubscribe := manager.Subscribe(r.PathValue("id"))
		defer unsubscribe()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case value, open := <-stream:
				if !open {
					return
				}
				if _, err := fmt.Fprintf(w, "data: {\"id\":%q,\"event_type\":%q}\n\n", value.ID, value.EventType); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("POST /api/v1/projects/{id}/notifications/{notification_id}/read", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.MarkRead(r.Context(), r.PathValue("id"), r.PathValue("notification_id")); err != nil {
			writeNotificationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/projects/{id}/notifications/{notification_id}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Acknowledge(r.Context(), r.PathValue("id"), r.PathValue("notification_id")); err != nil {
			writeNotificationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/projects/{id}/notifications/{notification_id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Retry(r.Context(), r.PathValue("id"), r.PathValue("notification_id")); err != nil {
			writeNotificationError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func writeNotificationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notification.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "notification not found")
	case errors.Is(err, notification.ErrRetryLimit):
		writeAPIError(w, http.StatusConflict, "retry_unavailable", "notification cannot be retried")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"go.uber.org/zap"
)

var (
	ErrInternal = errors.New("internal server error")
	ErrBadInput = errors.New("invalid input")
)

type ctxKey int

const (
	requestID ctxKey = iota
)

type wideResponseWriter struct {
	http.ResponseWriter
	length, status int
	internalErr    error
}

func (w *wideResponseWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	w.status = status
}

func (w *wideResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.length += n
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return n, err
}

// REST API.
type API struct {
	router *mux.Router
	logger *zap.Logger
}

// New возвращает [*API].
func NewApi(logger *zap.Logger) *API {
	api := API{
		router: mux.NewRouter(),
		logger: logger,
	}
	api.endpoints()
	return &api
}

// ServeHTTP - таким образом, мы можем использовать
// сам [*API] в качестве мультиплексора на сервере.
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.router.ServeHTTP(w, r)
}

func (api *API) endpoints() {
	api.router.Use(
		api.requestIDMiddleware,
		api.wideEventLogMiddleware,
		api.closerMiddleware,
		api.headersMiddleware,
	)
	api.router.HandleFunc("/comments", api.handleCommentCheck()).Methods(http.MethodPost, http.MethodOptions)
}

// closerMiddleware считывает и закрывает тело запроса
// для повторного использования TCP-соединения.
func (api *API) closerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	})
}

// requestIDMiddleware извлекает id запроса из параметров запроса.
// В случае если id запроса отсутствует, id генерируется.
// Далее id добавляется в контекст запроса.
func (api *API) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.URL.Query().Get("request-id")
		ctxWithID := context.WithValue(r.Context(), requestID, rid)
		rWithID := r.WithContext(ctxWithID)
		next.ServeHTTP(w, rWithID)
	})
}

// wideEventLogMiddleware собирает и регистрирует информацию о полученном запросе.
func (api *API) wideEventLogMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			wideWriter := &wideResponseWriter{ResponseWriter: w}

			next.ServeHTTP(wideWriter, r)

			addr, _, _ := net.SplitHostPort(r.RemoteAddr)
			api.logger.Info("request received",
				zap.Any("request_id", r.Context().Value(requestID)),
				zap.Int("status_code", wideWriter.status),
				zap.Int("response_length", wideWriter.length),
				zap.Int64("content_length", r.ContentLength),
				zap.String("method", r.Method),
				zap.String("proto", r.Proto),
				zap.String("remote_addr", addr),
				zap.String("uri", r.RequestURI),
				zap.String("user_agent", r.UserAgent()),
				zap.Error(wideWriter.internalErr),
			)
		},
	)
}

// headersMiddleware задает обычные заголовки для всех ответов.
func (api *API) headersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func (api *API) WriteJSONError(w http.ResponseWriter, err error, code int) {
	w.WriteHeader(code)
	if wrw, ok := w.(*wideResponseWriter); ok {
		wrw.internalErr = err
	}
	if code == http.StatusInternalServerError {
		err = ErrInternal
	}
	msg := map[string]string{"error": err.Error()}
	_ = json.NewEncoder(w).Encode(&msg)
}

func (api *API) WriteJSON(w http.ResponseWriter, data any, code int) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// handleCommentCheck проверяет входящий комментарий на
// содержание запрещенных слов.
func (api *API) handleCommentCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		var c Comment
		err := json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			api.WriteJSONError(w, err, http.StatusBadRequest)
			return
		}

		if Banned(c) {
			api.WriteJSON(w, map[string]string{"response": "banned"}, http.StatusBadRequest)
		} else {
			api.WriteJSON(w, map[string]string{"response": "allowed"}, http.StatusOK)
		}

	}
}

// пакет api предоставляет маршрутизатор REST API
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rtemka/agg/comments/domain"
	"github.com/rtemka/agg/comments/pkg/sqlite"

	"go.uber.org/zap"
)

var (
	ErrInternal = errors.New("internal server error")
	ErrBadInput = errors.New("invalid input")
	ErrNoNewsID = errors.New("invalid input: 'news_id' not found in query parameters")
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
	repo   domain.Repository
	logger *zap.Logger
}

// New возвращает [*API].
func New(db domain.Repository, logger *zap.Logger) *API {
	api := API{
		router: mux.NewRouter(),
		logger: logger,
		repo:   db,
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
	api.router.HandleFunc("/comments", api.handleCommentCreate()).Methods(http.MethodPost, http.MethodOptions)
	api.router.HandleFunc("/comments", api.handleCommentRead()).Methods(http.MethodGet, http.MethodOptions)
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

func (api *API) handleCommentCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		var c domain.Comment
		err := json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			api.WriteJSONError(w, err, http.StatusBadRequest)
			return
		}
		if c.NewsID == 0 {
			api.WriteJSONError(w, ErrNoNewsID, http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		id, err := api.repo.Create(ctx, &c)
		if err != nil {
			api.WriteJSONError(w, err, http.StatusInternalServerError)
			return
		}

		api.WriteJSON(w, map[string]any{"id": id}, http.StatusCreated)

	}
}

func (api *API) handleCommentRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		s := r.URL.Query().Get("news-id")
		if s == "" {
			api.WriteJSONError(w, ErrNoNewsID, http.StatusBadRequest)
			return
		}

		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			api.WriteJSONError(w, fmt.Errorf("%w: parsing 'news-id' %v", ErrBadInput, err), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		coms, err := api.repo.Read(ctx, id)
		if err != nil && err != sqlite.ErrNoRows {
			api.WriteJSONError(w, ErrInternal, http.StatusInternalServerError)
			return
		}
		api.WriteJSON(w, coms, http.StatusOK)
	}
}

// пакет api предоставляет маршрутизатор REST API
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rtemka/agg/gateway/domain"

	"go.uber.org/zap"
)

var (
	ErrInternal = errors.New("internal server error")
	ErrBadInput = errors.New("invalid input")
)

const (
	NewsServiceName       = "news"
	CommentsServiceName   = "comments"
	CommsCheckServiceName = "commscheck"
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
	// Services - ассоциативный массив, где ключ это имя сервиса
	// а значение это его адрес в сети.
	// После создания объекта API предполагается, что пользователь
	// установит сетевые адреса сервисов.
	Services map[string]string
}

// New возвращает [*API].
func New(logger *zap.Logger) *API {
	api := API{
		router:   mux.NewRouter(),
		logger:   logger,
		Services: map[string]string{NewsServiceName: "", CommentsServiceName: ""},
	}
	api.endpoints()
	rand.Seed(time.Now().UnixNano())
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
		api.secHeadersMiddleware,
	)
	api.router.HandleFunc("/news/latest", api.handleNewsLatest()).Methods(http.MethodGet, http.MethodOptions)
	api.router.HandleFunc("/news", api.handleNewsLatest()).Methods(http.MethodGet, http.MethodOptions)
	api.router.HandleFunc("/news/{id}", api.handleNewsDitailed()).Methods(http.MethodGet, http.MethodOptions)
	api.router.HandleFunc("/comments", api.handleCommentCreate()).Methods(http.MethodPost, http.MethodOptions)
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
		if rid == "" {
			rid = randStr(18)
		}
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

// secHeadersMiddleware устанавливает строгие заголовки безопасности для всех ответов.
func (api *API) secHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
		w.Header().Set("Server", "") // удаляет информацию о том, какой сервер используется
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

func (api *API) handleNewsLatest() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		u := api.serviceURL(r, NewsServiceName, NewsServiceName)

		api.forwardReq(&u, http.MethodGet, nil, w, r)
	}
}

func (api *API) handleCommentCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := api.serviceURL(r, CommsCheckServiceName, CommentsServiceName)

		var b bytes.Buffer
		tee := io.TeeReader(r.Body, &b)

		resp, err := makeRequest(&u, http.MethodPost, tee)
		if err != nil {
			api.WriteJSONError(w, err, http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
			return
		}

		u = api.serviceURL(r, CommentsServiceName, CommentsServiceName)
		api.forwardReq(&u, http.MethodPost, bytes.NewReader(b.Bytes()), w, r)
	}
}

func (api *API) handleNewsDitailed() http.HandlerFunc {

	nf := requestFunc(jsonDecFunc[domain.NewsFullDetailed])
	cf := requestFunc(jsonDecFunc[[]domain.Comment])

	return func(w http.ResponseWriter, r *http.Request) {
		nsu := api.serviceURL(r, NewsServiceName, NewsServiceName+strings.TrimPrefix(r.URL.Path, "/news"))
		csu := api.serviceURL(r, CommentsServiceName, CommentsServiceName)
		urls := map[url.URL]requester{nsu: nf, csu: cf}

		ch := make(chan any, len(urls))

		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for k, v := range urls {
			go func(u url.URL, r requester) {
				v, err := r(c, &u)
				if err == nil {
					ch <- v
				} else {
					ch <- err
				}
			}(k, v)
		}

		var news domain.NewsFullDetailed
		var comments []domain.Comment

		for i := 0; i < len(urls); i++ {
			v := <-ch
			switch r := v.(type) {
			case error:
				api.WriteJSONError(w, r, http.StatusInternalServerError)
				return
			case domain.NewsFullDetailed:
				news = r
			case []domain.Comment:
				comments = r
			default:
				api.WriteJSONError(w, errors.New("unknown return value from service"), http.StatusInternalServerError)
				return
			}
		}

		news.Comments = domain.ToTree(comments)

		api.WriteJSON(w, news, http.StatusOK)
	}
}

func (api *API) forwardReq(u *url.URL, method string, body io.Reader, w http.ResponseWriter, r *http.Request) {

	resp, err := makeRequest(u, method, body)
	if err != nil {
		api.WriteJSONError(w, err, http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func makeRequest(u *url.URL, method string, body io.Reader) (*http.Response, error) {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(c, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	return http.DefaultClient.Do(req)
}

type requester func(context.Context, *url.URL) (any, error)

func requestFunc[T any](f func(r io.ReadCloser) (T, error)) requester {

	return func(ctx context.Context, u *url.URL) (any, error) {
		resp, err := makeRequest(u, http.MethodGet, nil)
		if err != nil {
			return nil, err
		}

		return f(resp.Body)
	}
}

func jsonDecFunc[T any](r io.ReadCloser) (T, error) {
	defer func() {
		_ = r.Close()
	}()
	var t T
	return t, json.NewDecoder(r).Decode(&t)
}

func (api *API) serviceURL(r *http.Request, name, path string) url.URL {
	u := url.URL{
		Scheme: "http",
		Host:   api.Services[name],
		Path:   path,
	}
	q := r.URL.Query()
	q.Set("request-id", r.Context().Value(requestID).(string))
	if id, ok := mux.Vars(r)["id"]; ok {
		q.Set("news-id", id)
	}
	u.RawQuery = q.Encode()

	return u
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
var nums = []rune("1234567890")

// randStr генерирует простyю случайную строку вплоть до n
// символов, чередуя числа и буквы английского алфавита.
func randStr(n int) string {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		if i^1 == i+1 {
			b.WriteRune(nums[rand.Intn(len(nums))])
		} else {
			b.WriteRune(letters[rand.Intn(len(letters))])
		}
	}
	return b.String()
}

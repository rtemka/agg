package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rtemka/agg/news/pkg/storage"
)

type stor = storage.Storage
type item = storage.Item
type filter = storage.Filter
type timefilter = storage.TimeFilter

var (
	ErrInternal = errors.New("internal server error")
	ErrBadInput = errors.New("invalid input")
)

type ctxKey int

const (
	requestID ctxKey = iota
)

// параметр запроса.
const (
	pageQP    = "page"
	excludeQP = "exc"
	sortByQP  = "sortBy"
	dateQP    = "date"
	dateEndQP = "dateEnd"
	searchQP  = "s"
)

const (
	layoutDate = "2006-01-02" // YYYY-MM-DD
)

type Pagination struct {
	TotalPages  int `json:"total_pages"`
	PageSize    int `json:"page_size"`
	CurrentPage int `json:"page_number"`
	PageData    any `json:"page"`
}

// API приложения.
type API struct {
	r         *mux.Router
	db        stor
	logger    *log.Logger
	debugMode bool
}

// Возвращает новый объект *API
func New(storage stor, logger *log.Logger) *API {
	api := API{
		r:         mux.NewRouter(),
		db:        storage,
		logger:    logger,
		debugMode: false,
	}
	api.endpoints()
	return &api
}

// DebugMode переключает debug режим у *API
func (api *API) DebugMode(mode bool) *API {
	api.debugMode = mode
	return api
}

// Router возвращает маршрутизатор запросов.
func (api *API) Router() *mux.Router {
	return api.r
}

func (api *API) endpoints() {
	api.r.Use(
		api.requestIDMiddleware,
		api.logRequestMiddleware,
		api.closerMiddleware,
		api.headersMiddleware,
	)
	// получить новости
	api.r.HandleFunc("/news", api.itemsHandler).Methods(http.MethodGet, http.MethodOptions)
	api.r.HandleFunc("/news/{id}", api.itemHandler).Methods(http.MethodGet, http.MethodOptions)
}

func (api *API) headersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
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

// logRequestMiddleware логирует request
func (api *API) logRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		api.logger.Printf("request-id=%v, method=%s path=%s query=%s vars=%s remote=%s",
			r.Context().Value(requestID), r.Method, r.URL.Path, r.URL.Query(), mux.Vars(r), r.RemoteAddr)
	})
}

func (api *API) WriteJSONError(w http.ResponseWriter, err error, code int) {
	w.WriteHeader(code)
	msg := map[string]string{"error": err.Error()}
	_ = json.NewEncoder(w).Encode(&msg)
}

func (api *API) WriteJSON(w http.ResponseWriter, data any, code int) {
	w.WriteHeader(code)
	if data == nil {
		return
	}

	_ = json.NewEncoder(w).Encode(data)
}

// itemHandler возвращает одну новость по id.
func (api *API) itemHandler(w http.ResponseWriter, r *http.Request) {

	s := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		api.WriteJSON(w, "not found", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	it, err := api.db.Item(ctx, id)
	if err != nil {
		if it == (item{}) {
			api.WriteJSON(w, "not found", http.StatusNotFound)
			return
		}
		api.WriteJSONError(w, ErrInternal, http.StatusInternalServerError)
		return
	}

	api.WriteJSON(w, it, http.StatusOK)
}

// itemsHandler возвращает все новости.
func (api *API) itemsHandler(w http.ResponseWriter, r *http.Request) {

	f, err := api.parseQP(r.URL)
	if err != nil {
		api.WriteJSONError(w, err, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	total, err := api.db.CountItems(ctx, f)
	if err != nil {
		api.WriteJSONError(w, ErrInternal, http.StatusInternalServerError)
		return
	}

	items, err := api.db.Items(ctx, f)
	if err != nil {
		api.WriteJSONError(w, ErrInternal, http.StatusInternalServerError)
		return
	}

	p := Pagination{
		TotalPages: func() int {
			t := total / storage.PageSize
			tf := float64(total) / storage.PageSize
			if tf > float64(t) {
				return t + 1
			}
			return t
		}(),
		PageSize:    storage.PageSize,
		CurrentPage: f.Page,
		PageData:    items,
	}

	if len(items) == 0 {
		api.WriteJSON(w, p, http.StatusNoContent)
		return
	}

	api.WriteJSON(w, p, http.StatusOK)
}

// parseQP - парсит параметеры запроса: ?page=NUM.
// Возвращает фильтр.
func (api *API) parseQP(u *url.URL) (filter, error) {
	var (
		f   filter
		err error
	)

	params := u.Query()

	if qp, ok := params[pageQP]; ok {
		f.Page, err = strconv.Atoi(qp[0])
		if err != nil {
			api.logger.Printf("parse query param: %v", err)
			return f, fmt.Errorf("bad %q parameter: must be: page=NUM", pageQP)
		}
	} else {
		f.Page = 1
	}

	if qp, ok := params[sortByQP]; ok {
		f.SortBy, err = sortQParser(qp[0])
		if err != nil {
			api.logger.Printf("[ERROR] parse query param: %v", err)
			return f, err
		}
	}

	if qp, ok := params[dateQP]; ok {
		f.Date, err = timeQParser(qp[0], layoutDate)
		if err != nil {
			api.logger.Printf("[ERROR] parse query param: %v", err)
			return f, fmt.Errorf("bad %q parameter: must be of the form: YYYY-MM-DD", dateQP)
		}
	}

	if qp, ok := params[dateEndQP]; ok {
		f.EndDate, err = timeQParser(qp[0], layoutDate)
		if err != nil {
			api.logger.Printf("[ERROR] parse query param: %v", err)
			return f, fmt.Errorf("bad %q parameter: must be of the form: YYYY-MM-DD", dateEndQP)
		}
		if strings.Contains(f.EndDate.Operator, ">") || f.EndDate.Operator == "=" {
			return f, fmt.Errorf("bad %#q parameter: only '<' or '<=' is allowed", dateEndQP)
		}
		if f.Date.Value == 0 {
			return f, fmt.Errorf("bad %q parameter: you can't use it alone without %q,"+
				"if you want to search by date use just %s=[lte:gte]YYYY-MM-DD", dateEndQP, dateQP, dateQP)
		}
	}

	f.TitleSearch = append(f.TitleSearch, params[searchQP]...)
	f.Exclude = append(f.Exclude, params[excludeQP]...)

	return f, nil
}

func sortQParser(s string) (storage.Sort, error) {
	switch s {
	case "":
		return storage.Empty, nil
	case "date":
		return storage.Date, nil
	case "title":
		return storage.Title, nil
	case "match":
		return storage.Rank, nil
	default:
		return 0, fmt.Errorf("bad %#q parameter, must be either: 'date', 'title' or 'match'", sortByQP)

	}
}

// timeQParser - парсит параметер запроса ?date=[gte:lte:]YYYY-MM-DD
func timeQParser(qp, layout string) (timefilter, error) {
	if strings.HasPrefix(qp, "gte:") || strings.HasPrefix(qp, "lte:") {

		split := strings.SplitN(qp, ":", 2)
		// ожидаем в виде ?date=[gte:lte:]2012-12-31
		t, err := time.Parse(layout, split[1])
		if err != nil {
			return timefilter{}, err
		}
		return timefilter{Value: t.Unix(), Operator: operator(split[0])}, nil
	}

	t, err := time.Parse(layout, qp) // ожидаем в виде ?date=2012-12-31
	if err != nil {
		return timefilter{}, err
	}

	return timefilter{Value: t.Unix(), Operator: "="}, nil
}

func operator(o string) string {
	switch o {
	case "gte":
		return ">="
	case "lte":
		return "<="
	case "lt":
		return "<"
	case "gt":
		return ">"
	default:
		return "="
	}
}

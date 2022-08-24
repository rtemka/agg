package rsscollector

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"

	"strings"
	"sync"
	"time"

	"github.com/rtemka/agg/news/pkg/storage"
	"golang.org/x/net/html/charset"
)

// container - это наша зависимость, тип
// с которым работает пакет rsscollector
type container = storage.ItemContainer
type item = storage.Item

// Collector объект для обхода rss-ссылок
type Collector struct {
	logger *log.Logger
	poll   poller
	// когда установлен в true, логгирует промежуточные итоги,
	// по-умолчанию false
	debugMode bool
}

// Новый объект *Collector
func New(logger *log.Logger) *Collector {
	return &Collector{
		logger:    logger,
		poll:      poll, // функция опроса по ссылке
		debugMode: false,
	}
}

// DebugMode переключает debug режим у *Collector
func (c *Collector) DebugMode(on bool) *Collector {
	c.debugMode = on
	return c
}

// Poll опрашивает переданные rss-ссылки c заданным интервалом времени.
func (c *Collector) Poll(ctx context.Context, interval time.Duration, links []string) (<-chan container, <-chan error, error) {

	if len(links) == 0 {
		return nil, nil, fmt.Errorf("collector poll: no links provided")
	}

	dests := make([]<-chan container, len(links)) // по каналу для каждой rss-ссылки
	errs := make([]<-chan error, len(links))      // каналы ошибок для каждой горутины

	// Fan-Out мультиплексируем каналы
	for i := 0; i < len(links); i++ {

		ch := make(chan container)
		dests[i] = ch
		ech := make(chan error)
		errs[i] = ech

		go func(id int, values chan<- container, errors chan<- error, url string) {

			var fails uint // считаем ошибки во время работы горутины
			var polls uint // считаем опросы горутины

			defer func() {
				c.logTotal(id, url, polls, fails) // лог общего итога
				close(values)
				close(errors)
			}()

			poll := func() {
				polls++
				v, err := c.poll(ctx, url) // выполняем опрос
				if err == nil {
					values <- v
				} else {
					fails++
					errors <- fmt.Errorf("rsscollector: poll: %w", err)
				}
				c.log(id, url, len(v.Items), err) // лог промежуточных итогов
			}

			poll() // первый опрос сразу

			for {
				select {
				case <-time.After(interval):
					poll()
				case <-ctx.Done():
					errors <- ctx.Err()
					return
				}
			}
		}(i+1, ch, ech, links[i])
	}

	return merge(dests...), merge(errs...), nil // Fan-In (демультиплексируем каналы)
}

func (c *Collector) log(id int, url string, received int, err error) {
	if err != nil {
		c.logger.Printf("[ERROR] unit #%03d >> error=%v; task=%s", id, err, url)
	}
	if c.debugMode {
		c.logger.Printf("[DEBUG] unit #%03d >> items_received=%03d task=%s", id, received, url)
	}
}

func (c *Collector) logTotal(id int, url string, polls, errors uint) {
	c.logger.Printf("[INFO] unit #%03d >> totals: polls=%d errors=%d >> task=%s", id, polls, errors, url)
}

// merge демультиплексирует(собирает) переданные
// каналы в единый канал и возвращает этот канал
func merge[T any](sources ...<-chan T) <-chan T {

	dest := make(chan T, len(sources)) // единый канал

	var wg sync.WaitGroup
	wg.Add(len(sources))

	// читаем все источники и направляем в единый канал
	for _, ch := range sources {
		go func(c <-chan T) {
			defer wg.Done()

			for val := range c {
				dest <- val
			}
		}(ch)
	}

	// ждём закрытия всех источников
	// и закрываем единый канал
	go func() {
		wg.Wait()
		close(dest)
	}()

	return dest
}

// responseHandler - обработчик http-ответа
type responseHandler interface {
	process(*http.Response) error
}

// responseHandlerFunc - адаптер для responseHandler,
// а-ля http.HandlerFunc только для работы с http.Response
type responseHandlerFunc func(*http.Response) error

func (f responseHandlerFunc) process(resp *http.Response) error {
	return f(resp)
}

// poller - функция для опроса rss-канала, возвращает прочитанное тело ответа
type poller func(ctx context.Context, url string) (container, error)

func poll(ctx context.Context, url string) (container, error) {

	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(c, http.MethodGet, url, nil)
	if err != nil {
		return container{}, err
	}
	// чтобы rss-каналы не посылали нам ошибку 403
	// ставим заголовок User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0")

	request := requestFunc(req) // функция для выполнения запроса по сети

	var cont container
	// функция чтения тела ответа
	decfunc := responseHandlerFunc(func(r *http.Response) error {
		return xmlDecoderWithSettings(r.Body).Decode(&cont)
	})

	chain := bodyCloser(statusChecker(xmlEnforcer(decfunc))) // цепочка обработчиков ответа

	return cont, request(chain)
}

// decoderWithSettings возвращает *xml.Decoder с настройками
func xmlDecoderWithSettings(r io.Reader) *xml.Decoder {
	decoder := xml.NewDecoder(r)
	decoder.CharsetReader = charset.NewReaderLabel // некоторые rss-каналы возвращают не UTF-8
	return decoder
}

// requester выполняет стандартный http-запрос
// и передает ответ обработчику responseHandler
type requester func(responseHandler) error

// requestFunc возвращает функцию requester, которая будет выполнять
// стандартный http-запрос и передавать ответ обработчику,
// который она принимает в качестве аргумента
func requestFunc(req *http.Request) requester {

	return func(handler responseHandler) error {

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		return handler.process(resp)
	}
}

func bodyCloser(next responseHandler) responseHandler {
	return responseHandlerFunc(func(resp *http.Response) error {
		defer func() {
			_ = resp.Body.Close()
		}()
		return next.process(resp)
	})
}

func statusChecker(next responseHandler) responseHandler {
	return responseHandlerFunc(func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("statusChecker: response code is %d", resp.StatusCode)
		}
		return next.process(resp)
	})
}

func xmlEnforcer(next responseHandler) responseHandler {
	return responseHandlerFunc(func(resp *http.Response) error {
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "xml") {
			return fmt.Errorf("xmlEnforcer: Content-Type is '%s', want '...+xml", ct)
		}
		return next.process(resp)
	})
}

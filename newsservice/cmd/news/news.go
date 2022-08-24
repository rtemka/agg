package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rtemka/agg/news/pkg/api"
	"github.com/rtemka/agg/news/pkg/rsscollector"
	"github.com/rtemka/agg/news/pkg/storage"
	"github.com/rtemka/agg/news/pkg/storage/postgres"
	"github.com/rtemka/agg/news/pkg/storage/streamwriter"
)

// имя подсистемы для логирования
var (
	rsscolName = fmt.Sprintf("%16s", "[RSS Collector] ")
	dwName     = fmt.Sprintf("%16s", "[DB Writer] ")
	apiName    = fmt.Sprintf("%16s", "[WEB API] ")
)

// переменная окружения.
const (
	portEnv   = "NEWS_PORT"
	newsDBEnv = "NEWS_DB_URL"
)

// config - структура для хранения конфигурации
// передаваемой в качестве аргумента коммандной строки
type config struct {
	Links        []string `json:"rss"`            // массив ссылок для опроса
	SurveyPeriod int      `json:"request_period"` // период опроса ссылок в минутах
}

// readConfig функция для чтения файла конфигурации
func readConfig(path string) (*config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var c config

	return &c, json.NewDecoder(f).Decode(&c)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 1 {
		return fmt.Errorf("usage: %s <path-to-config-file>", os.Args[0])
	}
	_ = godotenv.Load() // загружаем переменные окружения

	config, err := readConfig(os.Args[1])
	if err != nil {
		return err
	}

	em, err := envs(newsDBEnv, portEnv)
	if err != nil {
		return err
	}

	db, err := connectDB(em[newsDBEnv], 10, time.Second)
	if err != nil {
		return err
	}
	defer db.Close()

	// логгеры для подсистем
	rsslog := log.New(os.Stdout, rsscolName, log.Lmsgprefix|log.LstdFlags)
	dbwriterlog := log.New(os.Stdout, dwName, log.Lmsgprefix|log.LstdFlags)
	apilog := log.New(os.Stdout, apiName, log.Lmsgprefix|log.LstdFlags)

	collector := rsscollector.New(rsslog).DebugMode(true)               // RSS-обходчик
	sw := streamwriter.NewStreamWriter(dbwriterlog, db).DebugMode(true) // объект пишуший в БД
	webapi := api.New(db, apilog)                                       // REST API

	// конфигурируем сервер
	srv := &http.Server{
		Addr:              em[portEnv],
		Handler:           webapi.Router(),
		IdleTimeout:       3 * time.Minute,
		ReadHeaderTimeout: time.Minute,
	}

	// создаем контекст для регулирования закрытия всех подсистем
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interval := time.Minute * time.Duration(config.SurveyPeriod)
	values, errs, err := collector.Poll(ctx, interval, config.Links)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	wg.Add(3)

	// читаем канал с ошибками
	go func() {
		errLogger(errs)
		wg.Done()
	}()

	// читаем канал с новостями и пишем в БД
	go func() {
		_, err = sw.WriteToStorage(ctx, values)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		wg.Done()
	}()

	// сервер
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		} else {
			log.Println(err) // server closed
		}
		wg.Done()
	}()
	log.Println(apiName, "server started at", srv.Addr)

	// ловим сигналы прерывания типа CTRL-C
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		s := <-stop // получили сигнал прерывания
		log.Println("got os signal", s)

		// закрываем сервер
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}

		cancel() // закрываем контекст приложения
	}()

	wg.Wait() // ждём всех

	return nil
}

// envs собирает ожидаемые переменные окружения,
// возвращает ошибку, если какая-либо из переменных env не задана.
func envs(envs ...string) (map[string]string, error) {
	em := make(map[string]string, len(envs))
	var ok bool
	for _, env := range envs {
		if em[env], ok = os.LookupEnv(env); !ok {
			return nil, fmt.Errorf("environment variable %q must be set", env)
		}
	}
	return em, nil
}

var ErrRetryExceeded = errors.New("connect DB: number of retries exceeded")

func connectDB(connstr string, retries int, interval time.Duration) (storage.Storage, error) {

	for i := 0; i < retries; i++ {
		db, err := postgres.New(connstr)
		if err != nil {
			log.Println(err)
			time.Sleep(interval)
			continue
		}

		return db, nil
	}

	return nil, ErrRetryExceeded
}

// errLogger логирует ошибки приходящие от подсистем.
func errLogger(errs <-chan error) {

	for err := range errs {
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "%T %v\n", err, err)
		}
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rtemka/agg/comments/domain"
	"github.com/rtemka/agg/comments/pkg/api"
	"github.com/rtemka/agg/comments/pkg/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// имя переменной окружения
const (
	portEnv = "COMMENTS_PORT"
	dbURL   = "DB_URL"
)

// настройки базы данных
const (
	maxConns        = 50
	maxConnIdleTime = 4 * time.Minute
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// переменные можно найти не только в файле
	_ = godotenv.Load()

	zl := zapLogger(os.Stdout)
	defer func() {
		_ = zl.Sync()
	}()

	em, err := envs(dbURL, portEnv)
	if err != nil {
		return err
	}

	db, err := connectDB(em[dbURL], 5, time.Second)
	if err != nil {
		return err
	}
	defer db.Close()

	// создание контекста для регулирования
	// закрытие всех подсистем
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	servers := []*http.Server{
		startRestServer(em[portEnv], db, zl, &wg),
	}

	// логика закрытия сервера
	cancelation(cancel, zl, servers)

	wg.Wait()

	return nil
}

// cancellation отслеживает сигналы прерывания и,
// если они получены, "мягко" отменяет контекст приложения и
// гасит серверы.
func cancelation(cancel context.CancelFunc, logger *zap.Logger, servers []*http.Server) {
	// ловим сигналов прерывания, типа CTRL-C
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-stop // получили сигнал
		sl := logger.Sugar()
		sl.Warnf("got signal %q", sig)

		// закрываем серверы
		for i := range servers {
			if err := servers[i].Shutdown(context.Background()); err != nil {
				sl.Info(err)
			}
		}

		cancel() // закрываем контекст приложения
	}()
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

func connectDB(connstr string, retries int, interval time.Duration) (domain.Repository, error) {

	for i := 0; i < retries; i++ {
		db, err := sqlite.New(connstr)
		if err != nil {
			log.Println(err)
			time.Sleep(interval)
			continue
		}
		db.DB.SetConnMaxIdleTime(maxConnIdleTime)
		db.DB.SetMaxOpenConns(maxConns)
		db.DB.SetMaxIdleConns(maxConns)

		if err := db.RunFile(filepath.Join("comments.sql")); err != nil {
			return nil, err
		}

		return db, nil
	}

	return nil, ErrRetryExceeded
}

// startRestServer запускает сервер REST API.
func startRestServer(addr string, db domain.Repository, logger *zap.Logger, wg *sync.WaitGroup) *http.Server {
	// REST API
	api := api.New(db, logger)

	// конфигурируем сервер
	srv := &http.Server{
		Addr:              addr,
		Handler:           api,
		IdleTimeout:       3 * time.Minute,
		ReadHeaderTimeout: time.Minute,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error(err.Error())
		}
		logger.Warn("server is shut down")
		wg.Done()
	}()
	logger.Info("REST server started", zap.String("address", srv.Addr))
	return srv
}

var encoderCfg = zapcore.EncoderConfig{
	MessageKey: "msg",
	NameKey:    "name",

	LevelKey:    "level",
	EncodeLevel: zapcore.CapitalLevelEncoder,

	CallerKey:    "caller",
	EncodeCaller: zapcore.ShortCallerEncoder,

	TimeKey:    "time",
	EncodeTime: zapcore.RFC3339TimeEncoder,
}

func zapLogger(w io.Writer) *zap.Logger {
	zl := zap.New(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderCfg),
			zapcore.Lock(zapcore.AddSync(w)),
			zapcore.DebugLevel,
		),
		zap.AddCaller(),
	)
	return zl
}

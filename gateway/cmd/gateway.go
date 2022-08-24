package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rtemka/agg/gateway/pkg/api"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// переменная окружения.
const (
	portEnv              = "GATEWAY_PORT"
	newsServiceEnv       = "NEWS_ADDR"
	commentsServiceEnv   = "COMMENTS_ADDR"
	commsCheckServiceEnv = "COMMENTS_CHECK_ADDR"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // загружаем переменные окружения
	em, err := envs(portEnv, newsServiceEnv, commentsServiceEnv, commsCheckServiceEnv)
	if err != nil {
		return err
	}

	zl := zapLogger(os.Stdout)
	defer func() {
		_ = zl.Sync()
	}()

	// создание контекста для регулирования
	// закрытие всех подсистем
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	servers := []*http.Server{
		startRestServer(zl, em, &wg),
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

// startRestServer запускает сервер REST API.
func startRestServer(logger *zap.Logger, env map[string]string, wg *sync.WaitGroup) *http.Server {
	// REST API
	a := api.New(logger)
	a.Services[api.NewsServiceName] = env[newsServiceEnv]
	a.Services[api.CommentsServiceName] = env[commentsServiceEnv]
	a.Services[api.CommsCheckServiceName] = env[commsCheckServiceEnv]

	// конфигурируем сервер
	srv := &http.Server{
		Addr:              env[portEnv],
		Handler:           a,
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

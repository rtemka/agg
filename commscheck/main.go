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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// имя переменной окружения
const (
	portEnv = "COMMSCHECK_PORT"
)

// Comment - модель данных комментария к rss-новости.
type Comment struct {
	ID       int64  `json:"id"`
	NewsID   int64  `json:"news_id"`
	ReplyID  int64  `json:"reply_id,omitempty"`
	PostedAt int64  `json:"posted_at"`
	Text     string `json:"text"`
	Author
}

// Author - автор комментария к новости.
type Author struct {
	ID   int64  `json:"author_id,omitempty"`
	Name string `json:"author"`
}

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

	port, ok := os.LookupEnv(portEnv)
	if !ok {
		zl.Sugar().Fatalf("environment variable %q must be set", portEnv)
	}

	// создание контекста для регулирования
	// закрытие всех подсистем
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	servers := []*http.Server{
		startRestServer(port, zl, &wg),
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

// startRestServer запускает сервер REST API.
func startRestServer(addr string, logger *zap.Logger, wg *sync.WaitGroup) *http.Server {
	// REST API
	api := NewApi(logger)

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

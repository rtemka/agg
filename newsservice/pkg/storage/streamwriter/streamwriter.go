package streamwriter

import (
	"context"
	"log"

	"time"

	"github.com/rtemka/agg/news/pkg/storage"
)

// container - объекты которые получает streamwriter
type container = storage.ItemContainer
type item = storage.Item

// stor - хранилище, в которое пишет streamwriter
type stor = storage.Storage

// StreamWriter пишет в БД, то что читает из канала
type StreamWriter struct {
	log     *log.Logger
	storage storage.Storage
	// когда установлен в true, логгирует промежуточные итоги,
	// по-умолчанию false
	debugMode bool
}

// NewStreamWriter возвращает новый объект *StreamWriter
func NewStreamWriter(log *log.Logger, storage stor) *StreamWriter {
	return &StreamWriter{
		log:       log,
		storage:   storage,
		debugMode: false,
	}
}

// DebugMode переключает debug режим у *StreamWriter
func (sw *StreamWriter) DebugMode(on bool) *StreamWriter {
	sw.debugMode = on
	return sw
}

// Stats - статистика работы *StreamWriter
type Stats struct {
	Containers uint // обработанные контейнеры
	Items      uint // обработанные новости
	Errs       uint // полученные ошибки
}

// WriteToStorage пишет в БД поступающие данные из канала,
// возвращает статистику своей работы и ошибку, если БД мертва
// или все приходящие данные не удается записать
func (sw *StreamWriter) WriteToStorage(ctx context.Context, in <-chan container) (Stats, error) {
	var stats Stats
	var threshold = cap(in)

	statsCh := make(chan Stats)
	defer close(statsCh)
	go sw.logDebug(statsCh, cap(in)) // логгирование промежуточных итогов

	for v := range in {

		dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := sw.storage.AddItems(dbctx, v.Items)
		if err != nil {
			sw.log.Printf("[ERROR] db_error=%v", err) // логгируем ошибку
			stats.Errs++
			statsCh <- stats
			// если беда со всей пачкой пришедших значений, то
			// смысла продолжать нет
			if stats.Errs >= uint(threshold) {
				return stats, err
			}
		}

		stats.Containers++
		stats.Items += uint(len(v.Items))

		statsCh <- stats
	}

	// лог общий итог
	sw.log.Printf("[INFO] totals: received_containers=%d received_items=%d db_errors=%d",
		stats.Containers, stats.Items, stats.Errs)

	return stats, nil
}

func (sw *StreamWriter) logDebug(in <-chan Stats, cycle int) {

	logcycle := cycle

	if sw.debugMode {
		for s := range in {

			if logcycle <= 0 {
				sw.log.Printf("[DEBUG] running totals: received_containers=%d received_items=%d db_errors=%d",
					s.Containers, s.Items, s.Errs)
			} else {
				logcycle--
			}

		}
	} else {
		for range in {
		}
	}
}

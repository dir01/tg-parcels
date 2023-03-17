package storage

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/dir01/tg-parcels/core"
	"github.com/hori-ryota/zaperr"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

func NewStorage(db *sqlx.DB) *Storage {
	s := &Storage{db: db, writeAccessMutex: &sync.Mutex{}}
	var _ core.Storage = s
	return s
}

type Storage struct {
	db               *sqlx.DB
	writeAccessMutex *sync.Mutex
}

func (s *Storage) SaveTracking(ctx context.Context, tracking *core.Tracking) (*core.Tracking, error) {
	dbTracking, err := dbStruct{}.fromBusinessStruct(tracking)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO trackings
			(user_id, tracking_number, display_name, payload, last_polled_at)
		VALUES
			(:user_id, :tracking_number, :display_name, :payload, :last_polled_at)
		`
	if dbTracking.ID == 0 || len(dbTracking.Payload) == 0 {
		query = query + `
		ON CONFLICT DO UPDATE SET display_name=excluded.display_name
		` // can't use `DO NOTHING` or `RETURNING` won't work
	} else {
		query = query + `
		ON CONFLICT DO UPDATE SET payload=excluded.payload, last_polled_at=excluded.last_polled_at, display_name=excluded.display_name
		`
	}
	query = query + `
		RETURNING id`

	query = strings.ReplaceAll(query, "\n", " ")
	query = strings.ReplaceAll(query, "\t", " ")
	fields := []zap.Field{
		zap.String("query", query),
		zap.Any("dbTracking", dbTracking),
	}

	// For some reason, sqlx.NamedQueryContext does nothing (not persisting data).
	// And sqlx.NamedExecContext only returns id for inserts, but not updates.
	// So we prepare query manually with sqlx.Named and then use sql.QueryRowContext,
	// this way it works.
	bindQ, bindA, err := sqlx.Named(query, dbTracking)
	if err != nil {
		return nil, zaperr.Wrap(err, "failed to bind", fields...)
	}

	s.writeAccessMutex.Lock()
	defer s.writeAccessMutex.Unlock()

	if err := s.db.DB.QueryRowContext(ctx, bindQ, bindA...).Scan(&dbTracking.ID); err != nil {
		return nil, zaperr.Wrap(err, "failed to execute", fields...)
	}

	tracking, err = dbTracking.toBusinessStruct()
	if err != nil {
		return nil, err
	}

	return tracking, nil
}

func (s *Storage) GetTrackingsLastPolledBefore(ctx context.Context, time time.Time) ([]*core.Tracking, error) {
	var dbTrackings []*dbStruct
	err := s.db.SelectContext(ctx, &dbTrackings, `
		SELECT * FROM trackings WHERE last_polled_at is NULL OR last_polled_at < ?`, time,
	)
	if err != nil {
		return nil, err
	}

	var trackings []*core.Tracking
	for _, dbTracking := range dbTrackings {
		tracking, err := dbTracking.toBusinessStruct()
		if err != nil {
			return nil, err
		}
		trackings = append(trackings, tracking)
	}
	return trackings, nil
}

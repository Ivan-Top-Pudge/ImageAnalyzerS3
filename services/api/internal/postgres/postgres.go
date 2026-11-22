// Package postgres хранит задачи анализа и события outbox в PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/domain"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Storage использует пул соединений PostgreSQL для работы с задачами анализа.
type Storage struct {
	pool *pgxpool.Pool
}

// New создаёт репозиторий с переданным пулом соединений.
func New(pool *pgxpool.Pool) *Storage {
	return &Storage{
		pool: pool,
	}
}

// CreateWithOutbox сохраняет задачу и событие outbox в одной транзакции.
// Возвращает UUID задачи после успешной фиксации транзакции.
func (s *Storage) CreateWithOutbox(ctx context.Context, imageID string) (uuid.UUID, error) {
	const op = "storage.CreateWithOutbox"

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%s failed to begin transaction: %w", op, errTaskNotCommited(err))
	}

	defer func(tx pgx.Tx, ctx context.Context) {
		// TODO: fix hardcode timeout
		rollBackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err := tx.Rollback(rollBackCtx)
		if err != nil {
			// TODO: log
		}
	}(tx, ctx)

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	var taskID uuid.UUID
	sql, args, err := psql.
		Insert("tasks").Columns("object_key").
		Values(imageID).Suffix("RETURNING \"id\"").ToSql()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%s failed to generate sql: %w", op, errTaskNotCommited(err))
	}

	if err = tx.QueryRow(ctx, sql, args...).Scan(&taskID); err != nil {
		return uuid.Nil(), fmt.Errorf("%s failed to insert task for object %s: %w", op, imageID, errTaskNotCommited(err))
	}

	sql, args, err = psql.
		Insert("outbox").Columns("task_id").
		Values(taskID).ToSql()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%s failed to generate sql: %w", op, errTaskNotCommited(err))
	}

	_, err = tx.Exec(ctx, sql, args...)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%s failed to insert into outbox for task %s: %w", op, taskID.String(), errTaskNotCommited(err))
	}

	if err = tx.Commit(ctx); err != nil {
		if errors.Is(err, pgx.ErrTxCommitRollback) {
			return uuid.Nil(), fmt.Errorf("%s failed to commit transaction: %w", op, errTaskNotCommited(err))
		}
		return uuid.Nil(), fmt.Errorf("%s: %w", op, err)
	}

	return taskID, nil
}

// errTaskNotCommited добавляет ErrTaskNotCommitted к исходной ошибке.
func errTaskNotCommited(err error) error {
	return errors.Join(domain.ErrTaskNotCommitted, err)
}

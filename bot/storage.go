package bot

import (
	"context"

	"github.com/jmoiron/sqlx"
)

func NewStorage(db *sqlx.DB) Storage {
	return &SqliteStorage{db: db}
}

type SqliteStorage struct {
	db *sqlx.DB
}

func (s *SqliteStorage) SaveUserChatID(ctx context.Context, userID int64, chatID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users_chats (user_id, chat_id) VALUES (?, ?) 
		ON CONFLICT DO UPDATE SET chat_id = ?`, userID, chatID, chatID)
	if err != nil {
		return err
	}
	return nil
}

func (s *SqliteStorage) UserChatID(ctx context.Context, userID int64) (int64, error) {
	var chatID int64
	err := s.db.GetContext(ctx, &chatID, `SELECT chat_id FROM users_chats WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return chatID, nil
}

package repository

import (
	"context"

	"time"
)

type SyncGapConv struct {
	Platform       string
	AccountID      string
	ConversationID string
	CustomerID     string
}

func (r *MessageHubRepository) FindSyncGapConversations(ctx context.Context, since time.Time) ([]SyncGapConv, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	const triad = "(CASE" +
		" WHEN m.is_group AND m.conversation_id <> '' THEN m.conversation_id" +
		" WHEN m.conversation_id <> '' AND (m.sender_id LIKE (m.conversation_id || ' %') OR m.receiver_id LIKE (m.conversation_id || ' %')) THEN m.conversation_id" +
		" ELSE (CASE WHEN m.direction = 'inbound' THEN m.sender_id ELSE m.receiver_id END)" +
		" END)"
	missingInbox := r.db.WithContext(ctx).
		Select("1").
		Table("inbox_conversations ic").
		Where("ic.platform = m.platform AND ic.account_id = m.account_id AND ic.customer_id = " + triad)
	var rows []SyncGapConv
	err := r.db.WithContext(ctx).
		Table("message_hub m").
		Distinct().
		Select("m.platform, m.account_id, m.conversation_id, "+triad+" AS customer_id").
		Where("m.conversation_id IS NOT NULL AND m.conversation_id <> ''").
		Where("m.created_at >= ?", since).
		Where("NOT EXISTS (?)", missingInbox).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

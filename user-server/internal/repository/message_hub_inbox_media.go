package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// SetInboundMediaURLs 按 (platform, account_id, conversation_id, msg_id) 四键定位入站行，
// 一次写全转存后的媒体链接：media_url 存 urls[0]（列表页与工作台只读这一列），Extra.media_urls 存全量。
// urls[0] 是首个成功转存的链接，不是官方顺序里的第一张。
// 会话维度必须在键里：message_hub 唯一键是 (platform, msg_id, conversation_id)，
// 少这一维的 First 会把媒体写进同消息的另一行；账号维度是 fail-closed 保险，对不上宁可漏回填也不写错行。
// found=false 表示没有匹配的 hub 行（入站落库与异步转存间的竞态），此时不碰任何列；读库真错走 err。
func (r *MessageHubRepository) SetInboundMediaURLs(
	ctx context.Context,
	platform, accountID, conversationID, msgID string,
	urls []string,
) (found bool, err error) {
	// 句柄缺失＝装配漏线，不是"这条消息不存在"。
	if r == nil || r.db == nil {
		return false, errors.New("message hub media backfill: repository has no DB handle")
	}
	if platform == "" || accountID == "" || conversationID == "" || msgID == "" || len(urls) == 0 {
		return false, nil
	}
	var hub model.MessageHub
	if err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND conversation_id = ? AND msg_id = ?",
			platform, accountID, conversationID, msgID).
		First(&hub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	// 合并而非整列替换：入站时落在 extra 里的其他键必须留住，回填只新增 media_urls 一项。
	extra := mergeHubExtra(hub.Extra, map[string]any{"media_urls": urls})
	if err := r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ?", hub.ID).
		Updates(map[string]any{"media_url": urls[0], "extra": extra}).Error; err != nil {
		return true, err
	}
	return true, nil
}

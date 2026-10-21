// chat_channel_status_test.go 渠道状态值域的回归（实体 #58 审计补充）。
//
// chat_channels.status 没有 CHECK 约束，服务层是唯一的值域落点：写进一格拼错的状态后，
// ChatChannelIsActive 判否、而任何读侧都不会报错 —— 访客 widget 静默失效。
// 本用例钉住"非法状态拒绝且不落库，合法状态照常生效"。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

func TestChatChannelUpdate_StatusDomainEnforced(t *testing.T) {
	database := testutil.NewTestDB(t, &model.ChatChannel{})
	svc := MustNewChatChannelService(repository.NewChatChannelRepositoryWithDB(database))
	ctx := context.Background()

	created, err := svc.Create(ctx, &CreateChannelRequest{
		ChannelName:    "状态值域用例",
		AllowedOrigins: []string{"https://example.com"},
	}, 1)
	if err != nil {
		t.Fatalf("建渠道失败: %v", err)
	}
	channelID := created.Channel.ChannelID

	bad := "ARCHIVED"
	if _, err := svc.Update(ctx, channelID, &UpdateChannelRequest{Status: &bad}); err == nil {
		t.Fatal("非法状态应被拒绝，实际更新成功")
	}
	got, err := svc.GetByChannelID(ctx, channelID)
	if err != nil {
		t.Fatalf("回读渠道失败: %v", err)
	}
	if got.Status != model.ChatChannelStatusActive {
		t.Errorf("非法状态被写库：status=%q want %q", got.Status, model.ChatChannelStatusActive)
	}

	disabled := string(model.ChatChannelStatusDisabled)
	got, err = svc.Update(ctx, channelID, &UpdateChannelRequest{Status: &disabled})
	if err != nil {
		t.Fatalf("合法状态更新失败: %v", err)
	}
	if got.Status != model.ChatChannelStatusDisabled {
		t.Errorf("status=%q want %q", got.Status, model.ChatChannelStatusDisabled)
	}

	// 首尾空白是表单常态：清洗后落在值域内应放行，而不是当成另一个状态。
	padded := "  active  "
	got, err = svc.Update(ctx, channelID, &UpdateChannelRequest{Status: &padded})
	if err != nil {
		t.Fatalf("带空白的合法状态应被清洗后放行: %v", err)
	}
	if got.Status != model.ChatChannelStatusActive {
		t.Errorf("清洗后 status=%q want %q", got.Status, model.ChatChannelStatusActive)
	}
}

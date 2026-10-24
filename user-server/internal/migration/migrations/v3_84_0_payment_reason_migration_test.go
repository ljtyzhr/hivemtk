package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// v3.84.0（E-10 退款行"原因"格）验收判据：
//
//	① payments.reason 列存在（可空，varchar(255)）；
//	② 幂等重入：第二次 Up 不报错、不重复加列；
//	③ AutoMigrate 与迁移双路同形：模型 tag 建的新库与迁移改的存量库
//	 列形状一致（与 v3.82.0 同一验收取向）；
//	④ Down 不删列（declineColumnDrop：在用对象降级不销毁）。
func TestPaymentReasonMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	database := testutil.NewTestDBOrSkip(t, &model.Payment{})
	ctx := context.Background()

	m := NewPaymentReasonMigration(database)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	// 幂等重入
	if err := m.Up(ctx); err != nil {
		t.Fatalf("幂等重入失败: %v", err)
	}

	if !database.Migrator().HasColumn(&model.Payment{}, "reason") {
		t.Fatalf("① payments.reason 列不存在")
	}

	if err := m.Down(ctx); err != nil {
		t.Fatalf("④ Down 应拒绝删列而不报错: %v", err)
	}
	if !database.Migrator().HasColumn(&model.Payment{}, "reason") {
		t.Fatalf("④ Down 后 reason 列不应被删除（declineColumnDrop 判据）")
	}
}
